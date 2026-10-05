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
- `OpenPRs(dir)` — one `gh pr list --state open --json
  url,reviewDecision,mergeable,statusCheckRollup --limit 100` per repo, keyed
  by URL. Only `APPROVED` and `MERGEABLE` count as true; every other GitHub
  word (`REVIEW_REQUIRED`, `CHANGES_REQUESTED`, `CONFLICTING`, `UNKNOWN`, a
  no-review-required repo's empty decision) is "not true." The rollup becomes
  one `ChecksVerdict` (`checksVerdictOf`, over `Check.Outcome` — the one
  table of check words, which the TUI PR screen and `actions.MergeRefusal`
  read too): any failure → failing, else any unfinished or unknown state →
  pending, else passing; no checks → `ChecksNone` (no verdict). The board folds failing into `domain.PRChecksFailing`.
  **Limit 100 is past gh's default of 30** — a repo with more open PRs than
  that has its oldest silently missing, which reads exactly like a PR that
  closed. Not listed = not open; a failed listing is logged
  and returned as an error, never treated as "nothing open."
- `ViewPR(dir, ref)` — `gh pr view <ref> --json ...` for the full-detail
  screen. GitHub's own vocabulary is kept as-is (`State`, `ReviewDecision`,
  `Mergeable`, `MergeStateStatus`) rather than translated, except a check —
  `CheckRun` vs `StatusContext` differ only in field names, not meaning, so
  both decode into one shape: a name, a state/conclusion, a link.
  `checksOf` names and orders every check, for every face: a run reads
  `<workflowName> / <job>` (`jobName`, the last ` / ` segment — `Pull
  request / Gate`), and only runs that still clash take their full path
  under the workflow; a StatusContext or a run with no workflow stays bare.
  Never the triggering event — gh's rollup doesn't carry `(pull_request)`.
  The list is sorted by that final name, case-insensitively and stably,
  which is GitHub's own checks-list order (gh returns creation order).
- `MergePR(dir, ref)` — `gh pr merge <ref> --merge`. The strategy flag is
  mandatory: a `Runner` subprocess has nothing on stdin, so without it gh
  prompts for a strategy and the merge hangs.
- `EditReviewers(dir, ref, add, remove)` — `gh pr edit <ref> --add-reviewer
  a,b --remove-reviewer c`; an edit naming nobody is refused before gh runs
  (gh would prompt). `Collaborators(dir)` — `gh api
  repos/{owner}/{repo}/collaborators --paginate --jq .[].login` (needs push
  access; a refusal is an error, never an empty list). `ViewPR` also reads
  `reviewRequests` (a user's login, a team's slug).
- `FailedLog(dir, ref)` — a failed check's log: `gh run view [--job]
  --log-failed`, falling back to `gh api repos/<o>/<r>/actions/jobs/<job>/logs`
  where gh refuses because a sibling is still running.
- `ActionsJob(dir, ref)` — `gh api repos/<o>/<r>/actions/jobs/<job>`: a job's
  status, `created_at`/`started_at`, runner and steps, answered while it runs.
  `JobLog(dir, ref)` — the same path `/logs`; for a job still running gh exits
  `HTTP 404` with a `BlobNotFound` body on stdout, which is `ErrLogNotReady`.
  Both refuse a ref short of owner, repo or job before gh runs. Observations
  are in the doc comments (October 2026, gh 2.83.1).
- `RunStatus`, `CancelRun`, `RerunRun(failedOnly)`, `RerunJob` — `gh run view
  <run> --json status`, `gh run cancel <run>`, `gh run rerun <run> [--failed]`,
  `gh run rerun --job <job>`, each `--repo <o>/<r>` where the ref knows it (a
  check's run may be in another repository than the worktree's). GitHub
  cancels whole runs only, and refuses a re-run of a run still going (403;
  gh: "run <id> cannot be rerun; …", per gh's source). These log method, ids
  and exit code only (`logRunCall`), never gh's words.
- `NormaliseURL(url)` — strips query/fragment, trailing slash, lowercases
  owner/repo — so a URL pasted from a review comment matches the canonical
  one gh prints.
