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
- Context menus that stop opening: `docs/debugging/context-menus.md` (the
  `NatMenuDebug` default / `NAT_MENU_DEBUG` trace, what is ruled out).

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
`SettingsTab` rows under a "Settings" heading (the window has no title bar,
the traffic lights over the sidebar; tile + name, each tile a fixed
`DesignTokens.tile*` gradient whatever the palette, the selection filled in
the accent; About apart under a rule) beside the section's groups (bold heading,
`settingRow`s left-aligned under it), one fixed 760×560 window whose
sections scroll — reads `nat config-show`, writes one `nat config-set <key>
<value>` per changed key. Sections: General, Agents, Sources, About. About
reads `Bundle.main` (`AppVersion`, `dev` where unset) and `nat --version`
(`NatClient.natVersion`). Stories: `settings`, `settings-agents*`,
`settings-sources*`, `settings-about`. **Per-project settings are not
here**: the project menu's Project settings… (a project row's right-click
or its hover-only three-dot) opens `ProjectSettingsView`, a sheet on the
main window titled with the project's name — one grouped `Form`, no
sidebar or tabs — holding the working directory (field + Choose…). Its
logic is `ProjectSettingsModel` (NatKit, tested): the same one `config-set`
per changed key (`SettingsModel.workingDirKey`), a refusal kept under its
row with the baseline as read, `AppModel.reloadConfig` after any write so
Reveal and launches use the new path at once. A further per-project row is
a `ProjectSettingsFields` field and a row in the sheet. Stories:
`project-settings`, `project-settings-refused`. Project, milestone and task
rows in the sidebar tree each carry that hover-only three-dot
(`RowMenuButton`), opening exactly the row's right-click menu — beside a
project's `+`, in a milestone's count slot, in a kept slot at a task's
trailing edge (stories `sidebar-project-hovered`,
`sidebar-milestone-hovered`, `sidebar-slice-hover`). `WorkflowStage`
(`stage(for:)`) is the one source of where a slice stands: the navigator's
phase (`NavigatorModel`), the sidebar's dots (`displayState(for:)`) and
`RailModel.isReviewSlice`/`isActiveSlice` all read it, and it mirrors
Go's `domain.StateOf` (Notion's status is the one source of lifecycle truth,
so a Done slice is never in-flight even with `nat pr-status` still reporting
its PR open) — change `internal/domain/state.go` and the stage together. A
live session never moves the stage. In progress reads resumed → `working`
(nat's `resumed` on `info --json`, `Slice.resumed`, **never re-derived**: a
project with no Branch column holds a PR and no branch legitimately and stays
`pr`), else a PR → `pr`, else handed back → `review`, else `working`.

