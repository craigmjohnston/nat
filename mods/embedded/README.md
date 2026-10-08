# nat-embedded

nat's own Claude Code mod. nat loads it into every agent session it launches
with `claude --plugin-dir`, for that one session only; it is never installed
and never reaches a session nat did not start. Nothing here is meant to be
loaded by hand.

Tested with Claude Code **2.1.294**.

- `hooks/register.ts` — the hooks. Today one: the dim prompt hint under the
  input (`? for shortcuts`) draws empty, which is also how a load is seen
  from the pane.
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
