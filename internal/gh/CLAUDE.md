# internal/gh

A thin wrapper on the `gh` binary. gh already knows the repo, the remote and
the auth — none of that is reimplemented here. Agents never open pull
requests themselves; opening one is the review screen's `a` key, after a
human has read the diff.

## Conventions

- Every call takes a working directory (`dir`) — the slice's repo — because
  `Runner` is a subprocess, not `agent.Runner`; this package has no business
  borrowing tmux's type.
- A `gh` that ran and refused comes back as `*ExitError`, whose `Error()` is
  the **first non-empty line of gh's stderr** (`firstLine`) — gh follows its
  message with usage text, and the message is the one line worth a toast.
  `"a pull request for branch X already exists"` is the model case.
- A ref (branch, PR number, or the URL from the slice's `PR` property) is
  refused **before gh runs at all** when empty — `ViewPR`, `MergePR`,
  `CommentPR` all do this. gh given nothing named reads/writes against
  whatever branch the directory happens to be checked out on, which in a
  shared checkout is nobody's slice in particular.
- `Runner`/`StdinRunner` are the test seams. `StdinRunner` is kept separate
  from `Runner` (not folded in) so every existing fake keeps satisfying
  `Runner` unchanged; only `CommentPR`'s fake needs the stdin method too.

## Calls

- `CreatePR(dir, branch, title, body)` — `gh pr create --head <branch>
  --title <title> --body <body>`, or `--fill` when `title` is empty (a
  hand-back written before `--pr-description` existed). No `--base`: gh's own
  default-branch answer is correct. Returns the **last** `https://` line gh
  printed (`prURL`), not the first — gh sometimes prepends a line about the
  branch it pushed.
- `CommentPR(dir, ref, body)` — `gh pr comment <ref> --body-file -`, body on
  gh's own stdin via `StdinRunner`, never `--body` as an argument: a review
  comment quoting diff lines has no length bound and a shell's argument list
  does.
- `OpenPRs(dir)` — one `gh pr list --state open --json url,reviewDecision,mergeable
  --limit 100` per repo, keyed by URL. Only `APPROVED` and `MERGEABLE` count
  as true; every other GitHub word (`REVIEW_REQUIRED`, `CHANGES_REQUESTED`,
  `CONFLICTING`, `UNKNOWN`, a no-review-required repo's empty decision) is
  "not true." **Limit 100 is past gh's default of 30** — a repo with more
  open PRs than that has its oldest silently missing, which reads exactly
  like a PR that closed. Not listed = not open; a failed listing is logged
  and returned as an error, never treated as "nothing open."
- `ViewPR(dir, ref)` — `gh pr view <ref> --json ...` for the full-detail
  screen. GitHub's own vocabulary is kept as-is (`State`, `ReviewDecision`,
  `Mergeable`, `MergeStateStatus`) rather than translated, except a check —
  `CheckRun` vs `StatusContext` differ only in field names, not meaning, so
  both decode into one shape: a name, a state/conclusion, a link.
- `MergePR(dir, ref)` — `gh pr merge <ref> --merge`. The strategy flag is
  mandatory: a `Runner` subprocess has nothing on stdin, so without it gh
  prompts for a strategy and the merge hangs.
- `NormaliseURL(url)` — strips query/fragment, trailing slash, lowercases
  owner/repo — so a URL pasted from a review comment matches the canonical
  one gh prints.