**Resuming: Send back to agent.** A handed-back slice — in review, or at its
open pull request — can go back to its agent for more: the action bar's
secondary **Send back to agent** (`NavigatorModel.showsSendBack`, enabled
with a live agent or where `LaunchPlan` can launch one), which opens
`SendBackEditor` over the bar (drawn in the column, not a popover, so the
gallery renders it): what to change, prefilled with the PR's own trouble
where it has any (`sendBackReason`: failing checks, a conflict). Sending is
`AppModel.sendBack`, the one-shot `.sendBack` (no stage advance; the view puts
the terminal up once it has gone): **the record first** — `nat slice-resume
--note -` (stamped `Resumed`, then the Branch cleared) — then a live agent is
told by `agent-send` (`sendBackPrompt`, ending in the `complete-slice
--branch` hand-back on the branch the slice had), else the ordinary `nat
slice-launch` (a relaunch). There is no fix launch and no `fixing` stage:
`LaunchPlan` treats an In progress slice with a PR as an ordinary relaunch
(dependencies and all, as nat's own does) and refuses Done. A resumed slice is
`working` (Active's working half, its dot the accent, pulsing while live),
its phase `.thread` — selecting it opens the Task log and the terminal — and
neither Approve nor Merge is offered until the next hand-back (the bar's
greyed stand-in is Launch). Its Changes, Visual changes and PR stay
(`NavigatorModel.hasBranch` counts `resumed` and `takenBack`: `slice-diff`/`slice-file` read
its agent branch), each header wearing **Reworking** (`NavigatorModel.showsReworking`,
`worksAgain`): a warning-toned small `Chip` with `arrow.triangle.2.circlepath`,
before any New/Updated, its tooltip `NavigatorModel.resumedNotice` — no
callout in the section bodies — and a `MainPaneNotice` across the top of
its main pane. A review sent back before any PR is not `resumed` (that needs a
PR) but nat's `taken_back` (`Slice.takenBack`: In progress, Branch cleared on
a project with a Branch column, a Handed back on its log; true for a resumed
slice too) — it keeps Changes and Visual changes with the same badge, and
moves no stage or PR gate: `resumed` alone drives those. Story:
`window-taken-back`. Changes' Send goes to a taken-back slice's live agent too
(`showsChangesSend`), and `DiffStore.sendComments` — like `VisualStore`'s —
asks for the hand-back and runs `slice-rework` only where the slice is
handed back. Stories: `window-resumed`, `window-resumed-notices`,
`window-task-log-resumed`, `window-resumed-pr-open` (sent back from its
open PR mid-story: Task and the terminal up, the PR section folded and kept),
`window-pr-send-back`,
`window-pr-send-back-prefilled`, `action-bar-send-back-and-merge`.

**New and Updated.** One rule, one store: `SeenMemory` (UserDefaults
`seenSnapshots`, per project, slice and `SeenSection`; `.inMemory()` for tests
and stories) remembers each section as item → fingerprint as last seen. No
snapshot — never looked at — badges nothing, and the first reading records
everything (`baseline`); then an item the snapshot lacks is **New**, one it
holds at another fingerprint **Updated** (`seenBadge`); seeing an item
(`markSeen`) records it, taking its badge off; a reading prunes items gone
(`retain`), so one that comes back is New. Changes (`DiffStore.badge`): a
file by path at `DiffFileModel.seenFingerprint` (sha256 of its rows, line
numbers aside), seen once its rows are on screen in the diff
(`DiffCanvasActions.filesShown`, `DiffLayout.shownFiles`) or marked viewed;
badged on its navigator row and its diff header (`DiffCanvasState.badges`).
Visual changes (`VisualStore.badge`): an image by name at its `identity`,
seen on screen or marked viewed — nat's `changed` no longer badges. PR
(`PRStore.badge`): the head (`pr-view`'s `head_ref_oid`) as last seen,
Updated while it has moved, seen when the PR section is open or its view up.
Each section header takes `NavSectionStatus.of` its items (New over
Updated; `SeenBadgeChip`, New in Merged's green, Updated in the accent).
Stories: `window-resumed-badges`, `window-pr-updated`, `window-visuals-new`.

**Failing checks.** `PRStatusStore` holds `pr-status`'s reading **per project, for every open
project** (`PRReading`: readiness, failing checks, conflicts) — the active
one's taken with its plan (`updateReviewStats`), each background one's after
its plan lands (`loadBackgroundProject`, `refreshBackgroundProjects`), all
skipped where no slice has a PR or stands in review (`inReview`). A project's reading is replaced only by a
newer reading of it (a failed one leaves it standing; switching projects
touches nothing), written beside its plan in the read cache
(`PlanCaching.writePRStatus`, `<id>.pr-status.json`) and restored before the
first fresh read (`restore`). Given a `Cadence` (`NatApp` passes
`prStatusFastInterval: 10 s`; tests and stories none), the store also reads
every open project on its own loop, whatever is on screen: each reading, by
whoever asked, schedules the next — **fast** while an open PR's checks are
`pending` or a live agent sits on a slice with an open PR, else **slow**,
the plan poll's cadence — one sleeping task per project, skipped where the
plan has nothing to read (`shouldRead`), stopped by `forget` (a closed tab)
and `stop`; a second `update` joins the one under way, and a reading equal
to the last publishes nothing. `PRStatusStore.marks` (by slice id) puts
`PRMarks` on **both** sidebar row kinds — `SidebarActiveRow.marks` and
`SidebarSliceRow.marks`, pr stage only (`atPullRequest`) — drawn by
`PRMarksView`: the checks' `xmark.octagon.fill` and the conflict's own
`ConflictMark` (`MergeIcon`, "Conflicts with <base>" / "Merge
conflicts"), which takes a `BranchConflict` and nothing about a PR, for a
conflicting branch with no PR to reuse — and, in the checks' slot, the
success mark (`checkmark.circle.fill`, "Checks passing") where
`prMarks(_:for:agent:)` keeps `checksPassing`: the `.pr` stage (not
resumed), no live agent working, verdict `passing`, not conflicting or
failing — and, under the same gate, the running mark (`checksRunning`,
verdict `pending`: a static, neutral `PRMarks.runningSymbol` —
`ellipsis.circle.fill` — "Checks running"). The PR section header draws the
same gate as its outline `checkmark.circle` (or `ellipsis.circle` for
running) where it has no warning (`NavSectionView.passing`/`running`). The
Checks block's rows lead with the same marks (`CheckRowMark`): a running or
queued check the running mark, a skipped one `slash.circle`, its line faded
and its name struck through.
`attention(projectID:)` reads the
project's own reading. In the navigator, `checksNotice` is the PR section
header's danger icon (`NavSectionView`'s `warning`, its text the tooltip;
"sent to the agent to fix" when the latest recorded event is the nudge's
Sent back). A conflict is never a callout in a section body: it is a
**Conflict** badge in the header (`NavSectionView.conflict`: a small danger
`Chip` with `MergeIcon` before `NavigatorModel.conflictLabel` — the sidebar
mark's glyph), its tooltip the notice's text, pointing at Send back to agent
or naming the live agent (both prefill Send back's note). The PR header's is
`conflictNotice` (the reading's conflict, unless a loaded `PRDetail` of that
PR decides — `conflict(reading:detail:prURL:)`; drawn before `pr-view`
lands), "merge <base> in". `projectAttention` counts a red pr slice once, and only
with no live agent on it (see **The dock**).
**A hand-back with no PR**: `PRStatusDoc.branches` (nat's own merge test of
the branch; absent where it could not test) gives
`PRReading.branchConflicts`, merged into `marks`; `prMarks` draws a slice
`inReview` with its conflict alone, and the Changes header wears the badge,
its tooltip `branchConflictNotice`'s — "rebase it on <base>"
(`ConflictNotice.hasPullRequest` false) — and Send back's prefill says the
same; a launch then carries nat's rebase passage. Stories:
`window-review-conflicting`, `window-review-conflicting-send-back`,
`sidebar-checks-failing`,
`sidebar-pr-marks`, `sidebar-pr-marks-passing`, `window-pr-checks-passing`,
`sidebar-pr-marks-running`, `window-pr-checks-running`,
`window-pr-checks-failing`, `window-pr-checks-agent-told`,
`window-pr-conflicting`, `window-pr-conflicting-checks-failing`,
`window-task-log-checks-failed`.

**Re-running and cancelling checks.** The PR section's Checks block
(`PRSectionBody`, `ChecksControlsView.swift`): each check row — a sidebar task
row's height, its outcome glyph in the tree's dot column — ends in a re-run
(`arrow.clockwise`) and a cancel (`xmark`) icon button, drawn only under the
pointer or while that row's own call is under way (hidden in place, so nothing
shifts), and the Checks heading
(`ChecksHeading`) in the list form of the pair — `ListActionGlyph`, drawn
because SF Symbols on macOS 15 has no such pair: `checklist` with the row
glyph in its lower right, the list cut a point round it as `DoneFolderGlyph`
cuts its check (Lucide's `list-restart`/`list-x` shape). All four sit in fixed
`CheckControlSlot` columns at the trailing edge. Heading re-run is a menu: Re-run
all, Re-run failed. Row re-run is `slice-checks-rerun --check`, row cancel
`slice-checks-cancel --check`, heading cancel `slice-checks-cancel`; nat cancels
a running run before re-running, so nothing is disabled for running. What is
enabled, and each tooltip's list of the siblings a call would stop (same `run`,
still queued or running), is `ChecksControls` (NatKit, unit-tested) from the
checks' states, `rerunnable` and `run` (`pr-view`'s new fields). The heading's
buttons are absent where no check is `rerunnable`, and so are all of them on a
session's PR (`checksStore` nil). A row is washed full bleed under the pointer
(`gnatRow`, padded out by NavProse's 12 and back). `PRStore.rerunChecks`/
`cancelChecks` run one call at a time (`checksActionSource`: its button a
spinner, every other disabled), then re-read the PR and show nat's report
(`checksActionNotice`: "Cancelled …, then re-ran …") or its refusal as the
body's `NavNotice` (`checksNotice`, cleared on another slice). No confirmation.
Stories: `window-pr-checks-controls`, `pr-checks-controls`,
`pr-checks-row-hovered`, `pr-checks-nothing-run`, `pr-checks-mid-call`,
`pr-checks-cancelled-then-reran`.

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
up, a slice row scrolls it (`showProposedSlice`) and unfolds it — each box
folds to its header, drawn as a diff file header is (its title in the
Changes file rows' sans), on a click
(`foldedProposedSlices`) — and ⌘1/⌘2 switch the two
while the workshop is on screen (`WorkshopMenuActions`). Opening a
workshop pins its row in Active (`workshopPinnedProjects`) until a launch or
the row's ✕; the row is "Workshop", with the `wand.and.stars` glyph in the
state dot's place (`workshopSymbol`, the crumb too). While its tab has a
proposal up the row wears a green ✓ **Plan ready** badge before the ✕
(`SidebarActiveRow.planReady`, set by `buildSidebarModel` from
`proposedWorkshops` — `AppModel.proposals`' keys — never by the view; stories
`sidebar-workshop-plan-ready`, `-hovered`). Pins, drafts, attached
plan files, launched requests and the open Untitled tabs (with their
workspace ids) are kept across relaunches in `workshops.json`
(`WorkshopCaching`: `DiskWorkshopCache` only in `NatApp`, in memory
everywhere else), restored once config is read, written debounced and on
quit (`flushWorkshops`). Stories: `workshop-*` (`workshop-proposal-scrolled` the Plan tab scrolled, `workshop-proposal-folded` boxes folded), `window-workshop*`,
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

## The dock

`attentionItems` (`ProjectAttention.swift`) is everything waiting on the
user in a project — one `AttentionItem` per slice (or session, or planning
agent), under its most urgent `AttentionKind`: waiting, review, checks
failed, conflict, ready to merge (the sidebar's `checksPassing` gate). A
pull request's failing checks or conflict never count while a live agent is
on the slice — nat has already sent it the failure — so nothing that clears
on its own ever shows. `projectAttention`'s count is its count.
`AppModel.dockAttention` is every open project's items (computed, observed
through the same stores; no poll). `DockAttention` (NatApp) badges
`NSApp.dockTile` with the count, builds the dock menu on demand
(`AppDelegate.applicationDockMenu`, `dockMenuSections`: a heading per kind,
"<tag> <name>" rows that select through `AppModel.select(_:)`) and bounces
once (`.informationalRequest`, never while active) when
`AttentionChange.arrivals` finds an item by identity the last reading did
not have. No story can render the dock.

## Closed tabs stay closed

`AppModel.closeProject` records a closed project's ID in `ClosedTabMemory`
(UserDefaults; `.inMemory()` for tests and stories) once the tab has gone —
never on a refused close, never for an Untitled tab. It is app state only:
the config entry stays, `nat` knows nothing of it. `start()` leaves recorded
projects out of the strip (never scratch; all ignored for the launch where
they would leave no closable tab) and drops any config no longer names. Every
path that puts a config project's tab back (`addProject`,
`ensureSourceProjects`, `projectMirrored`) forgets its close. The "+" tab's
sheet offers the closed ones first (`AppModel.closedProjects`, read off
config, not `project-list`) and opens one straight into `addProject`, writing
nothing. Tests: `ClosedTabTests`.

## Release build quirks

- Bundled `nat` and `gnat` itself are both **arm64 only** — Intel is not
  supported (`GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w …"`;
  `swift build --arch arm64`). Both are stripped; gnat is stripped in
  make-app.sh, before release-app.sh signs, and its unstripped executable
  is kept as `.build/gnat-<version>-unstripped` and published with the
  release. If x86_64 ever comes back, build each arch on its own and lipo —
  never `swift build --arch arm64 --arch x86_64` in one call, which routes
  through XCBuild's cross-arch path and can't resolve SwiftTerm's build-tool
  plugin (swiftlang/swift-package-manager#7442). Needs the Go toolchain too.
- The icon ships twice: `Assets.car`, compiled by make-app.sh's `actool`
  from the layered `Resources/AppIcon.icon` (`CFBundleIconName`) — what
  macOS 26 draws, light or dark by the system's own setting, app open or
  not — and the two icns for anything older, where `NatApp.setDockIcon`
  swaps them by hand while the app runs. On 26 a bundled app leaves the
  dock alone. `make-icon.sh` renders both from the same two SVGs.
- Of the icns, only `Contents/Resources/AppIcon*.icns` ship: SwiftPM's
  `nat_NatApp.bundle` (the same two icns, for the bare `swift run`
  executable) is not copied in, as nothing reads it through `Bundle.module`.
  Copy it back if NatApp ever does, or the generated accessor traps.
- `tmux`, `gh`, `ntn` are never bundled — the machine's own install.

## The window

The gnat hi-fi design (Claude Design project `e81457f6-…`, `gnat.html` with
`gnat-data/shell/nav/main.jsx` and `gnat.css`) is the spec: `SidebarView`
(Active across every project, the Projects tree, then the scratch project
as a Scratch fold of its own — `SidebarModel.scratch`, whose unfiled
milestone's slices, `Milestone.unfiled`, sit loose at its head), the navigator's
stacked Thread/Changes/PR foldouts (`SliceNavigatorView`, with
`NavigatorModel` deciding phase, liveness and header actions). The brief is
the Thread's first item, not a section. A header click puts its section's
view up in the main pane — Thread the terminal, Changes the diff, PR the
description and conversation (the PR section keeps checks and review) — and
folds it again when that view is already up; the chevron only folds
(`NavigatorFocus`). Folded bodies stay built, so unfolding reloads nothing.
The main pane has no heading band: the titlebar band over it and the
navigator (`TitlebarBand`) carries the breadcrumb, its tabs and a
handed-back slice's run button, and nothing else. The live agent's model, effort and context — a slice's, a session's,
the planning agent's; none for a container — are the status bar's trailing
item (`AgentModelHeading`, in the bar's own sans, a divider before the context clause, the long form as a tooltip). A
slice's major actions — Send back to agent, Launch agent / Relaunch agent,
Approve changes (Approve with comments while comments are pending on the
diff, opening the same confirmation), Merge PR — live only in the **action
bar** pinned to the slice navigator's foot (`NavigatorActionBar`, a
`NavigatorColumn` footer: header-band height, chrome, a top rule, no title,
fold or body; Send back's editor opens over it). `NavigatorModel.bar` decides it: each action only while
relevant (absent, not greyed, otherwise; disabled where relevant but not
pressable), the primary trailing; with none relevant, the latest live
section's primary (Merge PR, else Approve changes, else Launch agent)
greyed with a tooltip saying why; a Done slice just "Task completed". The
Slice menu's Launch and Merge are nil exactly where the bar's button is absent
or disabled. Section headers keep secondaries only: Send N comments in
Changes and Visual changes, Open in GitHub in the PR head, a container's Open
in <source> in its Story head before New task (both `HeaderLinkButton`,
glyph-only where the head has no room); the Task head carries nothing. The
session, workshop and container navigators have no bar — their actions stay
in their headers. Other view actions live in the section whose view they act
on: the diff's commit switcher (`DiffCommitsMenu`) a row atop the Changes
body; the workshop's launch shortcut is in the brief editor's placeholder. The PR's title heads the PR view's own body. The Thread ends, while the slice can be launched, on a
`LaunchCard` item — what Launch will do, model and effort as chips, the base;
quietened, chips disabled, when blocked — with Launch itself only in the
action bar; its prose items cut short as the brief does (`Excerpt`). View ▸ Hide done items
(`showsDoneItems`) drops done slices, ended sessions and the Done folder from
the sidebar. Every "merge" icon is `MergeIcon` — the Merge button's own `MergeGlyph`,
never `arrow.triangle.merge` (the Task log's Merged item through
`ThreadIcon`, the conflict mark). A sidebar milestone row (and the Done and Ad hoc sessions folders)
folds as a project row does: washed under the pointer, its folder giving way
to the chevron (`TreeMilestoneLine.folds`; story `sidebar-milestone-hovered`). `AppModel` keeps its one *active* project —
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
navigator's width. As room runs out the selection's name is kept longest
(`BreadcrumbFit`, from widths the breadcrumb measures): it ellipsizes to
80% of itself, then the project crumb turns into the project's tag, then the
milestone (or container) ellipsizes to half of itself, and past that the
breadcrumb gives way to the Active row's line alone — dot, tag, name
(stories `titlebar-band-fit-*`). Then the `MainPaneTab`s at the
right — Zed-style tabs, full height and square, one per section that would
put its view up (`NavigatorModel.tabs`, `MainPaneTab.forSession`,
`WorkshopTab.available`, which say which exist) — **filling from the right**
(`TitlebarBandLayout.leftToRight`): the first rightmost, so a full slice
reads PR, Visual changes, Changes, Terminal and Terminal never moves; then,
rightmost of all, a handed-back slice's run button (`TitlebarBand.trailing`),
the rightmost tab closing its trailing edge with the tabs' 1pt line only
where it is there (`MainPaneTabButton.closed`). The breadcrumb ends at least
`GnatMetrics.breadcrumbGap` (20pt) short of the tabs or run button at every
fitting stage, its room measured inside that gap.
Run button and tabs live only in the main pane's part of the band
(`TitlebarBandLayout`, the run button taking its width first), cut at their
leading edge rather than crossing the split; a tab is
`NavigatorFocus.showing`, which opens and never folds. The project, milestone
and container crumbs and the selection's own each open `CrumbTreePicker`
(projects → milestones → slices, `CrumbTree`) on themselves; with nothing
selected there is no breadcrumb. Stories: `titlebar-band-*`,
`status-bar-agent-readout*`, `changes-section-commits`, `action-bar-*`. The Thread is labelled "Task" whatever the
slice's state, and draws `slice-show`'s `events`
in order — hand-backs, send-backs (`slice-rework --comments`), releases,
relaunches, work resumed (`slice-resume`: "Work resumed", why as its body,
its stamp as its time; the hand-back that ends it the ordinary card after
it), notes (`nat slice-note`, headed "Another agent left a note";
`fromSlice` matched once against the loaded plan by name and milestone
name — `noteSourceSlice` — is a `task` fact drawn as the brief's
`DependencyRow`, through `ThreadEventCard.taskRow`, else `source` with the
provenance as text; a slice never launched shows its notes alone, and
notes ahead of every other recorded event sit before Launched),
follow-ups (a proposal is its count line, then one item per decided
follow-up — headed "<Queued | Folded in | Dismissed> proposed follow-up"
(`followUpDecisionHeading`), its title then its brief as the body, a queued
one's slice as a `task` row; a proposal still pending is its batch's own
triage item in its place — `FollowUpCards`, drawing only the items
`pendingFollowUps(batch:in:)` pairs with it (`slice-show`'s `followUps` and
the event share a `batch`), with its own Discard all and Apply; a proposal
awaits triage only where nat lists items of its batch, so a Done slice's
never-triaged batch draws as a record. `FollowUpStore` keys choices (by
place in the batch, not nat's index, which moves as other batches are
decided), the apply in flight and the error by slice **and batch**; any
batch's apply holds every card of that slice through the re-read that
follows, so none applies stale indexes. Stories: `window-followups`,
`window-followups-two-batches`, `window-followups-decided-and-pending`),
then approve and merge. No item is boxed: each is a `LogItem` — its icon in a margin
column, who and its meta (in its tone) as the header — and a rule runs down
the margin from one icon to the next (`LogConnector`, chosen by
`threadBody`, which knows the sequence): none after the last, dashed after it
while an agent is live. Something happening now or awaiting the user
(`ThreadEvent.isLive`: the live agent, a pending proposal) has its icon in
its hue; every key column takes the
width of `widestThreadFactKey` (`ThreadFactKey`), and the brief's Edit is
drawn only while the slice is Todo. Each item with an `at` shows it at
its header's end (`threadTimestamp`: time today, `d MMM` this year, `d MMM
y` before) — a decided follow-up its `decidedAt`, the triage item its
proposal's `at`; Launched the recorded `launched` event's, where nat recorded
one (none for a slice launched before it did, or claimed by hand); the live
agent, approve and merge have none. The Thread offers Relaunch only where
that launch is recorded and no agent is live (`launchIsRelaunch`), else
Launch. The quiet items — notes, blocked, a triaged proposal and its
decisions (`ThreadEvent.isCollapsible`) — draw folded to icon, title and
time, a click on the header row opening one and its icon a chevron under the
pointer; three or more in a row fold into one group (`threadLogItems`:
stacked icon, "N other items" in italic, `threadTimestampRange`), which
opens onto its items, each folded, in one recessed well (`.rowAlt`, rounded,
inset from the log's rule — `LogConnectorRule` runs on past it unbroken —
its items a step in from the header), its last line "Hide N items"
(`threadGroupFoldTitle`, the header's italic secondary, lit under the
pointer), which folds the group and scrolls back to its header where that
had gone off screen. `threadFoldsOpen` (`StorySeams`) opens them for a story. Stories:
`window-task-log-notes`, `window-task-log-note-todo`, `window-task-log-folds`,
`window-task-log-folds-open`, `task-log-fold-hover`. Selecting sets the selection *before* awaiting the project's
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
`DiffReview`: images loaded through a swappable `loader` (local paths
only — any other URI is a placeholder card, no network) and **cached by URI +
hash** (`VisualChange.imageKey`/`beforeKey`), so a re-render saved over the
same path loads afresh — the views' load `.task` is keyed by
`VisualChange.loadIdentity` for the same reason, and images no slice's
hand-in still names are dropped. Zoom per image, comments per slice at a
point in the image's own pixels or on the whole image (dropped when the image
they sit on is re-rendered), and viewed/folded marks keyed by
`VisualChange.identity` (name + image + before), following
`DiffStore.toggleViewed`'s rule (viewed folds; a re-render starts afresh).
**New / Updated** (see **New and Updated** above): New for an image by a
name not seen before, Updated for one handed in again with other content —
seen once its image section is on screen in `VisualsPane`
(`onScrollVisibilityChange`) or it is marked viewed; drawn on the navigator
row, the image header and the section header. **Pairs** (`before`) are one row
(the after's thumbnail) and one section: `VisualCompare` (NatKit, tested)
owns the divider fraction (0 after whole, 1 before whole, middle to open),
the Before / After toggle selected only at an end, the frame (larger of each
dimension, top-leading, one scale) and the difference mask (computed off the
main actor, cached by the two keys; refused with a tooltip where sizes differ
or an image is unavailable); `VisualStore` holds divider, highlight and mask
beside zoom. Comment pins stay in the after's pixels; an unavailable before
draws the after alone, pair controls disabled. `VisualDivider` wears
`.columnResize`. Send is `agent-send`, then `slice-rework` only where the
slice is handed back; a failed send keeps the comments. The comment box is
drawn in the pane, not a `.popover`, so the gallery can render it — at a
point, it is the pane's one floating overlay: the image section draws only
its pin and publishes the pin's anchor (`VisualDraftPinKey`), and
`VisualsPane` resolves it over the whole vertical scroll and floats the box
where `VisualEditorPlacement` (NatKit, tested) puts it — below the pin, else
above, never over it, 16pt inside the pane, held at the edge while the pin is
scrolled away — at its measured height, adding nothing to the scroll's
content. A comment on the whole image keeps its box at its section's top
trailing corner.
`VisualsPane` is the one scrolling pane SwiftUI lays out (pinned headers,
`ScrollViewReader`) — safe only because nothing draws until every image's
pixel size is known, befores included, and every image has an explicit frame;
keep it so. Stories: `window-visuals`, `window-visuals-comments`,
`window-visuals-new`, `visuals-zoomed`, `visuals-comment-editor`,
`visuals-comment-editor-image-foot`, `-zoomed-edge`, `-pane-foot`,
`visuals-pair`, `visuals-pair-highlight`, `visuals-pair-before`,
`visuals-pair-size-mismatch`.

**Run commands** (`docs/run-commands.md`): a project's `runs` live in its
config entry alone (`ProjectConfig.runs`, `RunCommand`) — no settings screen.
The titlebar's play button (`TitlebarRunButton`, beside Settings) opens
`RunTreePicker`, `CrumbTreePicker`'s shape — every project with runs
(`AppModel.runProjects`), then the open one's runs; a handed-back slice's
`RunSplitButton` is the titlebar band's trailing item (`WindowShellView.sliceRunButton`),
greyed once the stage is done — `▶ <label>`, glyph first (unlike the shared
`HeaderActionLabel`, which it composes its own label instead of), the
spinner in the glyph's fixed slot, the words at `GnatMetrics.titlebarText`.
Both call `AppModel.startRun` → `nat run`; nothing in Swift picks a directory
or default. No tab or pane opens on a run: its session is held in
`AppModel.runs` until tmux says it is gone (`watchRun`,
`TmuxSession.exists`), and the button spins meanwhile
(`AppModel.isRunBusy`; the titlebar's `anyRunBusy`). Stories: `titlebar-run`,
`titlebar-run-menu`, `window-run-heading`, `window-run-heading-merged`,
`titlebar-band-run`, `titlebar-band-run-narrow`, `titlebar-band-run-terminal`,
`titlebar-band-run-busy`, `titlebar-band-run-hover`.

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
rows back into SwiftUI. Exactly one rule ever sits between two file boxes: a
header draws its bottom rule always and its top rule only where no folded
file's header is above it (`followsFoldedFile`); the Plan tab's boxes follow
the same rule. Stories: `diff-stress`, `diff-stress-unwrapped`, `diff-folds`.
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
