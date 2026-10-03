# Debugging keys in gnat's agent pane

How a key press reaches Claude Code from gnat's agent pane, where each hop can
be watched, and the harness that found why shift+enter submitted.

## The chain

```
NSEvent (hardware or NSApp.postEvent)
  → NSApplication: local key-down monitors          ← FirstLayoutTerminalView's monitor
  → NSWindow.sendEvent → first responder keyDown     ← SwiftTerm TerminalView.keyDown
      → interpretKeyEvents → doCommand(insertNewline:) → sendKittyFunctionalKey
  → TerminalView.send(data:) → LocalProcessTerminalView.send(source:data:)  ← NAT_KEY_DEBUG logs here
  → pty → tmux attach client (-T 256,RGB,extkeys,focus)
  → tmux server (extended-keys on, terminal-features *:extkeys:hyperlinks)
  → pane (Claude Code asks for extended keys itself: #{pane_key_mode} = Ext 2)
```

| Hop | How to observe it |
|---|---|
| NSEvent offered to the monitor | `NAT_KEY_DEBUG=1`: `offered …` line (key code, raw flags, repeat, same window, first responder, focus) |
| Monitor's decision | `intercepted keyCode=… -> …` when it sends a CSI-u |
| Monitor lifecycle | `monitor installed view=… window=…` / `monitor removed view=…` |
| Bytes to the pty | `pty <- …`, escaped; a bare `\r` also logs `sent by:` with its call stack |
| What the pane receives | the recorder below, on a private tmux server |

## Already settled (2026-10-03, tmux 3.7b, Claude Code 2.1.281, SwiftTerm 5d14406)

- From the attach client's pty inward everything works: `ESC [ 1 3 ; 2 u`
  written into a `tmux -u -T 256,RGB,extkeys,focus attach-session` client with
  nat's server options reaches Claude Code as a line break, under both
  `extended-keys-format` values (the default `xterm` hands the pane
  `ESC [ 2 7 ; 2 ; 1 3 ~`). A pane that has not asked for extended keys gets
  the shift folded away to `\r` — a shell's case, not Claude's.
- SwiftTerm's own `keyDown` sends a plain `\r` for shift+return:
  `kittyFunctionalKey(from:)` maps no Return, so the press goes through
  `interpretKeyEvents` → `doCommand(insertNewline:)`, and SwiftTerm ignores
  the `CSI > 4 ; 2 m` tmux sends it, so its kitty flags stay empty. The
  monitor is the only thing that produces the right bytes.

## The cause (found 2026-10-03)

The monitor closure was

```swift
self?.interceptModifiedEnter(event) ?? event
```

`interceptModifiedEnter` answers nil to swallow a press it has sent, and
`?? event` — meant only for a view that has gone — turned that nil back into
the event. So every intercepted shift+return was delivered twice: the monitor
wrote `\x1b[13;2u` (a line break), then SwiftTerm's `keyDown` wrote `\r`
(submit). That is "submits, sometimes with a line break, never a line break
alone". The fix is `KeyMonitorAnswer` (NatKit): a gone view passes the event
on, a live view's answer stands, nil included.

What the harness showed:

| | gnat → pty | recorder |
|---|---|---|
| before, shift+return | `\x1b[13;2u` then `\r` | `\x1b[27;2;13~` then `\r` |
| before, ctrl+return | `\x1b[13;5u` then an empty run | `\x1b[27;5;13~` |
| after, shift / ctrl / plain (synthesized and real) | `\x1b[13;2u` / `\x1b[13;5u` / `\r` | `\x1b[27;2;13~` / `\x1b[27;5;13~` / `\r` |

The `sent by:` stack for the stray `\r` went
`NSWindow sendEvent → TerminalView.keyDown → interpretKeyEvents →
doCommand → sendKittyFunctionalKey → send` — the same event the monitor had
already answered.

## The harness

**Never run a bare `tmux` command, and never `kill-server`, from a shell inside
an agent session.** `$TMUX` is set there, and a tmux command with no `-L`/`-S`
talks to the server `$TMUX` names — the user's, with every agent session on
it. Setting `TMUX_TMPDIR` does not change that. Give every command `-S`, and
end things with `kill-session`/`kill-window` by name.

The socket lives under a short path: macOS caps a unix socket path at 104
bytes, which a scratchpad path exceeds.

