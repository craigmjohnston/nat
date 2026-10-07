# Debugging gnat's context menus

The wedge: right-click stops presenting gnat's SwiftUI `.contextMenu`s —
slices, milestone and project rows, Active rows, containers — and stays
broken until relaunch. It has tended to follow sleep/wake or a long idle.

**Worked around, cause unknown.** The sidebar no longer relies on SwiftUI's
right-button handling: `ContextMenuRegion` (NatKit), mounted behind
`SidebarView`, takes every right-click and control-click inside the sidebar,
asks the hit view's `menu(for:)` walking up (as `RightClickDiagnosis` does),
and puts the answer up with `NSMenu.popUpContextMenu(_:with:for:)` — the
control-click path, which kept working while wedged. It consumes the click,
so SwiftUI never presents a second menu, and it does so always, wedged or
not. `.contextMenu` is still where every menu and its actions are defined.
Its monitor is installed after the trace's, so the trace below still logs
every click. What breaks inside SwiftUI's right-button path is not known.

## The captured trace (2026-10-06, release build 1.0.209, macOS 15.7.3)

Saved whole at
`~/Library/Logs/notion-agent-tracker/gnat-menu-debug-2026-10-06.log`.

- The instance launched at 12:38 with the trace on. Its first right-click
  came at 15:10; every one after missed. Whether right-click ever worked in
  that instance is not known: there was none before 15:10 to say. (The slice
  and project menus that opened at 12:52 had no right-click line before
  them: the hover three-dot buttons, a left click.)
- 14 misses between 15:10 and 15:17, all two-finger trackpad clicks (the
  trace cannot tell a trackpad from a mouse; the user reported it). 11 are
  `menu-not-presented`, on slice rows, a milestone row, a project row and
  the workshop row: `menu(for:)` answered the row's whole menu —
  `["Launch agent", "Edit description…", "Open in Notion", "", "Move to",
  "Delete…"]`, `["New task…", "Rename…", "", "Move up", "Move down", "",
  "Delete"]`, the project menu, `["End workshop session…"]` — and no menu
  began tracking. The other 3 are `no-menu-from-swiftui`, clicks outside
  the sidebar (the main pane, twice the agent terminal), which have no menu.
- Every miss reads open menus 0, no modal, no sheet, the main window key and
  the app active. The first responder was `FirstLayoutTerminalView` (the
  embedded agent terminal, `AgentTerminalHostView.swift`) on 13 of them and
  the window itself on the first.
- A control-click on a slice row at 15:17:08, in the same state, presented
  its menu at once; the menu bar, the `+` menu and alerts had been opening
  all afternoon.
- No sleep, wake or session change since launch. There *were* screen
  changes: bursts of `NSApplicationDidChangeScreenParameters` (~60 a
  second, ~1,500 in all) between 13:38 and 14:52, the main and terminal
  windows posting `DidChangeScreen` with them. Whether they wedged anything
  is not known — there was no right-click across them.

So the diagnosis is `menu-not-presented`: SwiftUI builds the menu and hands
it over on request; what does not run is the right-button path from
`rightMouseDown` to presentation, where control-click (AppKit asking
`menu(for:)` itself from `NSWindow`'s event handling) still does.

What a wedged instance showed (2026-10-04, release build, up ~7 h):

- **Control-click still opens the same menus; right-click does not** —
  neither a mouse's right button nor a two-finger trackpad click, held or
  quick. Both gestures end in the hosting view's `menu(for:)`, so SwiftUI
  still has the menus: what has died is the right-button path to them.
- Every `.contextMenu` in the app is in `SidebarView`, so "app-wide" is
  the sidebar. Text-field menus in the main window, a sheet's fields and
  the Settings window all still answer a right-click, as does the menu bar.
- No window of gnat's covers the main one (`CGWindowListCopyWindowInfo`:
  the main window is the only one on screen).
- The unified log has no AppKit warning around it. The last context menu
  that opened did so at 09:03:54, its item picked from a submenu at
  09:03:57; two alerts followed at 09:04:18 and 09:04:28. When right-click
  first failed is not known.
- A release build cannot be attached to (hardened runtime, no
  `get-task-allow`), so a wedged instance can only be read from outside —
  hence the trace below, which has to be on *before* the wedge.

## Turn the trace on

For the installed app, which carries it for as long as it takes to come:

```bash
defaults write com.craigmjohnston.nat.NatApp NatMenuDebug -bool YES
# quit and relaunch gnat
```

