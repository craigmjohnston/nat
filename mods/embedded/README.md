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
