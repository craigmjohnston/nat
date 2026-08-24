# notion-agent-tracker

A Go TUI over Notion for tracking project work executed by Claude Code agents.
Craig manages the plan and launches agents from the TUI; agents run as fresh
`claude` sessions in tmux and reach the tracker only through the headless `nat`
commands — they need no Notion access of their own. The TUI talks to the Notion
REST API directly (`Notion-Version: 2026-03-11`, data-source model).

## Architecture

- `main.go` — entrypoint; module path `github.com/craigmjohnston/nat`, so
  `go install github.com/craigmjohnston/nat@latest` yields a `nat` binary.
  It runs in the terminal it was started in and hosts itself in nothing: the
  status band is drawn inside nat's own frame and the agent terminal beside the
  board is nat's own widget, so there is nothing a session of its own would
  provide, and inside a tmux session the user made it behaves exactly the same.
  The agents still live in tmux — that is what lets one outlive the board and be
  attached to again — and the server they need is the one the first detached
  launch starts, so all `requireTmux` does for them is check the binary is on
  PATH before the terminal is taken over, naming `brew install tmux` when it is
  not. A subcommand runs before even that: none of them launches an agent.
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
  writes nothing back — the planning form only once `ctrl+o` has asked for it,
  since a workshop launch nearly always wants the pair the config already
  names. `W` is the exception, as it always is — it asks nothing
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
  themselves; startup failures name the log path on stderr, because a terminal
  that closes with the process takes stderr with it. Everything on its
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
  pre-upgrade nat left joined, and comes out next release — `TUISession` is
  kept for it alone, the name of the session nat used to host itself in and
  makes no more.
  Every session nat makes is an agent's, and has tmux's own status bar off: the
  bar says nothing its session does not. The sessions the user was already in
  are never nat's to set options on, nat's own terminal included.
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
  `SendPrompt` is the one thing nat says to a running agent rather than about
  one: the diff screen's review comments, typed at the session's pane and
  submitted. It goes through a paste buffer of the session's own
  (`set-buffer`, then `paste-buffer -d -p`) rather than send-keys' literal
  mode, because a prompt is several lines and keys sent one at a time would
  submit at the first newline; the enter after the paste is what sends the turn,
  and the bracketing is how Claude Code's composer tells a pasted newline from a
  typed one. A paste that never happened takes its buffer back off the server,
  and the text is never logged — a review comment is the user's own words about
  their own code.
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
  approve action turns that branch into one with `gh pr create --head <branch>`,
  run in the slice's repo, titled and bodied with the description the agent
  recorded at hand-back and read back off the slice page. A hand-back that left
  none — every one written before there was a flag for it — falls back to
  `--fill`, which is what the key always did: nothing is ever asked for at a
  prompt, since a board key cannot answer one. No `--base`, because gh's own
  answer — the repository's default branch — is the right one. `Runner` is the seam the tests replace, and it takes a working
  directory, which is the whole reason it is not `agent.Runner`. A gh that ran
  and refused comes back as an `*ExitError` whose message is the first line of
  its stderr, since "a pull request for branch X already exists" is the entire
  point of showing the failure. `OpenPRs` is the one thing it reads rather than
  writes: `gh pr list --state open --json url,reviewDecision,mergeable --limit
  100`, in the slice's repo, answering with every pull request the repository
  currently has open — keyed by URL — and, of each, whether a review has approved
  it and whether GitHub can merge it as it stands. One listing per repository
  rather than one `pr view` per pull request, because the board takes this
  reading for every slice that has a pull request recorded, a whole project's
  Done ones included: a view per slice would grow with the plan forever and a
  listing does not grow at all. Being in the answer is itself a fact the caller
  reads — a merged or closed pull request is simply not listed — which is why a
  gh that failed is logged and handed straight back, as is output that is not the
  JSON it was asked for: nothing may be concluded from a listing that never
  happened. Three fields rather than the whole pull request, since the rest is
  JSON nobody here reads; GitHub's own words are decoded where they are known —
  only `APPROVED` and `MERGEABLE` count, and everything else it says
  (`REVIEW_REQUIRED`, `CHANGES_REQUESTED`, `CONFLICTING`, `UNKNOWN`, and the
  empty decision of a repository that requires no review) is the fact not being
  true. The limit is past gh's own default of thirty, and a repository with more
  open than that has its oldest left out, which reads as a pull request no longer
  open — the same thing an unread one reads as, and the quiet direction to be
  wrong in. `NormaliseURL` is how a URL typed onto a Notion page finds the
  canonical one gh prints: the query string or fragment of a link copied from a
  review or a comment, a trailing slash, and the case of an owner or repository
  are none of them distinctions GitHub makes.
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
  `Show` is the second thing it asks, and only because a unified diff is a few
  lines of context around each change and nothing else: `git show
  <branch>:<path>` is the whole file at the branch, which is the only place the
  lines between the hunks can come from. Textconv is refused for the reason the
  diff refuses an external driver — the lines are lined up against the diff's own
  numbers rather than shown as they stand — and a file the branch does not have,
  which is one the change deleted, comes back as git's own refusal, logged and
  handed on: what it costs is the expanding around that one file's diff.
  `ParseFiles` splits that output into `File`s — the paths, the ± tallies,
  whether git described the file rather than diffing it, and the section's lines
  verbatim, since the viewer is read-only and the shape it needs is the shape git
  already produced. The paths come from the `+++`/`---` lines rather than the
  `diff --git` header they follow, which pairs two paths with one space between
  them and cannot be split where a filename holds spaces of its own. `Runner` is
  its own rather than `gh.Runner`, because a package about git has no business
  importing the GitHub CLI to borrow a type off it.