For a dev run, `NAT_MENU_DEBUG=1 swift run --package-path macos gnat`.
`defaults delete com.craigmjohnston.nat.NatApp NatMenuDebug` turns it off
again. Off, it installs nothing (`MenuDebug.startIfAsked`).

## When it wedges

Before relaunching:

1. Right-click a slice row, a milestone row and a project row, once each,
   a second or so apart. Then control-click a slice row.
2. Note whether the menu bar opens, and whether a text field's right-click
   menu appears.
3. Save the trace and AppKit's own lines from around then:

   ```bash
   log show --last 1d --style compact \
     --predicate 'process == "gnat" AND eventMessage CONTAINS "nat menu-debug"' \
     > ~/Desktop/gnat-menu-debug.log
   log show --last 15m --style compact --predicate 'process == "gnat"' \
     > ~/Desktop/gnat-all.log
   ```

`--last 1d` reaches back past whatever preceded the wedge; widen it if the
app had been up longer.

## Reading it

Every line starts `nat menu-debug:`.

| Line | Says |
|---|---|
| `right-click #N right\|control at (x, y) in <window> hit A < B < NSHostingView<…>` | the click reached the app; the views the window hit-tested it to, innermost first |
| `menu began tracking` / `menu ended tracking (open: n)` | a menu opened or closed, with how many are open — a count that never returns to 0 is a menu AppKit thinks is still up |
| `right-click #N presented a menu` | healthy |
| `NO MENU for right-click #N: <diagnosis> \| …` | no menu began within 0.75 s, with the window state at that moment |
| `NSWorkspace…Sleep/Wake…`, `NSApplicationDid…Active`, `NSWindowDid…Key`, `…Sheet`, `…Screen`, `…BackingProperties` | what came before |

No `right-click` line at all after a click: the event never reached the app's
local monitors (not expected — text-field menus still work).

The diagnosis says where a click that opened nothing stopped. SwiftUI serves
`.contextMenu` through its hosting view's `menu(for:)` (checked on macOS
15.7), and the probe asks that only after a miss:

- `not-swiftui` — the click was hit-tested to a view outside SwiftUI's
  hosting view: something is covering the window (an overlay window or
  view). Fix where that view comes from.
- `no-menu-from-swiftui` — SwiftUI got the click and has no menu for the
  point: its context-menu state is lost. Expected for a click on a row
  with no menu (empty space), so read it against what was clicked.
- `menu-not-presented` — SwiftUI answers the menu and AppKit never put it
  up: presentation is what fails. What the 2026-10-06 trace caught, and what
  `ContextMenuRegion` works around. With it in place a sidebar right-click
  never reaches SwiftUI's path, so a miss on a sidebar row now means gnat's
  own presentation failed.

The trailing state (`open menus`, `modal`, `sheet`, `key`, `active`, `first
responder`) is what to compare across the healthy clicks before the wedge
and the failed ones after it.

## Already ruled out (2026-10-04, macOS 15.7.3)

A standalone SwiftUI app with the same shape (`.contextMenu` rows in an
`NSHostingView`), driven by right-clicks posted to its own queue and
counting `NSMenu.didBeginTrackingNotification`, kept presenting menus after
each of:

- a left mouse-down with no mouse-up (a release lost across sleep), and a
  right mouse-down with no mouse-up;
- a mouse-down, then the app hidden and reactivated;
- the clicked row removed, re-rendered, or replaced while its menu was open
  (a board refresh mid-menu), and the window's root view replaced;
- the menu's item performed;
- the sidebar's own shape — rows with `onHover`, `onTapGesture` and a
  `.contextMenu` holding a submenu, in a `LazyVStack` with pinned section
  headers inside a `ScrollView` — with a submenu item that moves the row to
  another section, and an item that deletes it.

None of these wedges SwiftUI's menus by itself, so the trigger is something
gnat's real window or a real sleep/wake supplies.

## Menu items with no icons

Not the wedge, but it looks like a menu bug: on macOS 15 SwiftUI builds a
menu item with no image unless the label style is `.titleAndIcon` — every
`Button(_, systemImage:)` in a `.contextMenu` or `Menu` came out as title
only (read off the `NSMenu` the hosting view's `menu(for:)` returns:
`NSMenuItem.image` nil). `SidebarView` sets `.labelStyle(.titleAndIcon)` on
its root, which reaches every menu under it, submenus included; a menu with
icons anywhere else needs the same.
