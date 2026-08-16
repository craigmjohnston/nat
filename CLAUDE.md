# notion-agent-tracker

A Go TUI over Notion for tracking project work executed by Claude Code agents.
Craig manages the plan and launches agents from the TUI; agents run as fresh
`claude` sessions in tmux and reach the tracker only through the headless `nat`
commands — they need no Notion access of their own. The TUI talks to the Notion
REST API directly (`Notion-Version: 2026-03-11`, data-source model).

## Architecture

- `main.go` — entrypoint; module path `github.com/craigmjohnston/nat`, so
  `go install github.com/craigmjohnston/nat@latest` yields a `nat` binary.
  Started outside tmux it re-execs itself into a `nat-tui` session
  (`tmux new-session -A`, so a second launch attaches rather than starting a
  rival); started inside tmux it runs in place and does not nest. `NAT_NO_TMUX=1`
  opts out. The board is a pane so the tmux bar under it can draw the status
  line and so the agents it launches have a server to live in; viewing one no
  longer needs it — the terminal beside the board is nat's own, and there is
  nothing to hand back on the way out.
- `internal/config/` — XDG config (`~/.config/notion-agent-tracker/config.json`)
  + the Notion bearer token, read from Notion's official CLI via
  `ntn auth token` (the app stores no credential of its own).
  `AgentModel` — a `model` and an `effort`, exactly as Claude Code's own flags
  take them — is which Claude Code a launched agent runs as, and the config
  holds two of them: `workshop_agent` for the planning agent and `slice_agent`
  for a slice's, because workshopping a plan is conversation and often wants a
  lighter model than the agent that writes the code. Either half of either may
  be unset, and an unset half contributes no flag at all, which is the launch
  saying nothing and Claude Code deciding as it always did. Both are defaults
  rather than settings a launch is bound by: the planning form and the launch
  options show the pair prefilled and editable for that one launch, which
  writes nothing back. `W` is the exception, as it always is — it asks nothing
  at all, so it takes the workshop pair as it stands.
  `SplitPercent()` and `PollInterval()` swap the default back in for a number
  outside the bounds, which is a typo read as an instruction and lost without a
  word; `ValidSplitPercent`/`ValidPollSeconds` are the same bounds said out
  loud, so the settings form refuses exactly what a later read would have
  discarded. Zero passes both: it is how the config writes "unset", and the
  getters answer it with the default of their own accord.
- `internal/notion/` — hand-rolled stdlib Notion client (no third-party client
  supports data sources); minimal structs, only fields we use. Built with
  `NewWithToken(TokenFunc)`: the token is fetched per attempt and a 401 is
  retried once with a fresh one, so a token rotated by the CLI is picked up
  mid-session.
- `internal/domain/` — Project/Milestone/Slice models, progress math
- `internal/logging/` — the log file: `~/Library/Logs/notion-agent-tracker/` on
  macOS, the XDG state dir elsewhere, size-capped with one previous file kept.
  Opened by `main` and discarded until then, so importing it writes nothing.
  Failures and writes are logged by the Notion client and the tmux layer
  themselves; startup failures name the log path on stderr, because the TUI's
  tmux pane dies with the process and takes stderr with it. Everything on its
  way to the file passes through a redactor — never log a token or a request
  body.
- `internal/nudge/` — the marker file the headless commands touch after a
  successful Notion write (`nudge` in the log/state dir), and the board's way of
  reading it: the TUI stats its mtime every second and refetches the plan when
  it moves, so agent-driven changes show within a second instead of a poll
  interval. Fire-and-forget on the CLI side — a touch that fails is logged and
  swallowed, and commands are wired to it through `Env.Nudge`.
