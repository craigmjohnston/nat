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
  `paths`, `config-show`/`-set`, `project-create`/`-open`).
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
(`stage(for:)`) is the one source of where a slice stands: the pane's landing
tab and `RailModel.isReviewSlice`/`isActiveSlice` both read it, and it mirrors
Go's `domain.StateOf` (Notion's status is the one source of lifecycle truth,
so a Done slice is never in-flight even with `nat pr-status` still reporting
its PR open) — change `internal/domain/state.go` and the stage together. A
live session never moves the stage; `fixing` comes only from
`AppModel.fixLaunched`.

## The new-project proposal

An Untitled tab's workshop proposes with `nat plan-propose`; the app never
reads the proposal file itself. `AppModel` watches the nudge marker (its own
`NudgeWatcher`, alive while any Untitled tab is) and reads `nat plan-proposal`
per tab; the rail draws `PlanProposal.folders` with the ordinary folder/slice
rows. Accept is `nat plan-accept` (project + plan are nat's doing), then the
workshop session is killed and `addProject(replacing:)` hands the tab over.
Stories: `window-untitled-proposal`, `window-plan-accepted`.

## The Notion mirror nudge

After an accepted plan, `AppModel.acceptProposal` arms `MirrorNudgeMemory`
(UserDefaults, per project; `.inMemory()` for tests and stories) and the rail
foot draws `MirrorNudgeCardView` while `mirrorNudgeShown` (armed **and** local).
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

## Design tokens

Every colour is a named, dynamic token in `DesignTokens.swift` (Catppuccin
Mocha/Latte from `Palette.swift`), never a bare `Color(hex:)`/`(nsColor:)` at
a call site (`ColorSourcesTests` enforces this) — reference is
`docs/design/nat-ui-v2/nat-ui-v2.html`'s `.nat` CSS block.
