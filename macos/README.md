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

### Verify a UI change: render the gallery

A change to how the app looks is verified by rendering the stories it touches
and looking at the PNGs — not by launching the app. Launching it needs a
Notion the run may not have, a plan in whatever state that Notion happens to
be in, and a person to drive it to the pane in question; a story is the same
pane, canned, in about a second and a half. So: build, find the story, render
it, open it. Four commands, and they work as written from the repository root:

```bash
swift build --package-path macos
macos/.build/debug/gnat --list                                  # the index: every story and what it shows
macos/.build/debug/gnat --story diff-handed-back --out /tmp/diff.png   # one of them
macos/.build/debug/gnat --all --out /tmp/gallery                # the whole catalog
open /tmp/diff.png
```

`--list` writes nothing and takes no other flag. `--story` names one story by
the slug `--list` printed and `--out` the PNG file to write; a name the
catalog does not hold is refused with the whole catalog listed, so a typo
costs one run. `--all` renders every story into the directory `--out` names,
creating it if it is not there, one `<story-name>.png` apiece, and prints each
path as it lands — that is the run to make when a change is to the theme, the
window chrome or anything else that is not one pane's own. Both writes
overwrite, so rendering before and after a change into two directories is how
the difference is read.

If no story shows what changed, add one — an entry in
`Sources/NatApp/Gallery/AppStories.swift` and nothing else — and render that.
A pane that cannot be reviewed without launching the app is a gap in the
catalog rather than a reason to launch it, and `StoryNamesTests` is what
holds the catalog to covering every surface.

`NAT_SNAPSHOT` and a live screenshot are for what a story cannot show, and
only that: a real tmux session attached in the agent terminal, a real load
against Notion, the onboarding checklist as it reads on this machine. Those
are the two regions the gallery draws rather than runs (below) plus anything
whose point is that the data came from outside — everything else has a story
or should have one.

A *story* pairs a name with a view built from `NatFixtures` and the size to
draw it at, and the gallery is how a window gets reviewed the way the Go TUI's
golden snapshots let a screen be reviewed: a run touches no Notion, no `nat`
and no tmux, writes its PNGs and exits, so the same pixels come out on any
machine and in a clean checkout. A named story is about a second and a half.

The catalog is `Sources/NatApp/Gallery/AppStories.swift` — adding a story is an
entry in that array and nothing else. It covers the window shell, the rail in
each state a load leaves it in, every tab of the workflow, the workshop pane,
settings and onboarding, and it reads down in that order, which is what makes
`--list` an index of the app's UI rather than a heap of file names. Names are
slugs, because a name is both a `--story` argument and a file name, and every
story carries the one-line summary `--list` prints beside it;
`Tests/NatKitTests/Gallery/StoryNamesTests.swift` holds both rules — and the
coverage of the surfaces — over the source, since a catalog of views cannot be
built in a test target.

Two regions are drawn rather than run, because what they show is not data that
can be canned: the agent terminal is a tmux session on a pseudo-terminal, and
the onboarding checklist is the machine the app is running on. Both are
environment values with the real thing as their default (`Views/StorySeams.swift`),
so a story pins them and the app is untouched. `NAT_SNAPSHOT` — set to a path,
with the app launched normally — is still the headless eye on the *live*
window, and is what those two regions and a real Notion load are reviewed
through; the selector that used to drive it to a slice or to the workshop is
gone, since every state it could reach is a story now.

`Story`/`StoryCatalog` and the argument parsing are in `NatKit/Gallery`, where
they are tested; the AppKit capture is in `NatApp/Gallery`, where a window
belongs. It captures the window's own drawn pixels
(`bitmapImageRepForCachingDisplay` + `cacheDisplay`, the path `NAT_SNAPSHOT`
already trusts) rather than `ImageRenderer`, which renders the tree afresh and
skips scrollable containers' content — half the states worth drawing are inside
a scroll view, so a renderer pass would hand back a gallery of empty panes.
Reading the arguments has to happen before `App.main()` takes over the process,
which is why `Sources/NatApp/main.swift` is the entry point and `NatApp` carries
no `@main`.

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
