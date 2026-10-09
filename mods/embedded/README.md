# nat-embedded

nat's own Claude Code mod. nat loads it into every agent session it launches
with `claude --plugin-dir`, for that one session only; it is never installed
and never reaches a session nat did not start. Nothing here is meant to be
loaded by hand.

Tested with Claude Code **2.1.294**.

- `hooks/register.ts` — the hooks, which quiet the pane's chrome by
  rewriting Claude Code's own render-site props: the prompt hint
  (`? for shortcuts`, which is also how a load shows in the pane) and the
  run-in-background pill draw empty, the turn duration line and the notices
  under the logo draw nothing, and the spinner says `Working`. The session
  modes are left alone. They also set the pane's waiting flag
  (`nat agent-waiting` / `nat agent-working`) while an AskUserQuestion
  dialog, a permission prompt or an MCP elicitation waits on the user, or a
  turn ended on an error or a refusal; a plain finished turn marks nothing.
  And they deliver what nat sends the session: from `session.start`, once a
  second, each file in the inbox `NAT_INBOX` names, in name order, is
  removed and then submitted with `$.prompt.submit({ text, asUser: true })`
  — a turn of its own once idle, no composer involved.
  And they record a resume: a prompt the user typed (`origin.kind`
  `composer` or `bridge`) on a session with `NAT_SLICE` and `NAT_PROJECT`
  set runs `nat slice-resume` with the prompt on stdin before it goes on.
  And they hand the session its brief: `prompt.context` appends the file
  `NAT_BRIEF` names as a `natBrief` context block, which the model reads
  and the pane never draws; nat starts the session on one opening line
  pointing at it. Checked live on 2.1.294: hidden in the pane and under
  ctrl+o, re-read from the file on `/compact`, kept as recorded (not
  re-read) on `claude --resume`. A session whose mod never loads (an older
  Claude Code) has the opening line alone and asks what to work.
- `types/index.d.ts` — the `$.state` contract: the wait last written, so a
  hot reload neither forgets it nor writes the flag again.
- `tests/` — `claude plugin test` suites.

Check it with `./scripts/mod-check.sh` from the repository root
(`claude plugin validate --strict` and `claude plugin test`; `claude` on
PATH, no sign-in or network). Write against the public mods reference
(`https://code.claude.com/docs/en/plugins/mods/reference`) and the
declarations the installed build lays beside a loaded mod
(`.claude-plugin/types/claude-code/index.d.ts`, gitignored with the
`tsconfig.json` beside it), never one build's quirks.

The contract — how nat embeds, writes and loads this folder — is
`docs/design/embedded-mod/README.md`.