- `internal/worktree/` — the worktrunk CLI (the `wt` binary), wrapped as thinly
  as `internal/gh` and `internal/git` are and with a `Runner` seam of its own
  for the same reason, so a slice's agent can be given a git worktree rather
  than made to share the project's one checkout with every other agent and with
  the user. Two operations: `Create` runs `wt switch --create <branch> --no-cd
  -y` in the repo — no directory to change, since a subprocess reaches the
  binary under worktrunk's shell function, and nobody at this end to answer an
  approval — and reads the path back through `Path` (`wt list --format json`),
  because switch prints for a person and the list is the one machine-readable
  answer worktrunk gives; `Remove` runs `wt remove <branch> -y` and leaves what
  removal means to worktrunk, which deletes the branch only if merged and
  refuses a dirty worktree outright. That refusal is synchronous — only the
  removal itself is backgrounded — so a nil error means the worktree is going
  rather than already gone. A wt that ran and refused comes back as an
  `*ExitError` whose message is the first line of its stderr, exactly as gh's
  does; a wt that is not there at all comes back as `ErrNotInstalled`, wrapped
  around `exec.ErrNotFound` rather than replacing it, because that is the one
  failure a caller recovers from — an agent on a machine without worktrunk runs
  in the shared checkout the way every agent did before there were worktrees,
  and only a distinguishable error can drive that.
