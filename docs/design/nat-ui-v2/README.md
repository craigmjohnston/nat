# nat UI v2 — design reference

Export of the Claude Design project **"macOS native UI for TUI app"**
(`claude.ai/design/p/e81457f6-c2ca-40ae-8f1f-77062e2aa319`), file `nat UI v2.html`
and the JSX/tokens it imports. This is the mock the `macos/` SwiftUI app is built
from. The React code here is a *spec to read*, not code to run or port literally.

## What's here

- `nat-ui-v2.html` — the canvas page. Mounts five screens and defines the
  **nat dark palette** (the `.nat {...}` style block: `--accent:#b1a8f2`,
  `--window-bg:#1e1e23`, the system tint overrides, etc.). That palette is the
  app theme; see `macos/NatKit/Sources/NatKit/Theme/`.
- `nat-sidebar.html` — the sidebar reference canvas (`nat Sidebar.html` in the
  design project): four rail states — default, Done expanded, todo selection,
  quiet — showing the empty-section rules, the TODO heading, and the DONE
  heading at the foot of the plan.
- `ui-v2-shell.jsx` — the window shell: header with browser-style project tabs,
  slice count, New Slice / Workshop toolbar buttons; the left rail (NEEDS
  REVIEW and ACTIVE when non-empty, a TODO heading over milestone cards with
  progress rings, and a DONE heading at the bottom the finished milestones
  expand under); the workflow tab strip (Brief → Agent → Diff → PR with
  disabled-until-reached and past-checkmark states); the bottom milestone
  progress border.
- `ui-v2-views.jsx` — the five right-pane screens: `AgentView` (native
  transcript — **not built**; kept for reference), `AgentTerminalView` (the
  embedded terminal — **this is what we build**), `BriefView` (brief + split
  Launch button + model/effort popover), `DiffView` (file list, GitHub-style
  diff boxes, pending comment cards, Send Comments / Approve & Open PR footer),
  `PRView` (status pill, description, conversation threads, CHECKS/REVIEW/
  CHANGES sidebar, merge-when-green footer).
- `ui-shared.jsx` — shared fixtures and smaller shell pieces (MacWindow, an
  earlier sidebar/plan-column variant, status glyphs ○ ◐ ✓ ⊘ and the ✻ star).
- `design-system/` — the "macOS Native Design System" readme and token CSS.
  Read `design-system/readme.md` first: it is the style contract (semantic
  colors only, system font, 13px body, 22px controls, hairline separators,
  SF Symbols, Title Case commands, no emoji in chrome).

## Mapping notes for SwiftUI

- Icon names in the JSX (`checkmark_circle`, `chevron_down`, `arrow_branch`,
  `wand_stars`…) are Framework7 ligatures drawn to SF style — most translate
  directly to SF Symbols (dots become periods: `checkmark.circle`). Pick the
  real SF Symbol nearest in meaning; never ship the web font.
- Token CSS uses `light-dark()` pairs; in SwiftUI use semantic `NSColor`s
  (`.controlBackgroundColor`, `.separatorColor`, label levels) so both schemes
  come free. The nat palette overrides only what the mock's `.nat` block names.
- The mock's pixel metrics (22px controls, 28px rows, 0.5px hairlines, radii
  6/5/10) are in `design-system/tokens/spacing.css`.

## Deliberate departures from the mock

- **The project tab's dot is a state, not an identity.** The mock gives each
  tab a coloured dot as a per-project identity mark, and the app shipped that
  as a hash of the project ID onto orange/green/yellow/red — a colour that
  meant nothing, and one a project could land on red by. It now says what,
  of everything in flight on that project, is most worth the eye: yellow an
  agent waiting for input, green a branch handed back or a pull request ready
  to merge, the accent purple agents working with nothing waiting, and a
  muted dot for a quiet project. Only the working dot pulses. The number
  beside it counts those same things needing attention rather than live tmux
  sessions. It is one vocabulary with the rail, which reads the same roles in
  the same colours — see `macos/Sources/NatKit/ViewModels/ProjectAttention.swift`.
  The accent for "working" is a departure from the rail's own earlier orange
  too: at 8px, orange and the waiting yellow are all but the same colour.

## Omitted from the export

- `_ds_bundle.js` and `styles.css` — the React *implementations* of the design
  system components (web-only; the readme describes them).
- Other files in the design project (`nat macOS UI.html`, `ui-ws-*.jsx`,
  `agent view styles.html`…) are earlier workshop iterations, superseded by v2.