- `internal/agent/` — agent prompt template + tmux session management. A
  running agent is identified by its pane's `@nat_slice` option (the full slice
  page ID); the session name `nat-<last-8-hex-of-slice-page-id>` is only a
  human label, and takes the tail because page IDs share a leading prefix.
  Viewing an agent joins no panes: the board runs an attach client on a PTY of
  its own and draws it, so an agent's pane stays in the session it launched in
  and nothing here makes a stray. `ReclaimStrays` — with its private
  `breakOut`/`breakOutAll`/`placeholderCommand`, all under one deprecation
  comment — is the exception, still run at startup to re-home the panes a
  pre-upgrade nat left joined, and comes out next release.
  `activity.go` is how those agents are told apart from each other's states:
  `Tmux.Activity` scans the panes once and reads the screen of each tagged one
  (`capture-pane -p -J`), answering working / waiting / gone / unknown per slice
  page ID — the same keys `LiveSlices` uses, so one map lays over the other. The
  signal is Claude Code's running status line, matched against the visible
  screen alone by its shape rather than its wording — a verb that trails off and
  then the turn's elapsed time in brackets, `✻ Quantumizing… (1m 6s · …)`: it is
  on every busy screen and no idle one, where enumerating the shapes it stops
  for — a permission prompt, a question, the end of a turn — would be a list
  that goes stale, and so would any of the words in the line itself. A dead pane
  (`pane_dead`, which only appears where the user has set remain-on-exit) is
  gone without a capture, as is one that vanishes between the scan and the
  capture; a capture that fails outright leaves the state unread rather than
  declaring a running agent gone. It is a poll, with no timer of its own — the
  TUI decides how often to take a reading.
  Both ways of attaching to a session — `AttachCmd`, full-screen through
  `tea.ExecProcess`, and `AttachClientCmd`, the hidden client the embedded
  viewer runs on a PTY of its own — build the same argv, `tmux -T
  <ViewerFeatures> attach-session -t <session>`, and drop `TMUX`/`TMUX_PANE`
  from the environment, since tmux refuses to nest an attach while they are
  set. Only the client command replaces `TERM` (with `xterm-256color`): the
  terminal on the far end of its PTY is the viewer's emulator, where the
  full-screen attach's is the user's own.
- `internal/gh/` — the GitHub CLI, wrapped as thinly as it can be. Agents open
  no pull requests; they hand a slice back on a pushed branch, and the board's
  approve action turns that branch into one with `gh pr create --head <branch>
  --fill`, run in the slice's repo. `--fill` because the summary of the work is
  already on the Notion page and a board key cannot answer a prompt; no
  `--base`, because gh's own answer — the repository's default branch — is the
  right one. `Runner` is the seam the tests replace, and it takes a working
  directory, which is the whole reason it is not `agent.Runner`. A gh that ran
  and refused comes back as an `*ExitError` whose message is the first line of
  its stderr, since "a pull request for branch X already exists" is the entire
  point of showing the failure.
- `internal/git/` — git, wrapped as thinly as gh is and for the same reason: the
  one thing the board asks of it is the diff of a slice's handed-back branch, so
  the work can be read before `p` turns it into a pull request. `Diff` runs one
  `git diff --merge-base <base> <branch>` in the slice's repo and hands back what
  git wrote alongside the base it measured against — the merge base rather than
  the tip, since what the branch did is the point and not everything main has
  moved on by. The base is whatever `refs/remotes/origin/HEAD` names and `main`
  when there is no such ref, which is logged and swallowed rather than returned:
  the fallback is the project's own convention, and refusing a diff over it would
  be worse than showing one against main. The prefixes are pinned and any
  external diff driver refused (`--src-prefix=a/ --dst-prefix=b/ --no-ext-diff
  --no-color`), because the output is parsed rather than shown as it stands and a
  repository configured with `diff.noprefix` would hand back something else.
  `ParseFiles` splits that output into `File`s — the paths, the ± tallies,
  whether git described the file rather than diffing it, and the section's lines
  verbatim, since the viewer is read-only and the shape it needs is the shape git
  already produced. The paths come from the `+++`/`---` lines rather than the
  `diff --git` header they follow, which pairs two paths with one space between
  them and cannot be split where a filename holds spaces of its own. `Runner` is
  its own rather than `gh.Runner`, because a package about git has no business
  importing the GitHub CLI to borrow a type off it.