- `internal/cli/` — the headless commands (`nat info [--json]`,
  `nat next-slice [--json]`, which claims the next unblocked Todo slice under
  the lowest-ordered open milestone and prints its brief,
  `nat start-slice <slice> [--json]`, which claims one named Todo slice and
  prints the same brief — the command a board-launched agent runs, since it is
  pointed at a slice rather than choosing one —
  `nat complete-slice <slice> [--branch NAME] [--pr URL] [--summary TEXT]
  [--pr-description TEXT|-] [--blocked]`, which closes out a slice the
  configured user holds — three
  endings, and no two of them at once: `--branch` records the branch the work
  was pushed to and leaves the slice in progress, handed back for review, which
  is how an agent ends now; `--pr` records a pull request and marks the slice
  Done; `--blocked` leaves it in progress with a note saying what stopped it.
  `--pr-description` belongs to the first of those alone — the only ending with
  a pull request still to open — and is filed on the page under a `PR
  description` heading beside the `Handed back` note, where it outlives the
  agent's session and is what the board's `p` opens the pull request with days
  later. `-` reads it from stdin, so a description too long for an argument
  gets in; there is one stdin, so `--summary` is then the flag —
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
  stdin, validated entirely before the first write, and creating pages and
  nothing else bar one relation — its optional `depends_on` names slices by
  title, one the document creates or one already in the plan, and those
  relations go on last, since a slice may wait on one written further down and
  there is no page to point at until every slice exists; the document's
  optional top-level `dependencies` list — `{"slice": <title>, "on":
  [<titles>]}` — is how it reaches a slice already on the board, added to what
  that slice already waits on the way `slice-depends --on` is and written in
  that same last phase, and it is the one thing a plan changes rather than
  creates, which is why a document may hold it and nothing else), and `nat setup`, which installs the embedded skills into
  `~/.claude/skills` — the only command that talks to neither Notion nor the
  config file, since it is what a machine with only the binary runs first), what
  the binary does when given a subcommand. Run before even the tmux check and
  with no TUI code in the path: a command prints to the terminal it was typed in
  and exits.
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
  The window is four bands, shared out from the bottom: the status band, the
  boxed header, the hints row and the body with what is left. The status band is
  the last of them — the mode of a screen over the board (the board itself has
  no chip: its heading names the app), the error or toast waiting, and the
  standing indicators — drawn inside a border like the header and the body and
  in the frame's own colour, with no fill of its own, so nat's bottom border is
  the last line of the terminal. Below the framed threshold every band is drawn
  bare, the status line included, and a window of one line is that line alone.
  The same text goes out as the terminal's title, stripped of its styling, since
  a title is text.
  `worktrees.go` sits between the launch flow's `workdirFor` and the session it
  starts: a slice's agent is given a worktree of its own through
  `internal/worktree`, so it works on its own branch in its own directory rather
  than sharing the one checkout with every other agent and with the user. The
  branch is derived rather than recorded — `slice/<the title slugged>`, since
  nothing holds a branch name until the agent hands the work back and a relaunch
  has to arrive at the same string — and a branch that already has a worktree is
  reused rather than cut a second one, because a relaunched slice wants its work
  so far. The path it answers with is the `agent.PromptContext.WorkingDir` the
  session is started in and the prompt is written from, so tmux and the agent
  never disagree about where it is. Two ways out fall back to the shared
  checkout with a toast saying which — no worktrunk on the machine, and a
  working directory that is in no git repository at all — since both are the
  launch that worked before there were worktrees, and where the agent is working
  is what decides what its branch instructions mean. A worktrunk that ran and
  refused is a toast too and launches nothing at all, because an agent placed
  half way is one working somewhere nobody chose. All of it is resolved inside
  the launch command rather than before it: cutting a worktree runs the
  repository's own hooks, and that is the goroutine to be slow in. Its
  `Worktrees` seam is the board's whole dealing with worktrunk rather than the
  launch's alone: `Remove` is on it too, for the approve key that takes the
  worktree away again once the work has become a pull request.
  `approve.go` is the `p` key, the board's one action that reaches outside
  Notion — the domain rule on `Branch` says what it does and why gh's failures
  are toasts. `diff.go` and `diffflow.go` are `v`, the key that is answered with
  it: the unified diff of the same branch, read through `internal/git` and drawn
  as a screen over the board like help and info, which is where the work is read
  before it is approved. It is read-only — nothing on it writes anything — and it
  holds the parsed files rather than the rendered body, because a body row is
  wrapped to the width it is drawn at and a resize renders again. A line too wide
  for the box takes as many rows as it needs rather than being cut off, since the
  tail of a long line is often what changed; `bodyLine.seg` is what tells a row
  that continues a line from one that starts one, so the cursor stops only where
  a line begins, a range covers whole lines, and the +/- colour carries onto a
  continuation while its numbers do not. The file jumps, the line cursor and the
  comments are all row numbers into that body, so `render` records where each
  file opens after the wrapping rather than before, and every one of those
  numbers is rebuilt with the body — the cursor and any range mark put back on
  the line they were on rather than the row it used to be at.
  `diffbox.go` is the shape the body takes: one bordered box per
  file, GitHub-fashion — a header row naming the path with its ± tally, the
  file's diff inside it, a footer row closing it, and the old and new
  line numbers of every line down the left, read off the same hunk headers
  `diffref.go` names a comment's lines by, so the gutter and the prompt are one
  answer. What git wrote about the file rather than in it is not drawn at all
  (`diffnoise.go`): the `diff --git` line, the `index` line and the `---`/`+++`
  pair are what the header row already says, and a hunk header is what the
  gutter already carries — the first of a file goes silently and every later one
  leaves a dashed break across the box, so the numbers jumping is not the only
  sign that lines were skipped — or, where the file behind the diff could be
  read, the expand controls `diffzones.go` puts there instead, which say the same
  thing and offer the lines besides. Only the render skips them: parsing keeps every
  line, so the numbers, the lines a comment quotes and the anchors a re-read
  finds them by are all read off the section as git wrote it, and a body row
  that is a line at all is still one of its lines. The number columns are as wide as the widest number anywhere in the
  diff, so code starts at the same column in every box; a box's own two rows
  belong to no file's section, which is what the line cursor steps over — bar
  the header row of a file with no line drawn under it, collapsed or all
  headers, which is all that file has — and what a
  jump scrolls to, since the row that names the file is worth the line it
  costs. A binary or otherwise described file keeps the one line git wrote for it
  inside its box like any other. The file list beside it is what `n`/`p` move
  through, and it
  goes entirely on a window under 60 columns, where the columns are worth more to
  the diff than to a list of paths — the jumps go on working either way, which is
  what per-file navigation actually needs. A line's shape is read off its prefix
  (`lineShapeOf`), the header lines tested before the +/- ones they look like,
  since `+++ b/main.go` is a header and not three added characters, and answered
  three ways: the colour it is drawn in, the wash it is drawn on, and whether it
  holds code to lex at all.
  `diffsyntax.go` is that lexing — the content of a line coloured by the language
  of the file it belongs to, chroma's lexers matched on the path (chroma is in
  the module graph either way, pulled in by glamour). It sits inside the diff's
  own +/- colouring rather than instead of it: an added line whose foreground has
  gone to the syntax says it is added with a wash under the whole row instead —
  the palette's `SuccessWash`/`DangerWash`, a fifth of the outcome colour mixed
  into the base — and the +/- itself keeps the green or the red. A file whose
  language chroma does not know, and one git described rather than diffed, takes
  no wash and is drawn exactly as the viewer drew everything before there was
  any highlighting, so what falls back falls all the way back. The colours are
  few on purpose — text, comment, keyword, string, number, and the names a file
  declares — and come from `Styles` like everything else, so the screen restyles
  with the light and dark palettes; the strings take the pending yellow rather
  than the green they take in most themes, since the green is what an added line
  is. A file is lexed once, when the branch is read, and into token kinds rather
  than styles: the render runs on every cursor move, and a palette swapped under
  the screen is picked up without a re-lex. Wrapping is over those runs
  (`wrapRuns`, which `wrapLine` is one unlexed run of), so a highlighted line
  takes exactly the rows an unhighlighted one would and `Diff.offsets` — what
  `n`/`p` scroll to — is unmoved by any of it. A read
  that fails takes the diff it replaced with it, unlike the info screen's,
  because a diff is of one branch at one moment and leaving the last one up under
  a failure would be showing the wrong change; the refresh key reads the branch
  again while the screen is up, which is what an agent pushing another commit is
  worth asking about.
  `diffzones.go` is what fills those skipped lines back in, GitHub's expand
  controls as box rows: every gap a file's hunks leave — above the first, between
  each pair, below the last — is a zone, measured off the hunk headers and, for
  the last one, off the file itself, since the diff says where its hunks end and
  nothing about how much file follows. A zone draws a control offering the next
  fifteen of its lines and, only where fifteen will not finish it, a second
  offering the whole gap; `enter` on the row under the cursor and a left click
  both activate one, and the render is what drops a control whose gap is full. A
  revealed line is drawn with its numbers on both sides and no ± of its own,
  since a gap is context and every line in it is on both sides — the base's
  number is the branch's plus the offset the hunks above have accumulated. It is
  coloured by the file's own language like every other line in the box, which is
  why `fileSyntax` keeps the lexer it lexed with: a revealed line comes from the
  file rather than from the diff, so it was not lexed when the branch was read
  and the render is where it goes through the same lexer. Every
  zone but the last reveals upwards, towards the hunk below it, and says so with
  `↑`; the gap after the last hunk has no hunk below to reveal towards, so it
  reveals down and out of the change, and draws `↓`. The lines are the file's
  rather than the diff's, so the cursor steps over them the way it steps over a
  comment's rows and there is nothing on one to comment on — the control itself
  is the one row that is no line at all and still a place the cursor stops. What
  has been expanded is the screen's own, like a fold, and a read of the branch
  measures its gaps afresh. A file git would not show — one the change deleted, a
  binary one — has no zones and draws exactly as it did before there were any,
  hunk breaks and all.
  `diffmouse.go` and the fold beside it are how a file that has been read is put
  away, GitHub's viewed checkbox as a box row: `enter` on the file the cursor is
  in — or a left click on either of its box's own two rows — collapses it to its
  header row alone, ticked where the rule would be, and does it again in
  reverse. That header is then the one place in the file the cursor stops, so
  `j`/`k`, `n`/`p` and the file list all move over a collapsed file and land on
  it, and `v`/`c` find nothing to say about a row that shows no lines. It is the
  screen's own state and nothing else's — never written anywhere, and dropped
  entirely by any read of the branch, since a fold says the user has seen what
  was there and that is exactly what a fresh read may have changed. A pending
  comment on a folded file is untouched: it is about lines, which are still
  there. The screen asks for the mouse while it is up (the button events alone,
  where the agent terminal wants all motion), which is what takes the wheel off
  the outer terminal, so the wheel scrolls the diff from here too.
  `diffcomment.go` and `diffref.go` are the review left on what that screen
  shows: `j`/`k` are a line cursor rather than the scroll they were, `v` marks
  the other end of a range — never leaving the file it was started in, since a
  comment across two files is two comments — `c` opens a huh box on the lines
  under it, prefilled with whatever was said about them before and emptied to
  take it back, and `s` hands every pending comment to the agent as one prompt.
  They are ephemeral by design: held in the session alone, never written to
  Notion or to GitHub, marked in a gutter column the body reserves on every line
  and cleared once they have actually reached the pane — a send that failed
  leaves them where they are, because nothing else is holding them. A comment is
  also drawn where it was left: its text, wrapped to the box and started at the
  column the code starts at, on rows under the last of the lines it covers, so a
  review is read in place rather than behind a mark. Those rows are a third kind
  of row that belongs to no file's section (`boxCommentRow`) — the line cursor
  steps over them the way it steps over a box's borders, `v` and `c` say nothing
  about them and a click on one folds nothing — and they are built with the body
  on every render like the wrapped rows and the fold offsets, so a resize wraps
  them again and a re-read draws them under wherever their lines have got to, or
  not at all where it dropped the comment. The prompt
  names each comment's lines by the numbers they sit at in the file
  (`diffref.go`, read off the hunk headers — the new side's, or the base's for a
  run that is nothing but deletions) and quotes them as git wrote them, since the
  numbers are what an agent opens the file by and the text is what tells it it
  has landed in the right place. A re-read of the branch carries a comment onto
  the lines it was left on wherever they have got to, and drops it — saying so —
  when they have changed or now occur twice; a fresh read of the same slice keeps
  them, and any other slice starts with none. `openForm` remembers the screen it
  was opened over for this one form's sake: every other form is the board's and
  closes back onto it, where dropping the user there after typing a comment would
  lose their place in a change they are half way through reading.
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
  and while it is focused every key is the agent's — `ctrl+c` and tmux's own
  prefix included, which is deliberate: nat sits in no session of its own for a
  prefix to drive by mistake, so it belongs to the agent's. A key that stands
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
  sessions it makes for agents and never for one the user started nat in. What
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
  `active.go` is the Active panel: the slices in flight, drawn as a vertical
  list in a box of its own above the plan's box rather than scattered through
  the milestones they happen to be filed under. The two are siblings of the
  body band, each framed the way the header and the status band are, so no
  border of the layout sits inside another — `App.bodyPanels` is where the band
  splits, `App.activeRegion` the panel itself, built like the agent terminal's
  region (a hand-made title line over a box drawn without its top border, since
  lipgloss has no border-title API). Membership is `domain.StateOf`'s own
  answer — a slice it says nothing about is one there is nothing to say about —
  so the section holds every slice in progress, plus a Done slice for as long as
  gh says its pull request is still open, and nothing else. A plan with none
  draws no panel at all and reads exactly as it did before there was one. An
  entry is two lines: a state dot and the slice's name, then a muted line
  reading `<state> · <milestone>`, with the dot and the state word in the
  state's own colour (`Board.stateStyle`, the roles the board already reads
  those states in). The selected entry is a fill rather than a marker, merged
  into every piece's own style through `wash` so the dot keeps its colour over
  it — the board's usual trick of drawing a selected row plain would flatten
  exactly what the entry is read by. The entries are rows of this same board,
  the first rows there are, so the cursor runs from the section straight on into
  the plan and every key that acts on a slice acts on the one under it —
  `Board.SelectedSlice` answers for an entry as for a plan row, since it is the
  same page drawn a second time. Everything the board measures over its rows is
  measured over the plan's alone (`Board.rowLines`, `CursorSpan`, `RowAtLine`,
  `CursorToVisible`, `LinkAt`), and the panel answers for its own in the lines
  of its own box (`ActiveLines`, `ActiveCursorSpan`, `ActiveRowAtLine`); the two
  scroll independently, the plan in `App.boardVP` and the panel on
  `App.activeOffset`, so a cursor in one says nothing about where the other is.
  The wheel is the plan's wherever it lands, since the panel follows the cursor
  rather than scrolling on its own. How many lines the panel gets is the
  layout's (`App.activeBandHeight`): as many as its entries need, never more
  than leaves the plan a band worth drawing in, and none at all where there is
  no room — which `Board.SetShowActive` tells the board, so the entries take no
  rows either and the cursor is never left on one nothing draws. Below the
  framed threshold the section follows every other band and draws bare: its
  heading on a line of its own where the panel has one let into its border.
  Nothing folds: an entry is a slice, and a slice row has never folded.
  `SetPRState` is the one reading that can change which slices the section holds
  at all, so it rebuilds the rows — but only when the reading says something the
  last one did not, since a rebuild the user cannot see is one the cursor pays
  for, and it puts the cursor back on what it was on either way: the slice for an
  entry of the section, whose position is exactly what the rebuild moves, and the
  row itself for anything in the plan below it.
  `prstate.go` is the second reading behind those states, beside the activity
  watcher's: what GitHub currently has open, read through `internal/gh` and kept
  as a map of `domain.PRReadiness` the board is given like the activity map. It
  has no timer of its own — it rides the plan's, kicked off by each plan that
  lands, since a pull request being approved is news of the same kind and much
  the same age as the plan itself — and it is skipped entirely when the plan has
  no pull request left to ask about, which is most boards most of the time. Two
  kinds of slice are asked about and for the same reason: one in progress is
  waiting on the review, and a Done one is waiting on the merge, since the board
  marks a slice Done as it opens the pull request and the work is not on main
  until that lands. It is one `OpenPRs` listing per repository the plan spans
  rather than one reading per slice, so the cost is the number of repositories
  rather than the number of pull requests the project has ever produced, and a
  pull request the listing no longer names is settled for the session
  (`App.prSettled`) and never asked about again — a merged pull request does not
  unmerge, and a mature plan is mostly finished work. One reading runs at a time
  (`App.prReading`), because a gh on a slow network can outlast the interval it
  was started on. A repository whose listing failed has every slice of it left
  out of the map, and settles nothing at all: the board reads an absent slice as
  a review still to come while it is in flight and as nothing whatever once it is
  Done, which is exactly what each said before there was any reading, so a gh
  that is not installed or not authenticated changes nothing and is logged and
  nowhere else — and, above all, a listing that never happened is never taken for
  a pull request that has landed. `readinessOf` is where gh's vocabulary becomes
  the rule's, the way `agentPresence` is for tmux's.
