# macos/ — the gnat app

A native macOS app over the same tracker `nat` serves, built as a pure
SwiftPM package (`swift-tools-version:6.0`, `.macOS(.v15)`). No Notion, tmux
or GitHub logic of its own — every read/write goes through a `nat`
subprocess. All logic/state/models live in `NatKit`; `NatApp`'s views bind
to it and format/display, never compute. See `macos/README.md` for the
fuller structure and theme system.

## Build, run, test

- Dev run: `swift run --package-path macos gnat`. Tests:
  `swift test --package-path macos`. Release:
  `bash macos/Scripts/make-app.sh` → `macos/.build/gnat.app`.
- CI pins the **newest installed Xcode 26** (`xcode-select -s` against
  `/Applications/Xcode_26*.app`) — the runner's default is older and refuses
  the Swift 6.2 syntax this codebase uses.
- **Verify a UI change by rendering the gallery, not launching the app**
  (`gnat --list`/`--story`/`--all` — flags in `macos/README.md`). Add a story
  (`Sources/NatApp/Gallery/AppStories.swift`) before a live screenshot.
- Read a render cropped or downscaled to the element under test, never as a
  full window frame, and pause any animation (or fix it to one phase) before
  comparing — an untimed capture reads a mid-sweep shimmer back as a layout
  bug. `macos/README.md` has the incidents behind both.
- A key that misbehaves in the agent pane: `docs/debugging/agent-pane-keys.md`
  (the key chain, the `NAT_KEY_DEBUG` harness on a private tmux socket).

## NatClient: `nat` is the only source of truth

- `NatKit/NatClient/NatClient.swift` shells out to `nat <command> --json`.
  **Never reimplement tracker logic in Swift** — state, readiness, blocking,
  migration, write ordering all live in the Go binary.
- Every call on a tracked project passes `--project <id>`, no fallback,
  mirroring the Go CLI (exceptions mirror the Go CLI's own: `status`,
  `paths`, `config-show`/`-set`, `project-create`/`-open`, `source-list`,
  `source-setup`, `plugin-*`).
- Plugin install: `PluginModels.swift` mirrors `nat plugin-list` and the
  other `plugin-*` answers; Settings ▸ Sources draws them through
  `PluginsModel` (Installed, Available, Plugin sources), each button one
  `nat plugin-*` call and then a fresh `plugin-list`, and an install,
  update, uninstall or setup Save is `AppModel.pluginChanged` (a
  `PluginChange` naming the plugin): it re-reads `AppModel.sourcePlugins` —
  which is how a plugin just connected gets its section (see **Task
  sources**) — and then that plugin's source projects' plans (`rereadSource`;
  `info` asks the plugin every read, so the new binary's tree, menus and
  filter fields show at once). Uninstalling a plugin a project uses asks
  first (`PluginsModel.pendingUninstall`, an alert naming the projects);
  only "Delete and Uninstall" sends `--delete-projects`, and the projects
  nat answers it deleted lose their tabs and stores (`projectsDeleted`), the
  plugin free to make a fresh one if installed again. An
  installed row draws its `describe_error` as a warning line and its
  `setup` fields beneath it (`SecureField` for `secret`; `set == false` a
  "<label> not set" warning over it, `true` a quiet "<label> set" ✓ and a
  "Replace …" placeholder, nil neither); Save is
  `PluginsModel.saveSetup` → `NatClient.sourceSetup`, the value on
  **stdin only** (never an argument; `NatClient` logs no request), the
  field cleared and the plugin's message (or refusal) kept under it, then
  `plugin-list` re-read. Stories: `settings-sources`, `-loading`, `-error`,
  `-empty`, `-setup`, `-setup-saved`.
- Task sources: `SourceModels.swift` (and `ProjectInfo.source`,
  `SliceDetail.container`, `PlanBackend.source`) mirror
  `docs/design/task-sources/README.md` field for field — change the spec and
  the models together. Where they are drawn: see **Task sources** under The
  window.
- `NatBinary.resolve` never falls through to PATH for a packaged app:
  `NAT_BIN` (dev override) → the binary beside the app executable. No
  bundled `nat` is a **damaged install**, reported as such; only a bare dev
  executable outside any `.app` searches PATH.