- `internal/cli/` — the headless commands (`nat info [--json]`,
  `nat next-slice [--json]`, which claims the next unblocked Todo slice under
  the lowest-ordered open milestone and prints its brief,
  `nat start-slice <slice> [--json]`, which claims one named Todo slice and
  prints the same brief — the command a board-launched agent runs, since it is
  pointed at a slice rather than choosing one —
  `nat complete-slice <slice> [--branch NAME] [--pr URL] [--summary TEXT]
  [--blocked]`, which closes out a slice the configured user holds — three
  endings, and no two of them at once: `--branch` records the branch the work
  was pushed to and leaves the slice in progress, handed back for review, which
  is how an agent ends now; `--pr` records a pull request and marks the slice
  Done; `--blocked` leaves it in progress with a note saying what stopped it —
  `nat release-slice <slice>`, which is the fourth ending and the only one that
  goes backwards: Status to `Todo`, the Assignee cleared and one line on the
  page saying so, for a session that ended without finishing at all,
  and the one-off additions
  `nat milestone-add <name>` (Queued, at the end of the plan) and
  `nat slice-add <title> --milestone <name> [--description TEXT|-]
  [--repo DIR] [--depends-on <slice>]...` (Todo and unassigned, description as
  the page body; `--description -` reads it from stdin, so a slice-add typed
  with no brief does not wait on one), `nat slice-depends <slice> [--on
  <slice>]... [--clear]`, which records what a slice waits on — `--on` adds and
  every named slice is read first, since a dependency nobody can fetch is a wait
  with no end, and `--clear` drops what is there, so on its own it frees the
  slice and with `--on` replaces the list outright — the wishlist pair `nat wishlist [--json]`, which prints
  the pending items written under the project page's Wishlist heading (with
  their block IDs under `--json`), and `nat wishlist-clear <block-id>...`,
  which trashes exactly the named items — never the section wholesale, so an
  idea typed while a workshop session runs survives it — and leaves the section
  holding one empty bullet, and `nat plan-apply [FILE]`, which creates a whole
  drafted plan of milestones and slices from a JSON document (read from FILE or
  stdin, validated entirely before the first write, and only ever creating
  pages — its optional `depends_on` names slices by title, one the document
  creates or one already in the plan, and those relations go on last, since a
  slice may wait on one written further down and there is no page to point at
  until every slice exists), and `nat setup`, which installs the embedded skills into
  `~/.claude/skills` — the only command that talks to neither Notion nor the
  config file, since it is what a machine with only the binary runs first), what
  the binary does when given a subcommand. Run before the tmux hosting step and with no TUI
  code in the path: a command prints to the terminal it was typed in and exits.
- `internal/vterm/` — a child command run on a PTY (`x/xpty`) with its screen
  mirrored by an in-process VT emulator (`x/vt`), so the TUI can draw an agent
  as a widget instead of joining its tmux pane. `Start` returns a `Session`:
  `Render`/`Cursor` to draw it, `SendKey`/`SendBytes`/`Paste` to type at it and
  `SendMouse` to click and scroll at it — a cell, a button, the modifiers held
  and a kind of event (press, release, motion, wheel), which the emulator
  encodes for the child's active mouse modes, or drops where the child has asked
  for no reporting; the modifiers are not decoration, since tmux binds
  `C-MouseDown1Pane` as well as `MouseDown1Pane` and a click stripped of its
  ctrl fires the wrong one,
  `Resize`, `Output` to know when to redraw and `Done`/`Err` when it has gone.
  Two goroutines: the read pump feeds the PTY into the emulator under the
  Session's own mutex (`vt.SafeEmulator`'s lock is not trusted alone — an
  upstream race fix there was merged and reverted), and the reply pump drains
  the emulator's answers to the child's startup queries back to the PTY, which
  a child that asks DA1/DSR stalls without. Two seams for fakes, `newPty` and
  `waitProcess`. Three details the packages make you find out the hard way: the
  parent's copy of the PTY's child end has to be closed after `Start` or the
  read never ends; the screen is read only through `Render`, since the damage
  list `Draw`/`Touched` work from goes nil across a resize; and the emulator's
  input pipe is ended by closing it directly rather than through the
  emulator's own `Close`, which races the reply pump. `internal/tui/agentview.go`
  is its one caller.
