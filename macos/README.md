# nat macOS App

A native macOS application for the notion-agent-tracker project, built as a pure SwiftPM package.

## Architecture

- **NatKit** — Core library containing all business logic, models, and theme definitions. All code lives here.
- **NatApp** — Minimal executable target with thin SwiftUI views. Views bind to NatKit logic but contain no business logic themselves.
- **NatKitTests** — Unit tests for NatKit. Views are not tested for pixel perfection; only the underlying logic is tested.

## Conventions

- All domain logic and state management lives in `NatKit`.
- Views in `NatApp` are thin bindings to logic in `NatKit` — they format and display state but never process it.
- Tests target logic, not pixels. The teatest/golden-snapshot approach from the Go codebase does not apply here; instead, focus on unit tests for models and business logic.
- The design reference lives in `docs/design/nat-ui-v2/nat-ui-v2.html` — all colors and token values come from the `.nat` CSS block in that file.
- Theme tokens are centralized in `NatKit/Theme/DesignTokens.swift`, and are
  *dynamic*: each one holds both palettes and resolves the one the window's
  appearance calls for, so a view says `DesignTokens.windowBg` and never asks
  which theme is on. The values themselves — Catppuccin Mocha and Latte — live
  in `NatKit/Theme/Palette.swift`, taken as published — the tests there assert
  every role is Catppuccin's own swatch, unedited. Do not bend a value to meet
  a contrast number: it would be a colour no other Catppuccin has.
- Which palette a window asks for is `NatKit/Theme/Theme.swift`: system, dark
  or light, persisted in `UserDefaults` under `Theme.storageKey` and switched
  from the Settings window. `system` pins nothing, which is what makes it
  follow the Mac's own appearance as that changes.
- No view file constructs a colour. Every one it draws — including the washes
  and bands that used to be a token behind a bare `.opacity(0.18)` — is a named
  token, so a weight is chosen once in the theme rather than typed again in
  each file that wants it. `Tests/NatKitTests/Theme/ColorSourcesTests.swift`
  scans `Sources` and holds the rule: no `Color(nsColor:)`, no `Color(hex:)`
  outside the theme, no SwiftUI system colour, and no `DesignTokens.x.opacity(…)`
  at a call site. `Color.clear` is allowed — it is the absence of paint.
- A colour that will not parse falls back to Mocha's mauve, the app's accent,
  rather than to white: a parse failure is a bug the tests catch, and the frame
  drawn before anyone reads them should still read as this app.
- The monospaced face is the app's own, not the Mac's: JetBrains Mono, four
  static faces under `NatKit/Resources/Fonts` (with the OFL beside them),
  registered with CoreText for this process alone at launch — nothing is
  installed on the Mac. `MonoFont` holds the names and the registration and
  `Typo.mono(size:weight:)` is the one thing a view calls, falling back to the
  monospaced system font wherever the face cannot be had. Everything
  monospaced goes through it — the terminal, the diff, markdown code spans —
  and so does every `TextField` and `TextEditor` in the app, each at the point
  size it already had — bar the settings window's, which is built as a
  built-in settings window is built and so sets its fields in the system
  font; `MonoSourcesTests` names that one exception rather than leaving each
  file to claim one. `.monospacedDigit()` is not monospaced text: those are
  proportional labels asking for lining digits, and they stay as they are.
  The fonts live in NatKit rather than NatApp because `Typo` and the markdown
  renderer do, and SwiftPM resources have to sit inside the target that
  declares them; `make-app.sh` copies `nat_NatKit.bundle` into the app beside
  `nat_NatApp.bundle` for exactly that reason.
- The agent terminal is the one surface a dynamic colour cannot reach —
  SwiftTerm resolves plain `NSColor`s once — so `NatApp/Views/TerminalTheme.swift`
  pushes the palette onto it whenever the appearance changes. It pushes the
  type too, from `TerminalType` in `DesignTokens.swift`: the app's monospaced
  face at `Typo.code`, the face the diff pane draws a line of code in, and
  macOS font smoothing off. Both are defaults SwiftTerm would otherwise pick
  for itself, and the smoothing is why the pane read as blurred — it dilates
  every stroke by about a fifth of its ink at any scale, which on a dark
  ground is a halo rather than weight, and nothing else in the window is
  drawn with it.

## Building and Running

### Run the app in development:
```bash
swift run --package-path macos gnat
```

Or open the project directly in Xcode:
```bash
open macos/Package.swift
```

### Run tests:
```bash
swift test --package-path macos
```

### Build a release .app bundle:
```bash
bash macos/Scripts/make-app.sh
```

This creates `macos/.build/gnat.app`, which can be run with:
```bash
open macos/.build/gnat.app
```

The bundle carries its own `nat`, built universal (arm64 + x86_64) from the
same checkout, so the app and the nat it runs are never out of step and
nobody go-installs the same tool twice — building the bundle therefore needs
the Go toolchain as well as Swift's. At startup the app composes its PATH
from the bundled nat's directory, the login shell's PATH, and whatever
launchd handed over (`PathBootstrap`), then carries it into the agent tmux
sessions, so a Finder launch resolves `nat`, `tmux`, `gh` and `ntn` exactly
as a terminal one would. `tmux`, `gh` and `ntn` remain the machine's own —
credentialed tools on their own update schedules are not ours to bundle.

The app itself does not resolve `nat` off that PATH, though: `NatBinary` says
which nat it runs, and says it explicitly — `NAT_BIN` (the dev override), then
the binary beside the app executable, by absolute path. A packaged app whose
nat is missing reports a damaged install, in the onboarding checks and as the
error of any command it was asked to run, rather than falling through to
whatever older install PATH offers; only a bare dev executable outside any
`.app` searches PATH at all. The PATH prepend stays because the agent sessions
do resolve `nat` off PATH, inside tmux.

## Package Structure

```
macos/
├── Package.swift                 — SwiftPM manifest
├── Sources/
│   ├── NatKit/
│   │   ├── Resources/Fonts/      — JetBrains Mono (bundled, OFL)
│   │   ├── Theme/                — Design tokens and styling
│   │   ├── Models/               — Domain models
│   │   ├── NatClient/            — Notion API client (future)
│   │   └── Stores/               — State management (future)
│   └── NatApp/
│       └── NatApp.swift          — Entry point and minimal views
├── Tests/
│   └── NatKitTests/              — Unit tests
├── Scripts/
│   └── make-app.sh               — Release build script
└── README.md                     — This file
```

## Next Steps

- Add models in `NatKit/Models/`
- Implement Notion API client in `NatKit/NatClient/`
- Add state management in `NatKit/Stores/`
- Build view hierarchy in `NatApp/` using tokens from `DesignTokens`
