# Agent prompt template (manual launches, until the TUI automates this)

Until the TUI's launch flow exists (M6), start agents by hand:

```sh
pane=$(tmux new-session -d -s nat-<last-8-of-slice-id> -c <working-dir> -P -F '#{pane_id}' \
  claude "$(cat /path/to/prompt.txt)")
tmux set-option -p -t "$pane" @nat_slice <full-slice-id>   # what the TUI finds it by
tmux attach-session -t nat-<last-8-of-slice-id>   # optional
```

The session name is only a label — the last 8 digits because slice IDs from
one workspace share a leading prefix. The `@nat_slice` pane option is the
identity.

Prompt template — fill in the `<...>` parts:

```
You are working on one slice of the "<project name>" project, tracked in
Notion. Your slice: <slice title>
Slice page: <slice notion url>

Steps:
1. Claim the slice via the Notion MCP BEFORE doing any work: set its
   Assignee to <assignee name> and Status to "Claimed".
2. Read the slice page body for the full brief, and the parent project page
   for project conventions. Read the repo's CLAUDE.md.
3. Do the work in <working dir>.
   <"Note: this slice overrides the project's default working directory." if applicable>
4. If the work is code: keep it to exactly ONE pull request for this slice.
   Create a branch, commit, push, open the PR, and record its URL in the
   slice's "PR" property. Do not merge it.
   If the work is not code (docs, research): produce the deliverable the
   brief asks for.
5. When complete: set the slice Status to "Done" and append a short summary
   of what you did (and any follow-ups) to the slice page.
6. If you cannot complete the slice: leave Status as "Claimed" and append a
   note to the slice page explaining what's blocking.

Never modify other slices or milestones.
```
