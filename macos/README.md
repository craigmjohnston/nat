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
- Theme tokens are centralized in `NatKit/Theme/DesignTokens.swift`.

## Building and Running

### Run the app in development:
```bash
swift run --package-path macos NatApp
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

This creates `macos/.build/NatApp.app`, which can be run with:
```bash
open macos/.build/NatApp.app
```

## Package Structure

```
macos/
├── Package.swift                 — SwiftPM manifest
├── Sources/
│   ├── NatKit/
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
