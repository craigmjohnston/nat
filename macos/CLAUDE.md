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

## NatClient: `nat` is the only source of truth

- `NatKit/NatClient/NatClient.swift` shells out to `nat <command> --json`.
  **Never reimplement tracker logic in Swift** — state, readiness, blocking,
  migration, write ordering all live in the Go binary.
- Every call on a tracked project passes `--project <id>`, no fallback,
  mirroring the Go CLI (exceptions mirror the Go CLI's own: `status`,
  `paths`, `config-show`/`-set`, `project-create`/`-open`, `source-list`).
- Task sources: `SourceModels.swift` (and `ProjectInfo.source`,
  `SliceDetail.container`, `PlanBackend.source`) mirror
  `docs/design/task-sources/README.md` field for field — change the spec and
  the models together. Settings ▸ Sources (`source-list`) is the only view so
  far; the sidebar/navigator for source projects is the next milestone.
- `NatBinary.resolve` never falls through to PATH for a packaged app:
  `NAT_BIN` (dev override) → the binary beside the app executable. No
  bundled `nat` is a **damaged install**, reported as such; only a bare dev
  executable outside any `.app` searches PATH.
- `PathBootstrap.bootstrap()` composes PATH at startup for the **agent tmux
  sessions nat spawns**, not for finding `nat` itself (`NatBinary`'s job).

## Settings and Done means merged

`SettingsView` (⌘,) matches System Settings/Safari's shape (toolbar tabs,
grouped stock forms), not the app's own chrome — reads `nat config-show`,
writes one `nat config-set <key> <value>` per changed key. `WorkflowStage`
(`stage(for:)`) is the one source of where a slice stands: the navigator's
phase (`NavigatorModel`), the sidebar's dots (`displayState(for:)`) and
`RailModel.isReviewSlice`/`isActiveSlice` all read it, and it mirrors
Go's `domain.StateOf` (Notion's status is the one source of lifecycle truth,
so a Done slice is never in-flight even with `nat pr-status` still reporting
its PR open) — change `internal/domain/state.go` and the stage together. A
live session never moves the stage; `fixing` comes only from
`AppModel.fixLaunched`.

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
the sidebar's own rows) and the sidebar shows nothing of it. Accept is `nat plan-accept`: on an
Untitled tab it makes the project, then the session is killed and
`addProject(replacing:)` hands the tab over; on a project it files the plan
(`--project`) and leaves the session running. The layout is one for both:
the navigator's Brief (the request; Launch, then End session) over Plan (the
proposal, Accept and Keep workshopping — absent until there is one), and the main pane, with no tabs, the
brief editor before launch and the terminal from launch on. Opening a
workshop pins its row in Active (`workshopPinnedProjects`) until a launch or
the row's ✕. Stories: `workshop-*`, `window-workshop*`,
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
navigator (`TitlebarBand`) carries its tabs, then at the trailing edge the live
agent's model, effort and context, kept small with the long form as a tooltip
(the status bar carries none of these), or the view's actions and selects (the
diff's commit switcher, Open in GitHub) — each pane hands these up as a
`*TitlebarTrailing` view; the PR's title heads the PR view's own body. The Thread ends, while the slice can be launched, on a
ghost `LaunchCard` (greyed and hatched when blocked); its prose items cut
short as the brief does (`Excerpt`). View ▸ Hide done items
(`showsDoneItems`) drops done slices, ended sessions and the Done folder from
the sidebar. `AppModel` keeps its one *active* project —
every per-project reading is keyed by it — and the sidebar selects across
projects by activating first (`selectSlice(_:inProject:)`). The Thread shows
only what nat reports (`buildThreadEvents`). The titlebar is two bands:
the sidebar's holds Settings and the `+` (anything the sidebar makes, its
project asked for by submenu); one band over the navigator and main pane, no
rule at the split, holds the selection as its Active row names it
(`TitlebarIdentityLabel` over `ActiveIdentityLabel`: dot, project tag, title,
read through `AppModel.titlebarIdentity`) from the navigator's inset, free to
run past the navigator's width and ellipsize, then the `MainPaneTab`s and the
trailing items at the right — Zed-style tabs, full height and square, one per
section that would put its view up (`NavigatorModel.tabs`,
`MainPaneTab.forSession`). The tabs and trailing items live only in the main
pane's part of the band (`TitlebarBandLayout`), cut at their leading edge
rather than crossing the split; a tab is
`NavigatorFocus.showing`, which opens and never folds. The selection's name, and a slice's
project and milestone crumbs in the status bar, each open `CrumbTreePicker`
(projects → milestones → slices, `CrumbTree`) on themselves; with nothing
selected there is no breadcrumb. The Thread is labelled "Task" until the
slice is under way and "Task log" after, and draws `slice-show`'s `events`
in order — hand-backs, send-backs (`slice-rework --comments`), releases,
relaunches, follow-ups (triaged ones as a record, pending ones as the triage
card in their place), then approve and merge. Selecting sets the selection *before* awaiting the project's
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
(`MainPaneTab.visuals`; no trailing actions, zoom being per image).
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
app's own (system sans, JetBrains Mono), not the design's.