```bash
SOCK=/tmp/natk/tmux-501/default            # what TMUX_TMPDIR=/tmp/natk resolves to
mkdir -p /tmp/natk/tmux-501 && chmod 700 /tmp/natk/tmux-501
env -u TMUX tmux -S $SOCK -f /dev/null new-session -d -s keyrec "python3 recorder.py /tmp/natk/recorded.log"
env -u TMUX tmux -S $SOCK set -s extended-keys on
env -u TMUX tmux -S $SOCK set -as terminal-features '*:extkeys:hyperlinks'
env -u TMUX tmux -S $SOCK display -p -t keyrec '#{pane_key_mode}'   # expect: Ext 2

swift build --package-path macos --product gnat
env -u TMUX -u TMUX_PANE TMUX_TMPDIR=/tmp/natk \
  NAT_TERM_SESSION=keyrec NAT_KEY_DEBUG=1 NAT_KEY_DEBUG_SYNTH=1 \
  macos/.build/debug/gnat 2>gnat.log
```

gnat's attach scrubs `TMUX`/`TMUX_PANE` (`AttachSpec.environment`), so with
`TMUX_TMPDIR` set it attaches to the private server. Drop
`NAT_KEY_DEBUG_SYNTH` to press the keys by hand: click into the pane, press
shift+enter, ctrl+enter, enter. Stop gnat by its PID; tear down with
`env -u TMUX tmux -S $SOCK kill-session -t keyrec`.

`recorder.py` — asks for extended keys as Claude Code does and appends every
stdin read, escaped:

```python
#!/usr/bin/env python3
import os, sys, tty, time
out = sys.argv[1]
fd = sys.stdin.fileno()
tty.setraw(fd)
os.write(1, b"\x1b[>4;2m")  # request extended keys, mode 2
def esc(b):
    s = []
    for c in b:
        if c == 0x0d: s.append("\\r")
        elif c == 0x0a: s.append("\\n")
        elif c == 0x5c: s.append("\\\\")
        elif 0x20 <= c <= 0x7e: s.append(chr(c))
        else: s.append("\\x%02x" % c)
    return "".join(s)
while True:
    b = os.read(fd, 1024)
    if not b: break
    line = "%.3f %s\n" % (time.time(), esc(b))
    with open(out, "a") as f: f.write(line)
    os.write(1, line.replace("\n", "\r\n").encode())
    if b == b"\x03": break
```

Two traps it hit:

- A dev binary's Sparkle updater used to run an app-modal alert at launch
  (now `UpdaterGate` starts it only from a bundled `.app`). Nothing in the
  run loop's default mode fires under a modal, so a synthesized press
  scheduled that way never happens — and an empty log reads as a pass. The
  synth uses common-mode timers and dismisses any modal, logging it.
- The synth focuses the pane itself (logged as `synth: focusing pane`) since
  nothing has clicked it; a real press needs the click.

## Telling the failure shapes apart in the log

- **Guard refused the press** — an `offered` line with no `intercepted`
  after it, then a `pty <- \r`. The fields on `offered` say which guard:
  `sameWindow=false`, `focus=false` (and `firstResponder=` names who has it),
  a `keyCode` other than 36/76, or `flags` carrying more than one of shift
  (0x20000), control (0x40000), option (0x80000), command (0x100000) —
  device bits like 0x100/0x2 are ignored.
- **Monitor not installed** — no `offered` line at all for the press; look
  for a `monitor removed` with no later `monitor installed` for that view.
- **Two monitors** — two `offered` lines, different `view=`, for one press.
- **Monitor swallowed but SwiftTerm still got it** (this bug) —
  `intercepted`, the CSI-u run, then a `pty <- \r` whose `sent by:` stack
  runs through `TerminalView.keyDown`.
- **Something else sent `\r`** — a `pty <- \r` whose `sent by:` stack does
  not come from `keyDown` for that press.

## Claude Code's hints inside tmux

Claude shows "ctrl+x ctrl+s to send now" rather than "ctrl+enter" in every
gnat pane, and that is Claude's choice, not gnat's: `Ope()` picks the hint as
`j9t(bindings, h1e() && !(TMUX || STY))` — inside tmux or screen it shows the
last send-now binding that is not a ctrl/shift+enter chord, whatever the
outer terminal can do. Its startup probe under gnat reads (`claude
--debug-file`): XTVERSION via `#{client_termtype}` = `SwiftTerm 1.20.0`,
`extendedKeys=yes (env: terminal=tmux)`, `kittyKeyboard=no (probe: no reply
to CSI ? u)`, `kittyGraphics`/`mousePixels=no (env: inside tmux or screen)`.
ctrl+enter still works (the harness above). A user keybinding of
`"ctrl+x ctrl+s": null` in the `Chat` context leaves ctrl+enter the only
send-now binding, and the hint falls back to it.