- `internal/tui/` — bubbletea v2 (`charm.land/*/v2` imports); root model in
  `app.go` routes screens; all Notion I/O via tea.Cmd → typed msgs.
  `approve.go` is the `p` key, the board's one action that reaches outside
  Notion — the domain rule on `Branch` says what it does and why gh's failures
  are toasts. `diff.go` and `diffflow.go` are `v`, the key that is answered with
  it: the unified diff of the same branch, read through `internal/git` and drawn
  as a screen over the board like help and info, which is where the work is read
  before it is approved. It is read-only — nothing on it writes anything — and it
  holds the parsed files rather than the rendered body, because a body line is
  cut to the width it is drawn at and a resize renders again. One diff line is
  one body line, truncated rather than wrapped: the file jumps scroll to a line
  number, and a body whose lines did not correspond to git's would send them to
  the wrong place. The file list beside it is what `n`/`p` move through, and it
  goes entirely on a window under 60 columns, where the columns are worth more to
  the diff than to a list of paths — the jumps go on working either way, which is
  what per-file navigation actually needs. Lines are coloured by their shape
  rather than their syntax, the header lines tested before the +/- ones they look
  like, since `+++ b/main.go` is a header and not three added characters. A read
  that fails takes the diff it replaced with it, unlike the info screen's,
  because a diff is of one branch at one moment and leaving the last one up under
  a failure would be showing the wrong change; the refresh key reads the branch
  again while the screen is up, which is what an agent pushing another commit is
  worth asking about.
  `settings.go` is `S`, the config file as a form, so nothing local has to be
  edited by hand: the active project's working directory, the agent split, the
  poll interval and the two model pairs — and nothing else in the file, since
  the workspace's databases and the assignee are wiring the wizard and the
  project keys write rather than text to type over, and the Notion token is not
  in the config at all. The numbers are held as the strings they were typed as,
  so a field cleared back to empty is "unset" and not a zero to render; both are
  validated against `config`'s own bounds rather than a copy of them, so the
  form refuses exactly what a later read would have swapped the default in for.
  It is one huh group and not a section apiece, because huh pages a form group
  by group and a settings screen is somewhere the user arrives knowing which
  field they came for: every field is on the one page and tab reaches any of
  them, and what the two model pairs belong to is said in their titles
  (`modelFieldsFor`) rather than in a group heading above them.
  The key is global rather than the board's — the config is the app's, not any
  one screen's — and out of the hints row for the reason `W` is: it is pressed
  rarely and once. Saving applies to the session first and persists after, the
  bargain `persist` describes, and it re-shares the window on the spot, which is
  what makes the split live; the poll takes its new interval on the next tick,
  and the directory and the models are read at the next launch, which the
  fields' own descriptions say. `poll.go` is
  the background refetch of the plan, for the changes no nudge reports because
  no `nat` command made them: a tick every `poll_seconds` (default 30, bounded)
  running the same load the refresh key does, so the plan stays on screen while
  it is in flight and a failure leaves the board as it was. It is passed over —
  never cancelled, so it resumes on the next tick by itself — while the wizard,
  a form, a row prompt, a write or another load is in flight, since a plan
  landing under any of those would clobber what the user is in the middle of. An
  open agent terminal never suspends it: the plan behind the split stays live.
  `agentview.go` is that terminal — `t` (and `w` for the planning agent) runs
  `AttachClientCmd` on a `vterm.Session` and draws it in a box beside the board,
  sized from `agent_split_percent`, so nat owns the whole rectangle while the
  agent goes on living in its own tmux session and survives a board restart. One
  is on show at a time; it opens unfocused (`tab` focuses, `ctrl+\` comes back)
  and while it is focused every key is the agent's — `ctrl+c` included, which is
  deliberate — bar the outer tmux prefix, which is swallowed. A key that stands
  for characters is written as those characters and only the rest goes to the
  emulator's key encoder, which writes a printable key only when it carries no
  modifier at all and so typed nothing for a capital letter or any shifted
  punctuation; ctrl is excluded, since a ctrl combination is a control byte
  however it was decoded. It is drawn only on
  the board and only in a framed window, and stays alive undrawn behind help,
  info and a form. Its redraw is capped at the renderer's own frame: the wait
  re-armed after a capture (`awaitFrame`) holds off `frameInterval` — 1/60s —
  before listening again, and the session coalesces everything the child wrote
  meanwhile into its one pending notification, so a burst costs one read of the
  emulator's screen and one render of the window per frame instead of one per
  write. The board beside it is drawn once and kept (`App.boardBox`, dropped by
  `syncBoard`, which everything that changes what the board shows goes through),
  so an agent writing flat out redraws only its own box.
  An agent that exits takes its box with it: the terminal closes
  (`dropViewer` — the viewer dropped, the session closed, the board resized back
  to full width), because a frame nothing will write to again is the board's
  columns held by a dead agent. That happens whichever way the news arrives —
  the hidden client's EOF, or the live poll finding the session gone — and
  dropping the viewer is what the second one is recognised by, so the
  after-viewing refetch (the slice's page, or the whole plan for the planning
  agent) runs exactly once. The trigger is the end of the client's
  pseudo-terminal, not the status it exited with: `tmux attach-session` exits
  zero whether its session ended under it or the user detached. A client that
  dies with its session still running closes the box too, but says so in a toast
  naming the session, since reattaching is what the user wants next.
  `T` is the hatch to the old full-screen attach. The
  mouse is the terminal's too: all-motion reporting is asked for on the view
  only while one is on show — with it off the user's own selection and
  scrollback are left alone, and there is nothing on screen a key does not reach
  — and an event inside the box is turned into a cell of the child's screen and
  handed to `SendMouse` with the modifiers it arrived with, a press there taking
  the keyboard as well. Everything outside the box is dropped, bar a press, which
  hands the keyboard back. Nothing between swallows it: tmux passes mouse
  reporting through unless its own `mouse` option is on, which nat sets for the
  sessions it makes for agents and never for the one it hosts the board in. What
  the agent's own tmux does with what arrives is unchanged by the hop: a click on
  an OSC 8 hyperlink fires the `MouseDown1Pane` binding and opens it, and a wheel
  enters copy-mode over the pane's scrollback and leaves it again on the way back
  down (tmux's stock `copy-mode -e`), exactly as they did through a joined pane.
  `boardmouse.go` is the other half of that, since reporting takes the mouse off
  the outer terminal over the whole window rather than over the terminal's box:
  what the viewer does not take, the board does. A left click selects the row
  the line it landed on belongs to — `Board.RowAtLine`, off the same `rowLines`
  the cursor's own span and the animation's re-sync are measured from — and the
  wheel scrolls the plan three lines a notch, dragging the cursor no further than
  the nearest row still whole on screen, because `syncBoard` would otherwise
  scroll straight back to it. A click on the PR chip opens the pull request:
  `Board.LinkAt` walks the drawn row for the OSC 8 hyperlink covering that cell
  and the app hands the URL to the platform opener (`agent.URLOpener`), which is
  what the terminal itself would have done with the click had nat not taken it.
  The board is deaf to all of it while the wizard, a form or a row prompt is up,
  for the same reason the keys are.
  `presence.go` is the star a slice with an agent on it is marked with: it
  pulses — the same star swelling and settling, one cell wide at every frame so
  the row does not shift under it — while the agent works, and holds a star of
  its own steady when the agent has stopped for input. Liveness is the live
  map's answer and the classification only refines it, so an agent that has gone
  has no star whatever was last read of it, and one nobody has classified draws
  as working. The whole
  board animates off one timer, armed by the live read and stopping itself as
  soon as nothing is pulsing; each frame re-syncs the board, since its rows are
  cached in a viewport. Both the glyph and the colour move, because a selected
  row is drawn without any chip's styling.
  `activity.go` is what does the classifying: a reading of
  `agent.Tmux.Activity` every two seconds — far shorter than the live read's
  half a minute, because this is the reading a star moves with — turned into the
  presence map the board draws from. Like the pulse it runs off one timer, armed
  by a live read that finds an agent and stopping itself once the last one has
  gone, and it takes no reading at all with nothing running. A failed reading is
  logged and dropped rather than toasted or written to the board: the stars go
  on saying what they last said, which for an agent still running is true for a
  while yet. An agent read as gone is left out of the map entirely — whether
  there is an agent at all is the live map's answer, and a second opinion here
  would only be a staler one.
- `skills/` — the agent skills (/queue-work planning, /next-slice execution),
  embedded in the binary with `go:embed` and installed by `nat setup`. A
  checkout works on them in place by symlinking them into `~/.claude/skills/`,
  which `nat setup` leaves alone rather than writing back through. /next-slice
  and `internal/agent`'s slice prompt end the same way and say so in the same
  words: the branch pushed and handed back with `complete-slice --branch`, no
  `gh` and no pull request, since opening one is the board's `p` after the user
  has reviewed the branch.

## Domain rules

- Slice workflow: Todo → in progress → Done. Never edit an in-progress or Done
  slice.
- The in-progress status is called `In progress`. Projects made before the app
  asked called it `Claimed`; that name is migrated away at load
  (`notion.MigrateProject`) rather than read anywhere. `notion.ShapeOf` still
  reads the types of the `Status` and `Milestone` columns off the Slices data
  source — either may have been converted to Notion's own status type in the UI
  — and whether the project has an `Assignee` or a `Branch` column at all.
- Claiming = Status → `In progress`, plus Assignee (people
  property, the configured real Notion user) where the project has that column.
  Without one, ownership is decided on status alone.
- Releasing is the claim undone, and the way out of the one state a slice gets
  stuck in: a session that died — a crashed agent, a killed pane, a context that
  ran out — leaves its slice in progress and held, where `next-slice` steps over
  it, `start-slice` refuses it and `complete-slice` only goes forward. Status
  back to `Todo` and the Assignee cleared (`NewPeople()` with no user, the empty
  list Notion reads as nobody, which is why `PropertyValue.People` is a pointer
  — the same reason `Relation` is), and nothing else on the page touched: the
  description, `Depends on`, `Repo` and any `Branch` are exactly the brief and
  the work-so-far the next session wants. One line goes on the page first, so a
  slice that went round twice reads as having done so; it is written before the
  status for the reason `complete-slice` writes its note first, since a slice
  already back at `Todo` is one the command would refuse to add a line to. Only
  a slice the configured user holds can be released — the same ownership rule
  `complete-slice` applies (`notOursError`, which now names the action it
  refused) — and the page is re-read for the type of its `Status` column, as
  every other write to a slice is. `nat release-slice <slice>` is the headless
  half; `R` on the board is the other, confirmed on the row the way `p` is,
  ignored on a slice that is not in progress, and refused with a toast on a
  slice an agent is still live on, since releasing one out from under a working
  session is how two sessions end up on one branch.
- Slices ↔ PRs are 1:1 when work is code; PR URL recorded in the `PR` property.
- A slice may carry a `Branch` — the branch an agent pushed its work to and
  handed back on. It is read off the page like any other property and empty on a
  project whose Slices table has no such column, and a slice in progress that
  names one is work waiting to be reviewed (`domain.Slice.HandedBack`), which
  the board draws as a green `↑ review` chip so a hand-back does not read as
  another slice being worked. `complete-slice --branch` is what writes it: the
  branch recorded, the status left alone, and the note filed under a
  `Handed back` heading rather than `Summary`. A branch is refused outright
  where `notion.ShapeOf` reads no `Branch` text column — a hand-back written
  nowhere is one lost — and refused before the note goes on, so the slice is
  left as it was. `v` on the board is how that wait is read — the branch's diff
  against the base it was cut from, on a screen of its own — and only a
  handed-back slice has one to read, the same rule and the same refusals `p`
  applies. `p` on the board is what ends that wait: it confirms on the row, runs `gh pr create` in the
  slice's repo from that branch, and writes the URL it gets back onto the `PR`
  property as it sets the status to `Done` — the one board key that reaches
  outside Notion, and the only place a slice's PR is recorded from the TUI. The
  page is re-read first for the type of its `Status` column, exactly as
  `complete-slice` does. gh refusing is a toast naming its own reason, not an
  error banner: the branch is still there and the slice is still handed back. A
  pull request opened and then not recorded is the one half-done state there is,
  and running the key again says so rather than opening a second one, since gh
  refuses a branch that already has one.
- Slices may carry a `Repo` override; otherwise the project default working
  dir from local config applies.
- A slice may declare the slices it waits on: `Depends on`, a single-property
  relation from the Slices data source to itself — single so there is no
  reciprocal `Blocks` column to keep in step — created alongside the other
  columns, though as a second write, since a self-relation cannot name a data
  source the create has not returned yet, and back-filled at load onto a project
  created before there was one. The rule is one line
  (`domain.Blockers`): a slice is blocked while any slice it names is not Done,
  and a slice names none where the column is absent or empty, so a project whose
  table has no such column behaves exactly as it did before there was one. A
  dependency whose page cannot be read is logged and passed over rather than
  counted, because a trashed slice must not wedge the plan forever. The two
  commands that hand work out are what honour it: `next-slice` steps over a
  blocked slice rather than stopping at it — the work below may well be ready —
  and says which slices wait on what only when every candidate is blocked, and
  `start-slice`, which was pointed at one slice and so has nothing to skip,
  refuses it by name and lists what it waits on, before it claims anything. The
  board honours it too: it indexes the blocked slices of the plan whenever one
  is loaded (`Board.Blockers`, off the whole plan, since a dependency is another
  row of the same board), draws a muted `⊘ blocked` chip on such a row — first
  of the row's chips, since it says the row cannot be worked at all — and
  refuses `l` on one with a status-bar toast naming what it waits on and how far
  off each is. A toast rather than an error banner: nothing has gone wrong, and
  the slice is still there to launch once its dependencies land.
- A project keeps its whole plan on one page: no Milestones database, and a
  `Milestone` **select** on the Slices data source whose options are the
  milestones, in plan order. `domain.MilestonesFromOptions` maps them —
  milestone name as ID, option index as order — so a milestone is nothing but
  its name. It has no status of its own: `domain.NewProject` computes it from
  the slices under it — Queued until one starts, Active while any is in progress
  or only some are Done, Done when they all are — so there is nothing on a
  milestone to write, and the board has no key that would.
- A project still in the old shape — a Milestones database with a `Milestone`
  relation on the Slices data source, and/or a `Claimed` status option — is
  migrated in place the first time it is loaded, by the board and by every
  headless command alike (`notion.MigrateProject`, run from `tui.fetchProject`
  and `cli.slicesDataSource`): the milestone pages become the options of a
  `Milestone` select, in plan order; every slice is refiled onto the option its
  relation named; and the Milestones database, its plan now wholly on the
  column, goes to Notion's trash — recoverable, and only after everything else
  has succeeded. The Slices database itself — a full-page child of the project
  page, its first view the table the plan's order is read from — is left
  exactly as it is. `Claimed` becomes `In progress` the long way — appended,
  the slices holding it moved over, then dropped — because the API silently
  ignores renaming an option in place (a 200 whose body still says the old
  name). The whole migration is settled before the first write — a `Status`
  column converted in the Notion UI cannot have its options written, and such a
  project is refused with the one edit to make there rather than half-migrated,
  as is one whose milestones data source names no database to trash — and the
  plan is read in full before the schema changes, because converting the column
  is what discards the relations it is read from. It is idempotent: a project
  already in the one shape is read and left alone, which is what every load
  after the first does. What changed is logged, and the board says so in a
  toast. One step there is every project's rather than an old-shape project's:
  a missing `Depends on` or `Branch` column is added (`addColumns`).
  `CreateProject` writes both, but only for the projects it creates, so a
  project older than slice dependencies, or than handing work back on a branch,
  has nothing for one to be recorded on and Notion refuses every write against
  it — and they go on last, in one write, after the shape changes and whether or
  not there were any, so a project refused part way through them is left as
  those steps found it.
- Filing a slice under a milestone — the board's `a` and `m`, `slice-add`,
  `plan-apply` — writes the option naming it (`domain.Milestone.Ref()`, sent as
  `SelectType`, the column's own type). `milestone-add` and `plan-apply` add
  milestones by appending options to the `Milestone` column, one schema write
  per run, after the options already there, since their order is the plan's
  order. A name the plan already holds is refused before that write, because
  such a milestone is nothing but its name — and for the same reason a milestone
  is named by name alone, never by URL or ID. `next-slice` reads the plan the
  way the board does and takes work from the lowest-ordered milestone that is
  not Done: a milestone is Queued until a slice under it starts, so gating on
  Active would leave a plan on which nothing has begun with no way to begin.
  `start-slice` names a slice's milestone from the schema's options, and
  `complete-slice` touches no milestone at all.
- Slice order is where the slices sit in the project's own board:
  `notion.PlanOrder` reads the row order of the Slices data source's first view
  (`GET /views`, then `POST /views/{id}/queries`, which is exposed from
  `2026-03-11`) and `domain.InViewOrder` applies it, with anything the view does
  not name trailing in the order it was queried. A failure to read it is logged
  and the plan drawn unordered rather than not at all. Notion records
  `created_time` only to the minute, so the created-time sort every other read
  uses is no order at all for a plan written in one go — which is why this one
  is read. `next-slice` reads it too, so the slice it hands out is the one at
  the top of the milestone on the board rather than whichever the query happened
  to return first.

## Conventions

- Bubble Tea v2 idioms: `View()` returns `tea.View`; match `tea.KeyPressMsg`;
  `tea.ExecProcess` for tmux attach.
- Tests: aim for 100% coverage of new code. httptest for the Notion client
  (assert exact request JSON), interfaces + fakes for the ntn CLI/tmux, teatest
  for TUI flows, golden snapshots for renders.
- Gate before claiming done: `go vet ./... && go test -race -cover ./... &&
  golangci-lint run`. All three run in CI, so a failing lint fails the PR;
  `brew install golangci-lint` if the binary is missing. `.golangci.yml` runs
  the default linter set with one exclusion — see the file — so an unchecked
  error is either handled or assigned to `_` where the reason can be read.
- Never log or commit the Notion token; it belongs to the `ntn` CLI and is only
  ever held in memory for the lifetime of a request.
- Before starting work, pull the latest `main` and branch off it. Only ever
  base branches — and PRs — on `main`, never on another slice branch, so every
  PR merges into `main`.