- `PathBootstrap.bootstrap()` composes PATH at startup for the **agent tmux
  sessions nat spawns**, not for finding `nat` itself (`NatBinary`'s job).

## Settings and Done means merged

`SettingsView` (⌘,) is laid out as 1Password's settings are: a sidebar of
`SettingsTab` rows (tinted tile + name, the selection filled in the accent;
About apart under a rule) beside the section's groups (bold heading,
`settingRow`s left-aligned under it), one fixed 760×560 window whose
sections scroll — reads `nat config-show`, writes one `nat config-set <key>
<value>` per changed key. About reads `Bundle.main` (`AppVersion`, `dev`
where unset) and `nat --version` (`NatClient.natVersion`). Stories:
`settings`, `settings-agents*`, `settings-projects`, `settings-sources*`,
`settings-about`. `WorkflowStage`
(`stage(for:)`) is the one source of where a slice stands: the navigator's
phase (`NavigatorModel`), the sidebar's dots (`displayState(for:)`) and
`RailModel.isReviewSlice`/`isActiveSlice` all read it, and it mirrors
Go's `domain.StateOf` (Notion's status is the one source of lifecycle truth,
so a Done slice is never in-flight even with `nat pr-status` still reporting
its PR open) — change `internal/domain/state.go` and the stage together. A
live session never moves the stage. `fixing` is read off the record
(`Slice.fixing`, nat's `store.Fixing` on `info --json`): entered by a
Relaunched or Sent back after approval (a fix launch, a checks nudge), left
by the hand-back that follows — so a restart mid-fix still shows it, and no
in-memory mark exists. A `fixing` slice with no live agent draws like a
working one with none: relaunchable, not pulsing.

**Fix launch and failing checks.** `LaunchPlan` admits an approved slice (In
progress, PR recorded) with no live agent whatever its dependencies
(`isFix`); `NavigatorModel.launchIsFix` (the `pr` state) makes the Thread's
Launch and `LaunchCard` say "Launch fix agent", through the ordinary
`nat slice-launch` and its one-shot optimistic advance to the terminal.
`ReviewStatsStore.failingChecks` (from `pr-status`'s `checks`, replaced
each reading, kept on a failed one) drives the Active row's danger marker
(`SidebarActiveRow.failingChecks`, pr/fixing stage only) and
`checksNotice` — the notice atop the Thread and PR bodies: Launch fix agent
with no agent, "sent to the agent to fix" when the latest recorded event is
the nudge's Sent back. `projectAttention` counts a red pr/fixing slice once.
Stories: `window-pr-fix-launch`, `window-fixing`, `sidebar-checks-failing`,
`window-pr-checks-failing`, `window-pr-checks-agent-told`,
`window-task-log-checks-failed`.

## The workshop and its proposal

Every workshop proposes with `nat plan-propose` — an Untitled tab's by its
workspace, a project's (gnat-launched) by `--project` — and the app never
reads the proposal file itself. `AppModel.refreshProposals` reads `nat
plan-proposal` per tab — every Untitled tab's, and a project's only while its
planner is live, its row pinned or a proposal is up — on its own
`NudgeWatcher`, running for the app's life, plus the poll and opening a
workshop. **No timing is relied on:** each tab's proposal is a
`ProposalState` — a reading takes a ticket before it asks and lands only if
nothing happened since (no later reading landed, no Accept began or ended),
nothing lands mid-Accept, a reading that finds no file clears the proposal
(the file is the one source), and a closed tab is discarded, not removed, so
its in-flight readings are dropped. A project's Accept stays under way until
its plan is re-read from the **replica** (`ProjectStore.load(.replica)` — nat
wrote through it, so there is nothing to pull) and only then drops the
proposal: the tree has the plan before the Plan section goes. `ProjectStore`
never drops a load asked for mid-read — it owes one more read, answering
every request made meanwhile — and a nudge reads the replica (`refresh(.replica)`;
the poll and the user's refresh pull). `plan-accept` claims the proposal file
before filing (so it is accepted at most once) and nudges only once it is
gone. Tests: `ProposalStateTests`, `ProposalRaceTests`. The navigator's Plan
section draws `PlanProposal.folders` (`TreeMilestoneLine`/`TreeSliceLine`,
the sidebar's own rows) and the sidebar shows nothing of it; under the
created work, a project's proposal draws the Todo tasks it changes
(`PlanProposal.removals`/`moves`/`edits`, from the plan document's
`remove`/`move`/`edit`) — a struck-through row per removal, a move with its
destination, an edit that unfolds its new brief (`expandedProposalEdits`) —
and a removal is warned of above the tree (story
`workshop-proposal-superseding`). Accept is `nat plan-accept`: on an
Untitled tab it makes the project, then the session is killed and
`addProject(replacing:)` hands the tab over; on a project it files the plan
(`--project`) and leaves the session running. The layout is one for both:
the navigator's Brief (the request; Plan, then End session) over Plan (the
proposal, Accept and Keep workshopping — absent until there is one), and the main pane: the brief
editor before launch, with no tabs; from launch on the titlebar band's
`WorkshopTab`s (`TitlebarBand` takes `TitlebarTab`, which `MainPaneTab` and
`WorkshopTab` both map to) — Terminal, and Plan once there is a proposal
(`WorkshopTab.available`, `AppModel.workshopTab`). A launch puts Terminal up,
a proposal's first arrival Plan, a revision neither; Keep workshopping goes
back to Terminal. The Plan tab boxes each proposed slice's brief
(`PlanProposal.ProposedSlice`) under its milestone; the Plan header puts it
up, a slice row scrolls it (`showProposedSlice`), and ⌘1/⌘2 switch the two
while the workshop is on screen (`WorkshopMenuActions`). Opening a
workshop pins its row in Active (`workshopPinnedProjects`) until a launch or
the row's ✕. Stories: `workshop-*` (`workshop-proposal-scrolled` the Plan tab scrolled), `window-workshop*`,
`untitled-*`, `window-untitled-proposal`, `window-plan-accepted`.

## The Notion mirror nudge

After an accepted plan, `AppModel.acceptProposal` arms `MirrorNudgeMemory`
(UserDefaults, per project; `.inMemory()` for tests and stories) and the
sidebar's foot draws `MirrorNudgeCardView` while `mirrorNudgeShown` (armed **and** local).
✕ disarms for good. "Choose page…" opens `NotionPickerSheetView` over
`NotionPickerModel` (`nat notion-search`); "Create page" is
`mirrorActiveProject` → `nat project-mirror`, which changes the project's ID, so
`projectMirrored` hands the tab over in place. A refusal shows in the sheet and
changes nothing. Stories: `window-plan-accepted`, `notion-page-picker`.

## Release build quirks

- Bundled `nat` and `gnat` itself are both **universal**: built per-arch
  (`GOOS=darwin GOARCH=arm64|amd64 go build`; `swift build --arch <each>`)
  and `lipo -create`'d together — never `swift build --arch arm64 --arch
  x86_64` in one call, which routes through XCBuild's cross-arch path and
  can't resolve a binary-dependency target for anything but the host arch
  (swiftlang/swift-package-manager#7442). Needs the Go toolchain too.
- `tmux`, `gh`, `ntn` are never bundled — the machine's own install.

## The window

The gnat hi-fi design (Claude Design project `e81457f6-…`, `gnat.html` with
`gnat-data/shell/nav/main.jsx` and `gnat.css`) is the spec: `SidebarView`
(Active across every project, the Projects tree, then the scratch project
as a Scratch fold of its own — `SidebarModel.scratch`, whose unfiled
milestone's slices, `Milestone.unfiled`, sit loose at its head), the navigator's
stacked Thread/Changes/PR foldouts (`SliceNavigatorView`, with
`NavigatorModel` deciding phase, liveness and header actions). The brief is
the Thread's first card, not a section. A header click puts its section's
view up in the main pane — Thread the terminal, Changes the diff, PR the
description and conversation (the PR section keeps checks and review) — and
folds it again when that view is already up; the chevron only folds
(`NavigatorFocus`). Folded bodies stay built, so unfolding reloads nothing.
The main pane has no heading band: the titlebar band over it and the
navigator (`TitlebarBand`) carries the breadcrumb and its tabs and nothing
else. The live agent's model, effort and context — a slice's, a session's,
the planning agent's; none for a container — are the status bar's trailing
item (`AgentModelHeading`, in the bar's own sans, a divider before the context clause, the long form as a tooltip). A view's
actions live in the navigator section whose view they act on: the diff's
commit switcher (`DiffCommitsMenu`) a row atop the Changes body, Open in
GitHub in the PR head before Merge, a container's Open in <source> in its
Story head before New task (both `HeaderLinkButton`, glyph-only where the head
has no room); the workshop's launch shortcut is in the brief editor's
placeholder. The PR's title heads the PR view's own body. The Thread ends, while the slice can be launched, on a
ghost `LaunchCard` (greyed and hatched when blocked); its prose items cut
short as the brief does (`Excerpt`). View ▸ Hide done items
(`showsDoneItems`) drops done slices, ended sessions and the Done folder from
the sidebar. `AppModel` keeps its one *active* project —
every per-project reading is keyed by it — and the sidebar selects across
projects by activating first (`selectSlice(_:inProject:)`). The Thread shows
only what nat reports (`buildThreadEvents`). The titlebar is two bands:
the sidebar's holds Settings and the `+` (anything the sidebar makes, its
project asked for by submenu); one band over the navigator and main pane, no
rule at the split, holds the breadcrumb (`TitlebarBreadcrumb`) from the
navigator's inset — project, milestone or container (or a workshop's or
session's project name), each followed by a quiet slash, then the selection
as its Active row names it (`TitlebarIdentityLabel` over
`ActiveIdentityLabel`: dot, project tag, title, read through
`AppModel.titlebarIdentity`; the tag dropped where a crumb before it names
the project, `TitlebarIdentity.lastCrumb`) — free to run past the
navigator's width and ellipsize at its tail, then the `MainPaneTab`s at the
right — Zed-style tabs, full height and square, one per section that would
put its view up (`NavigatorModel.tabs`, `MainPaneTab.forSession`). The tabs
live only in the main pane's part of the band (`TitlebarBandLayout`), cut at
their leading edge rather than crossing the split; a tab is
`NavigatorFocus.showing`, which opens and never folds. The project, milestone
and container crumbs and the selection's own each open `CrumbTreePicker`
(projects → milestones → slices, `CrumbTree`) on themselves; with nothing
selected there is no breadcrumb. Stories: `titlebar-band-*`,
`status-bar-agent-readout*`, `changes-section-commits`. The Thread is labelled "Task" until the
slice is under way and "Task log" after, and draws `slice-show`'s `events`
in order — hand-backs, send-backs (`slice-rework --comments`), releases,
relaunches, notes (`nat slice-note`, headed "Another agent left a note";
`fromSlice` matched once against the loaded plan by name and milestone
name — `noteSourceSlice` — is a `task` fact drawn as the brief's
`DependencyRow`, through `ThreadEventCard.taskRow`, else `source` with the
provenance as text; notes alone open no log on a slice never launched),
follow-ups (a proposal is its count line, then one card per decided
follow-up — title, brief, the decision as its meta, a queued one's slice as
a `task` row; pending ones as the triage card in their place), then approve
and merge. A card for something happening now or awaiting the user
(`ThreadEvent.isLive`: the live agent, a pending proposal) is washed and
bordered in its hue (`threadCard(live:hot:)`); every key column takes the
width of `widestThreadFactKey` (`ThreadFactKey`), and the brief's Edit is
drawn only while the slice is Todo. Each card with an `at` shows it at
its header's end (`threadTimestamp`: time today, `d MMM` this year, `d MMM
y` before); Launched, approve and merge have none. Story:
`window-task-log-notes`. Selecting sets the selection *before* awaiting the project's
activation (`AppModel.select(inProject:)`), so a later click is never
overwritten by an earlier one finishing. What the design does not draw
(workshop, sessions, follow-ups, menus) lives on as the row or section it
belongs to. gnat is one `Window` scene — no tabs (`allowsAutomaticWindowTabbing`
off), no New Window. The menu bar reaches the window through focused scene
values (`MenuCommands.swift`): the sidebar, shell and slice navigator each
publish the actions they already own, nil where their control is disabled.

**Visual changes** is a fourth navigator section, between Changes and PR,
**absent** unless `slice-show`'s `visuals` is non-empty (the images an agent
handed in with `nat slice-visuals`), and so is its titlebar tab
(`MainPaneTab.visuals`; no pane-wide actions, zoom being per image).
`VisualStore` (one per project,
`AppModel.visualStore`) and `VisualReview` (the shell's) mirror `DiffStore`/
`DiffReview`: images loaded by URI through a swappable `loader` (local paths
only — any other URI is a placeholder card, no network), zoom per image,
comments per slice at a point in the image's own pixels or on the whole image,
and per-image viewed/folded marks that follow `DiffStore.toggleViewed`'s rule
(viewed folds; a newer image at that index starts afresh).
Send is `agent-send`, then `slice-rework` only where the slice is handed back;
a failed send keeps the comments. The comment box is drawn in the pane, not a
`.popover`, so the gallery can render it. `VisualsPane` is the one scrolling
pane SwiftUI lays out (pinned headers, `ScrollViewReader`) — safe only because
nothing draws until every image's pixel size is known and every image has an
explicit frame; keep it so. Stories: `window-visuals`,
`window-visuals-comments`, `visuals-zoomed`, `visuals-comment-editor`.

**Run commands** (`docs/run-commands.md`): a project's `runs` live in its
config entry alone (`ProjectConfig.runs`, `RunCommand`) — no settings screen.
The titlebar's play button (`TitlebarRunButton`, beside Settings) opens
`RunTreePicker`, `CrumbTreePicker`'s shape — every project with runs
(`AppModel.runProjects`), then the open one's runs; a handed-back slice gets `RunHeadingRow` under its Task section, its
`RunSplitButton` greyed once the stage is done. Both call
`AppModel.startRun` → `nat run`; nothing in Swift picks a directory or
default. The run's session is the Run tab (`MainPaneMode.run`,
`TitlebarTab.run`) beside Terminal, held in `AppModel.runs` until tmux says
it is gone (`watchRun`, `TmuxSession.exists`). Stories: `titlebar-run`,
`titlebar-run-menu`, `window-run-heading`, `window-run-heading-merged`,
`window-run-tab`.

**Task sources.** There is no new-project entry for one: **connecting a
plugin makes its section.** `AppModel.ensureSourceProjects` makes exactly one
source project — `project-create --source`, no working directory, and named
by the plugin wherever gnat names it (`AppModel.tabName`: the plugin's
`displayTitle` from `sourcePlugins`, else the plugin's name; never config's
`name`) — for each plugin whose `describe` is connected
(`SourceDescribe.isConnected`: no setup field `set == false`), takes it into
the sidebar without opening it, and makes it once per run whatever a config
re-read says. It runs after each reading of `source-list` and once config is
read — **only in an `AppModel` made with `makesSourceProjects: true`**, as
`NatApp` makes the app's: tests drive models over the machine's real `nat`
(the default `NatClient`), and must never write a project into the real
config. A source project (config `backend: source`, or a plan carrying
`source`) is pulled out of Projects into a section of its own — its heading,
then its own scroll, as Active, Projects and Scratch have — between Projects
and Scratch (`SidebarModel.sources`, `SidebarSource`): the plugin's icon
(`SourceIconView` — `icon_svg` as a template, else the SF Symbol), the
plugin's **title**, never the project's name (nothing renames the section),
and the header `menu` (the `sidebar` response's where it sent one); then the
plugin's groups (one level of children), container rows (`SidebarContainer`
— the stacked-card glyph with tasks under it, `SourceGlyph.emptyContainer`
(one card) with none; badges, each one fixed width (`SourceBadgeView.width`,
three mono characters, longer text shrinks); under the pointer the `meta` and
a `+` centred in the badge's slot, hiding them, so nothing shifts; `menu`,
Open in <source>) and
each container's tasks, the
plan's slices whose `milestoneID` is the container's id, through
`displayState(for:)` like every row (Hide Done applies; a container in two
groups is one container). A lazy group's fold is `AppModel.sourceExpanded`,
passed on every read as `info --expand` (`ProjectStore.expand`). Plugin
actions run through `AppModel.runSourceAction` (`text` asks in a sheet,
`choice` is a submenu; `filter` is no menu item (`[SourceAction].menuItems`
drops it) but `SidebarView.filterButton` beside the ellipsis — always on the
header, on hover on a segment's row, filled in the accent while
`SourceAction.isNarrowing` — opening `SourceFilterPopover`, a `.popover`
anchored to the button, its single-choice fields menus wrapping an inline
picker (a `.menu` picker builds every option before the popover can show:
~0.2 s for 400 epics, ~1 s for 2,000), its choices a
`SourceFilterDraft`, "Any" naming what it falls through to, a `loading`
field read once more through `AppModel.rereadSource` — `destructive` is
confirmed), then re-read the plan. Every section but Active, folded, pins to
the sidebar's foot under the open ones (`SidebarView.foldSlots`); open ones
take at most their natural height and share the room only when short of it.
Active rows and the titlebar carry the plugin's `tag` (`sidebarTags`). A
container is a third selection kind (`selectedContainerID`, exclusive with
slice, session and workshop); `ContainerStore` caches `container-show` per
project and, like the PR screen, keeps a stale reading on a failed re-read.
`ContainerNavigatorModel` decides the sections (the first prose section's
facts and tasks, then comments/links; unknown kinds skipped) and
`ContainerFocus` what is open and what `ContainerPane` shows. A task under a
container shows the container and its `facts` in place of the milestone,
the PR section its `task_note`, and the breadcrumb `<container> / <task>` —
no project or group (segment) crumb, a selected container its own crumb
alone; `CrumbTree`'s middle column for a source project is its containers
(each once, in fold order), no group column. Stories:
`crumb-tree-picker-source`, `titlebar-band-source-task`,
`titlebar-band-container`.
`DesignTokens.wireTint`/`wireBadge` are the one place a plugin's `#rrggbb`
becomes a `Color` (as a hue through the palette's rules). Stories:
`sidebar-source`, `sidebar-source-error`, `window-container`,
`window-container-links`, `window-source-task-brief`,
`window-source-task-pr`, `sidebar-source-hover`, `sidebar-source-projects-open`,
`sidebar-source-folded`, `sidebar-source-all-folded`, `sidebar-source-segment-hover`,
`source-filter-popover`,
`source-filter-popover-section`, `source-filter-popover-loading`.

## The diff is AppKit, laid out exactly

The continuous diff is `DiffCanvasView` (NatKit), not a SwiftUI stack: a
lazy stack estimates the heights of rows it hasn't drawn, so the content
height and every offset shift as it scrolls (jumps at file boundaries, a
jumping scroller, a jump-to-file that lands wrong). `DiffLayout` computes
every row's height up front from its columns (`DiffText` — layout and drawing
wrap through the same code, so they can't disagree), and a viewport-sized view
draws only what's visible over a sizer document. Comments and the editor stay
SwiftUI, hosted per anchor row (`DiffCanvasRepresentable`). Don't move the
rows back into SwiftUI. Stories: `diff-stress`, `diff-stress-unwrapped`.
The gutter is one number column (the branch's side; a removed line's is
blank) and there is no +/- column — a row's fill, and its gutter stripe,
say what changed. A task's diff is read `expandable`: every gap around its
hunks is a `hunkBreak` row carrying a `DiffGap`, whose controls (GitHub's ↓
↑ ↕) sit in the gutter; `DiffStore.expand` reads the lines through `nat
slice-file` and `DiffFileModel.revealing` puts them back as context rows
numbered as git would have. Story: `window-review-expanded`.

## Design tokens

Every colour is a named, dynamic token in `DesignTokens.swift` (values in
`Palette.swift`), never a bare `Color(hex:)`/`(nsColor:)` at a call site
(`ColorSourcesTests` enforces this). The palettes are named — `light` (the
design's `gnat.css` light tokens), `oneLight` (the light default),
`tokyoDay`, `iceberg` (the dark default) and `slateInk` (community themes
laid onto the same roles) — and listed by `PaletteChoice`. `Theme`
picks dark, light or system; the user's dark-slot and light-slot palettes
(two `UserDefaults` keys, Settings ▸ General) say which palette each scheme
draws with, through `PaletteSelection`, which every token resolves against.
A palette pick rebuilds the window's content (`NatApp`'s `.id`): SwiftUI
keeps colours it resolved per appearance, so a palette change that is not
an appearance change repaints nothing otherwise. Adding a palette is a
`Palette` static plus a `PaletteChoice` case; `PaletteTests`/`PairingTests`
hold every case. Render one with `gnat --palette <id>`. The fonts are the
app's own (system sans, Fira Code), not the design's.