- `skills/` — the agent skills (/queue-work planning, /next-slice execution),
  embedded in the binary with `go:embed` and installed by `nat setup`. A
  checkout works on them in place by symlinking them into `~/.claude/skills/`,
  which `nat setup` leaves alone rather than writing back through. /next-slice
  and `internal/agent`'s slice prompt end the same way and say so in the same
  words: the branch pushed and handed back with `complete-slice --branch`, its
  `--pr-description` written ready to publish rather than as a report of the
  session, and no `gh` and no pull request, since opening one is the board's `p`
  after the user has reviewed the branch. They arrive at the same branch too: a board launch
  puts its agent in a worktree already on one and names it, and /next-slice is
  run by a session that is wherever the user was, so it cuts that worktree
  itself — `wt switch --create slice/<the title slugged> --no-cd` in the working
  directory the brief names, the slug rule `tui.sliceBranch` applies written out
  rather than shared, since a skill is read by an agent and not compiled.
  A machine with no worktrunk and a working directory in no repository fall
  back to branching in place, the two the board falls back to the shared
  checkout for.

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
  `Handed back` heading rather than `Summary` — with whatever
  `--pr-description` was given beside it under a `PR description` heading of its
  own, in the same write, since the two are one hand-back. That is where the
  description lives rather than only in the command that carried it, so it
  outlasts the agent's session and the approve can happen whenever the review
  does; `notion.PRDescriptionOf` is the read, taking the last such section, as a
  slice handed back twice has one per hand-back. A branch is refused outright
  where `notion.ShapeOf` reads no `Branch` text column — a hand-back written
  nowhere is one lost — and refused before the note goes on, so the slice is
  left as it was. `v` on the board is how that wait is read — the branch's diff
  against the base it was cut from, on a screen of its own — and only a
  handed-back slice has one to read, the same rule and the same refusals `p`
  applies. A review can go back rather than only be read: comments left on the
  diff's lines are sent to the agent that wrote the branch, all of them in one
  prompt, which needs that agent's session to still be running — one that has
  exited is not there to be told anything, and the comments stay pending until
  there is one that is. They are recorded nowhere: not on the slice, not on
  Notion, not on GitHub.
  `p` on the board is what ends that wait: it confirms on the row, runs `gh pr create` in the
  slice's repo from that branch, and writes the URL it gets back onto the `PR`
  property as it sets the status to `Done` — the one board key that reaches
  outside Notion, and the only place a slice's PR is recorded from the TUI. What
  it opens the pull request with is read off the slice page first: the last `PR
  description` section, its first line the title and the rest the body, or
  nothing at all for a hand-back that recorded none, which is gh's `--fill`
  again. A read that fails stops the approve rather than falling back, since a
  pull request opened under the wrong title is not one this key can open twice.
  The page is re-read for the type of its `Status` column before the write, exactly as
  `complete-slice` does. gh refusing is a toast naming its own reason, not an
  error banner: the branch is still there and the slice is still handed back. A
  pull request opened and then not recorded is the one half-done state there is,
  and running the key again says so rather than opening a second one, since gh
  refuses a branch that already has one. Once that write has landed the slice's
  worktree goes with it — `Worktrees.Remove` on the branch the slice was handed
  back on, in the same repo gh ran in, and only then, since a slice still handed
  back is one whose work is still being reviewed. The branch is read off the
  slice rather than derived the way the launch derives it: what the agent pushed
  is what its worktree is on, whatever it was cut as. A removal that fails — a dirty worktree, a slice that
  never had one, a machine with no worktrunk — is one line in the log and
  nothing else: the pull request is open and the slice is Done whatever became
  of the checkout, and worktrunk's own rules mean a refusal never costs any
  work. gh stays in the shared checkout, so the removal cannot strand it, and
  `R` deliberately keeps its worktree — the work so far is exactly what the
  next session wants.
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
  row of the same board) and draws such a row as one there is nothing to do
  about yet — a `⊘` in the row's marker cell, the one the agent star takes
  (`Board.marker`, and the star wins the cell in the case the two are somehow
  both true), the row's own text in the muted Blocked colour, and the row itself
  sunk to the bottom of its milestone (`appendGroup`, keeping the plan's order
  among the sunk ones). The sinking is the drawn rows and nothing else: the view
  order in Notion is untouched, so `next-slice` and `PlanOrder` hand work out
  exactly as they did. What the wait is on is the status band's, not the row's:
  while a blocked row is selected, the board's own screen reads `blocked by
  <milestone>: <slice>` for each blocker (`Board.BlockedBy` and
  `App.blockedIndicator`), first of the standing indicators, since it is the
  only one about the row the user is on. `l` on such a row is refused with a
  status-bar toast naming what it waits on and how far off each is. A toast
  rather than an error banner: nothing has gone wrong, and the slice is still
  there to launch once its dependencies land.
- Everything the board knows about a slice in flight adds up to one state
  (`domain.StateOf`, `internal/domain/state.go`): working, waiting, blocked,
  ready to push, awaiting review, ready to merge — or none at all for a slice
  that is not in flight, which is a Todo one and a Done one whose work has
  landed. A Done slice is tested first and against its pull request alone,
  because Done is Notion's word for the slice rather than for the work: `p`
  marks it Done as it opens the pull request, and until that merges the work is
  not on main and the review is not over, so such a slice is still in whatever
  state its pull request is in. It takes a positive reading to say so — with
  nothing read, a Done slice is in no state at all, which is what every Done
  slice a project ever finished must go on being. After that, the
  order the facts are tested in is the order they are true in: a live agent
  wins over everything on the page, because it is the only reading taken fresh
  — an agent running on a handed-back branch is the review going back to it —
  then work that is out (a `Branch`, a `PR`) is what there is to do something
  about, then the wait on a dependency, and what is left is a slice in progress
  that nothing is happening on and nothing has come out of. The agent is passed
  as `domain.AgentPresence`, domain's own saying of the board's two readings —
  the live map and the activity watcher — as one value, since `internal/agent`
  and `internal/tui` both import this package and neither's enum could be
  reached from here. What tells the two endings of work that is out apart is
  `domain.PRReadiness`, the same trick for the GitHub CLI: a pull request read
  as open and as approved and mergeable is ready to merge, and everything else
  about an open one — unreviewed, changes asked for, unmergeable — is the review
  still to come, which is what a branch handed back with no pull request on it is
  too. Both affirmative values mean a pull request positively read as open, since
  the reading is a listing of what a repository has open; the zero value is
  therefore three things at once — no pull request, none read of, and one no
  longer open — and no rule here wants them told apart. It is a board nobody has
  asked gh anything on, so the whole refinement is absent rather than wrong where
  there is no gh to ask, and it is the one thing keeping a Done slice in flight,
  so a Done slice with no reading behind it is out of the section rather than
  every Done slice in the project's history flooding it — see
  `internal/tui/prstate.go`. The Active section is what draws it — see
  `internal/tui/active.go`.
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
- Gate before claiming done: `go vet ./... && go test -race
  -coverprofile=coverage.out ./... && ./scripts/no-uncovered.sh &&
  golangci-lint run`. The profile rather than `-cover`, because the percentage
  is printed to one decimal and a single unrun statement in a package of
  thousands reads as 100.0%; `scripts/no-uncovered.sh` reads the profile itself,
  which does not round, and prints the blocks nothing ran. One `go test ./...`
  writes it, since the blocks are merged across packages — a helper covered only
  by another package's tests is still covered. All four run in CI, so a failing
  lint or an unrun statement fails the PR;
  `brew install golangci-lint` if the binary is missing. `.golangci.yml` runs
  the default linter set with one exclusion — see the file — so an unchecked
  error is either handled or assigned to `_` where the reason can be read.
- Never log or commit the Notion token; it belongs to the `ntn` CLI and is only
  ever held in memory for the lifetime of a request.
- Before starting work, pull the latest `main` and branch off it. Only ever
  base branches — and PRs — on `main`, never on another slice branch, so every
  PR merges into `main`.
