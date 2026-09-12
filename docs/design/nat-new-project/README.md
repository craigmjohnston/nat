# nat New Project — design reference

Export of the Claude Design project **"macOS native UI for TUI app"**
(`claude.ai/design/p/e81457f6-c2ca-40ae-8f1f-77062e2aa319`), file
`nat New Project.html` and the JSX it imports. This is the mock for the
new-project experience in the `macos/` SwiftUI app: the Untitled tab and
starter card, the workshop flow, the PROPOSED tree, the accepted state,
and the Notion mirror picker.

The design-system tokens the HTML links (`_ds/macos-native-design-system-…`)
are the same design system already exported at
`../nat-ui-v2/design-system/tokens/` — read them there. The file paths are
kept as the design wrote them, so the HTML is for reading, not opening in a
browser.

`ui-shared.jsx` and `ui-v3.jsx` are the shared shell components
(`MacWindow`, `U3SectionHead`, `U3ActiveRow`, `U3MsFolder`, …) the
new-project screens compose; `ui-newproject.jsx` is the Untitled-tab
starter screen and `ui-npflow.jsx` the workshop/proposal/accepted flow and
the Notion picker.

## Deliberate departures from this mock

Decisions made with Craig that override what the mock draws — the mock is
the spec except for these:

- **Tab strip style.** The mock's project-tab strip (`NPHeader`/`U3Header`,
  the top-rounded tabs with radial corner fillets) is the app's *old,
  deprecated* tab style. Keep the app's current tab style; take only the
  behaviour — the `+` button opening an Untitled tab, the italic "Untitled"
  title, the neutral (label-quaternary) dot, the close affordance.
- **ACTIVE empty state height.** The mock shows "Nothing running" as a
  single line of subheadline text. Build it reserving the height of one
  two-line active entry, so the section does not change height when the
  first entry lands.
- **Workshop terminal.** The mock's workshop pane transcript
  (`NFTerminal`) is illustrative. The real pane is the existing embedded
  Claude Code terminal (the workshop pane the app already has) — no custom
  workshop chat interface.
- **Colours.** Use the app's existing theme tokens nearest to what the
  mock shows; never invent new colours.
