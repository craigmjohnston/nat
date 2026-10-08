# internal/gh

A thin wrapper on the `gh` binary. gh already knows the repo, the remote and
the auth — none of that is reimplemented here. Agents never open pull
requests themselves; opening one is the review screen's `a` key, after a
human has read the diff.

## Conventions

- Every call takes a working directory (`dir`) — the slice's repo — because
  `Runner` is a subprocess, not `agent.Runner`; this package has no business
  borrowing tmux's type. `ReadPRs` alone runs in none (`""`): its document
  names every repository itself.
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

- `CreatePR(dir, branch, base, title, body)` — `gh pr create --head <branch>
  --title <title> --body <body>`, or `--fill` when `title` is empty (a
  hand-back written before `--pr-description` existed). `--base <base>` only
  where the project configures a `base_branch` (`OpenPR`'s `base`); otherwise
  gh's own default-branch answer is correct. Returns the **last** `https://` line gh
  printed (`prURL`), not the first — gh sometimes prepends a line about the
  branch it pushed.
- `CommentPR(dir, ref, body)` — `gh pr comment <ref> --body-file -`, body on
  gh's own stdin via `StdinRunner`, never `--body` as an argument: a review
  comment quoting diff lines has no length bound and a shell's argument list
  does.
- `EditPRBody(dir, ref, body)` — `gh pr edit <ref> --body-file -`, the
  body on stdin as `CommentPR`'s is, for `pr-edit`.
- `ReadPRs(BatchQuery)` — **the** polling read: one `gh api graphql -f
  query=<document>` for every pull request (by number), every branch (by
  name — an ad hoc session's) and at most one pull request in full detail
  (`--detail`, the PR tab on screen) a reading asks about, across every
  repository and every project. See **The batched reading** below.
- `StatusOf(PR)` — a reading's `PRStatus`: only `APPROVED` and `MERGEABLE`
  count as true; every other GitHub word (`REVIEW_REQUIRED`,
  `CHANGES_REQUESTED`, `CONFLICTING`, `UNKNOWN`, a no-review-required repo's
  empty decision) is "not true." The checks become one `ChecksVerdict`
  (`Verdict`, over `Check.Outcome` — the one table of check words, which the
  TUI PR screen and `actions.MergeRefusal` read too): any failure → failing,
  else any unfinished or unknown state → pending, else passing; no checks →
  `ChecksNone` (no verdict). The board folds failing into
  `domain.PRChecksFailing`. `State` (OPEN/MERGED/CLOSED) and `MergedAt` ride
  along: what settles a merge nat did not witness, with no view of its own.
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
- `MergePR(dir, ref, MergeOptions)` — `gh pr merge <ref> --<method>`, the
  project's `merge_method` (`merge` where unset; a word outside merge/squash/
  rebase refused before gh runs), plus `--delete-branch` only where
  `delete_branch` is set. The strategy flag is mandatory: a `Runner`
  subprocess has nothing on stdin, so without it gh prompts for a strategy and
  the merge hangs.
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
  one gh prints. `ParsePRURL` reads owner, repository and number off that
  shape (anything else names no pull request, and is left unread);
  `ParseRemote` a repository off a git remote URL (https, ssh, scp-like).

## The batched reading (`batch.go`)

GitHub's GraphQL budget is 5,000 points an hour, shared with every agent's
`gh` and Claude Code's own, and spent on **call volume**: every read nat
makes costs 1 or 2 points, measured with GraphQL's `rateLimit { cost }`
(October 2026) — `gh pr list` with the check rollup **2**, `gh pr view`
**1**, `gh pr list --head` **1**, and one document naming ten pull requests
by number across two repositories, each with review decision,
mergeability, base and a 30-context rollup, **1** — and the full PR-tab
detail added to such a document, still **1**. GitHub's published page-size
formula overstates these about a hundredfold; don't reason from it. Polling
was ~2,200 reads an hour from gnat with four tabs open; one document per
tick is 120.

- **The document**: `rateLimit { limit remaining resetAt cost }`, then an aliased
  `rN: repository(owner:, name:)` per repository (in the order first named),
  holding an aliased field per thing asked — `pN: pullRequest(number:) {
  ...status }`, `hN: pullRequests(first: 10, headRefName:, orderBy: CREATED_AT
  DESC) { nodes { number title url state mergedAt } }`, `d: pullRequest(number:)
  { ...status ...detail }`. `status` is `number url state isDraft mergedAt
  reviewDecision mergeable mergeStateStatus baseRefName` and `lastCommit:
  commits(last: 1)` → `statusCheckRollup { state contexts(first: 30) }`
  (CheckRun: name, status, conclusion, detailsUrl,
  checkSuite.workflowRun.workflow.name; StatusContext: context, state,
  targetUrl); `detail` is the rest of what `pr-view` prints — title, body,
  author, head ref and oid, additions, deletions, changed files,
  `allCommits: commits { totalCount }` (a count, never the commits),
  `reviews(last: 100)`, `comments(last: 100)` (the newest are the ones a
  reader waits on) and `reviewRequests(first: 100)` (User login, Team slug).
  **A fragment goes in only where it is spread** — GraphQL refuses an unused
  one. Strings are JSON-quoted (`gqlString`), which GraphQL accepts.
- Decoded through `gqlPR.pr()` into the same `prView` → `PR` path `ViewPR`
  takes, so check naming (`checksOf`), review requests and the rest are
  undone in one place. `Batch.PRs` by `PRRef`, `Batch.Heads` by `HeadRef`,
  `Batch.Detail`, `Batch.RateLimit` (the last document's).
- **Chunked at 25** things per document (`batchChunk`), the detail first.
- **Failure concludes nothing, per node where it can be**: gh exits non-zero
  on an answer with `errors` and still prints it, so the answer is read
  first. An error with no `path` (a budget refusal, a document GitHub would
  not run), an answer that is no JSON, or one with no `data` fails the whole
  document — everything it asked is absent, logged, and the error returned
  (joined across documents). An error **with** a path (a renamed repository,
  a number that is no pull request) leaves that node null — that node alone
  unread, logged — so one dead link on one slice never blinds the reading of
  every other. Absent always means unread: never "closed", never "merged".
- **The budget** (`budget.go`, `Budget`): `gh-budget.json` in nat's state
  directory (beside `github-reading.json`), shared by every nat process —
  the last two readings (`limit`, `remaining`, `resetAt`, read time) and a
  refusal's stop. `New()` keeps it (`WithBudget(DefaultBudget())`);
  `NewWithRunner` keeps none, so a test's fake runner never touches it.
  `WithBudget` wraps the runner (`budgetRunner`): a call whose stderr says
  `API rate limit already exceeded` / `API rate limit exceeded` /
  `exceeded a secondary rate limit` (or a document's `RATE_LIMITED` error,
  `documentRefusal`) records a stop until the last reading's reset, else five
  minutes on, and fails as `*LimitError` ("GitHub's API limit is spent until
  13:46; try again then"); any call that succeeds clears the stop. `ReadPRs`
  records each reading's rate limit and sums `rateLimit { cost }` into
  `Batch.Cost`. `PollPRs` is the polling read: before the stop's retry time it
  runs no gh and returns an empty batch, logged once per stop (`Logged` in
  the file). `Outlook(poll)` is the policy, all in `outlookOf`: projection
  `remaining − rate × (reset − now)` (rate off the last two readings of the
  same hour), reserve a fifth of `limit`; poll at `poll` while the projection
  holds the reserve, else `(reset − now) / (remaining − reserve)` capped at
  five minutes and floored at `poll` (the cap where already under the
  reserve); `Throttled` while that is longer than `poll`; a live stop is
  `PausedUntil` and its wait. No `gh api rate_limit` read — its `graphql`
  block reported another window than the one GraphQL enforces.
- What is worth asking is the caller's: `actions.PRsWorthAsking` (every In
  progress slice with a PR; a Done one only while its worktree exists) and a
  session's five most recent branches (`internal/cli`'s `sessionBranches`).
