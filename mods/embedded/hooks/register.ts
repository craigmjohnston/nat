import { atom, update } from 'claude-code'
import type { EngineInterface, Register, SessionRateLimit, StateDollar } from 'claude-code'

import type { Wait } from '../types'

type Engine = StateDollar & Pick<EngineInterface, 'process' | 'ui'>
type InboxEngine = Pick<EngineInterface, 'clock' | 'fs' | 'process' | 'prompt' | 'ui'>
type ResumeEngine = Pick<EngineInterface, 'env' | 'process' | 'ui'>

// What the mod last wrote on the pane, held by the host so a hot reload (a
// fresh module) neither forgets a wait it marked nor writes the flag again.
const wait = atom({ plugin: 'nat-embedded', key: 'wait' } as const, null)

// Sets the pane's waiting flag to `to` — what the agent's own `nat
// agent-waiting` / `nat agent-working` set — only where it changes; a wait
// for one thing becoming a wait for another rewrites nothing. `from` limits a
// clear to the waits it names. The session carries nat's PATH and the pane's
// TMUX_PANE, which is how the command finds the pane. Nothing here throws: a
// failure goes to the debug log, never the transcript, and never costs the
// hook calling it — the flag is a hint, not the work.
async function mark($: Engine, to: Wait | null, from?: readonly Wait[]): Promise<void> {
  try {
    let was: Wait | null = null
    const now = await update($, wait, value => {
      was = value
      return to !== null || !from || (value !== null && from.includes(value)) ? to : value
    })
    if ((was === null) === (now === null)) return
    const command = now === null ? 'agent-working' : 'agent-waiting'
    const { exitCode, stderr } = await $.process.run(['nat', command], { timeoutMs: 10_000 })
    if (exitCode !== 0) $.ui.log(`nat ${command} exited ${exitCode}: ${stderr.trim()}`, { to: 'debug' })
  } catch (err) {
    $.ui.log(`waiting flag (${to ?? 'working'}) not written: ${String(err)}`, { to: 'debug' })
  }
}

// One prompt nat sent, as `nat agent-send` (and every other sender) names it:
// the send's Unix time in nanoseconds, so name order is send order. A temp
// file still being written has another name and is never read.
const inboxFile = /^\d+\.md$/

// Delivers what nat left in this session's inbox (`NAT_INBOX`, set by nat on
// the launch) as the user's own prompts, each a turn of its own once the
// session is idle — no composer, so no dialog, permission prompt or draft in
// the pane can take it. Each file is removed before it is submitted, and only
// a removal that worked submits: a file nat took back after giving up on the
// mod (and pasted instead) is never sent twice. `busy` keeps a tick from
// starting while the last is still at it; a reload starts both afresh.
// Failures go to the debug log; the next tick tries again.
function pollInbox($: InboxEngine, dir: string): void {
  let busy = false
  $.clock.every(1000, async () => {
    if (busy) return
    busy = true
    try {
      if (!(await $.fs.exists(dir))) return
      const names = (await $.fs.list(dir))
        .filter(entry => entry.kind === 'file' && inboxFile.test(entry.name))
        .map(entry => entry.name)
        .sort()
      for (const name of names) {
        const path = `${dir}/${name}`
        const text = await $.fs.read(path)
        const { exitCode } = await $.process.run(['rm', path], { timeoutMs: 10_000 })
        if (exitCode !== 0) continue
        // Resolves once the turn starts: not awaited, so the next file is not
        // held behind a running turn.
        void $.prompt.submit({ text, asUser: true })
      }
    } catch (err) {
      $.ui.log(`agent inbox not read: ${String(err)}`, { to: 'debug' })
    } finally {
      busy = false
    }
  })
}

// The origins of a prompt the user wrote themselves: Enter in the pane (gnat's
// typing reaches it so) or a message through Remote Control. Every other
// origin — nat's own sends through the inbox (a plugin's), a background
// task's notification, a schedule, a peer — is not the user asking for more.
const typed: ReadonlySet<string> = new Set(['composer', 'bridge'])

// Puts a prompt the user typed at a slice's agent on the record as
// `nat slice-resume`, the prompt its note, before the agent reads it — what
// every nat send does itself before it sends. `slice-resume` writes nothing
// where the slice is not handed back, so every typed prompt runs it; where it
// is, the slice's board card reads as work in progress again. `NAT_SLICE` and
// `NAT_PROJECT` are set by nat on a slice's launch alone. Nothing here throws:
// a failure goes to the debug log, never the transcript, and never holds the
// prompt.
async function resume($: ResumeEngine, text: string): Promise<void> {
  try {
    const slice = await $.env.get('NAT_SLICE')
    const project = await $.env.get('NAT_PROJECT')
    if (!slice || !project) return
    const { exitCode, stderr } = await $.process.run(
      ['nat', 'slice-resume', slice, '--project', project, '--note', '-'],
      { stdin: text, timeoutMs: 30_000 },
    )
    if (exitCode !== 0) $.ui.log(`nat slice-resume exited ${exitCode}: ${stderr.trim()}`, { to: 'debug' })
  } catch (err) {
    $.ui.log(`resume not recorded: ${String(err)}`, { to: 'debug' })
  }
}

// The rate-limit windows `nat usage` reads, by the name the statusline payload
// gives them; any other kind (a gateway's spend limit) is not one it shows.
const windows: ReadonlySet<string> = new Set(['five_hour', 'seven_day'])

type UsageEngine = Pick<EngineInterface, 'clock' | 'env' | 'fs' | 'process' | 'ui'>

// Writes the account's rate-limit windows, as this session last measured them,
// to the file nat named at launch (`NAT_USAGE`), for `nat usage` to answer
// from while any agent is live instead of starting a session of its own. Each
// window is written only where the measurement carries it, and a measurement
// carrying none (off a subscription, or before the first reading) writes
// nothing. `$.fs.write` is not atomic, so the file goes through a temp name
// and an `mv`, as the statusline tee does: nat never reads half of one.
// Nothing here throws: a failure goes to the debug log.
async function writeUsage($: UsageEngine, rateLimits: readonly SessionRateLimit[]): Promise<void> {
  try {
    const path = await $.env.get('NAT_USAGE')
    if (!path) return
    const limits: Record<string, { used_percentage: number; resets_at?: string }> = {}
    for (const limit of rateLimits) {
      if (!windows.has(limit.kind)) continue
      limits[limit.kind] = { used_percentage: limit.percentUsed, ...(limit.resetsAt ? { resets_at: limit.resetsAt } : {}) }
    }
    if (Object.keys(limits).length === 0) return
    const readAt = new Date(await $.clock.now()).toISOString()
    const tmp = `${path}.tmp`
    await $.fs.write(tmp, JSON.stringify({ read_at: readAt, rate_limits: limits }))
    const { exitCode, stderr } = await $.process.run(['mv', tmp, path], { timeoutMs: 10_000 })
    if (exitCode !== 0) $.ui.log(`usage file not moved into place: mv exited ${exitCode}: ${stderr.trim()}`, { to: 'debug' })
  } catch (err) {
    $.ui.log(`usage file not written: ${String(err)}`, { to: 'debug' })
  }
}

// nat's hooks into the Claude Code sessions it launches. Written against the
// public mods reference and the declarations the installed build writes beside
// a loaded mod; a hook that fails is skipped and a tree that does not validate
// is replaced by Claude Code's own drawing, so a drift costs a feature, never
// a session.
//
// An agent's pane in gnat is a viewport onto the agent, not a terminal the
// user is learning key by key, so the chrome that teaches or decorates is
// quieted. Each hook rewrites the engine's own props rather than drawing a
// tree of its own, so Claude Code keeps drawing everything it knows; the
// mode labels (`SessionMode`) are information and are left alone.
export const register: Register = on => {
  // Prompts nat sends this session arrive through its inbox; a session nat
  // launched with none (an older tmux) is sent them by a paste instead.
  on('session.start', async ($, e, next) => {
    const dir = await $.env.get('NAT_INBOX')
    if (dir) pollInbox($, dir)
    return next(e)
  })

  // The session's brief, which nat leaves in a file (`NAT_BRIEF`) rather than
  // in argv, rides the first user message as a context block beside
  // CLAUDE.md's: the model reads it, the pane never draws it, and a
  // compaction or `/clear` re-reads it here. The pane shows only nat's one
  // opening line. A brief that cannot be read is logged and left out — the
  // agent then has the opening line alone and asks, a visible failure.
  on('prompt.context', async ($, e, next) => {
    const path = await $.env.get('NAT_BRIEF')
    if (!path) return next(e)
    let text: string
    try {
      text = await $.fs.read(path)
    } catch (err) {
      $.ui.log(`agent brief not read: ${String(err)}`, { to: 'debug' })
      return next(e)
    }
    return next({ ...e, blocks: [...e.blocks, { name: 'natBrief', text }] })
  })

  // After each turn, and whenever a window moves a whole point, the account's
  // rate limits go where `nat usage` reads them.
  on('session.measure', async ($, e, next) => {
    await writeUsage($, e.rateLimits)
    return next(e)
  })

  // The dim `? for shortcuts` / `esc to interrupt` line under the prompt
  // draws empty. It is also the one visible mark that a session loaded this
  // mod.
  on('ui.render', { component: 'PromptHint' }, ($, e, next) =>
    next({ ...e, props: { ...e.props, hint: '' } }),
  )

  // The `Baked for 3s` line closing each turn draws nothing.
  on('ui.render', { component: 'TurnDuration' }, ($, e) => {
    const { Box } = $.ui.resolve(e)
    return h(Box, {})
  })

  // The dim notices under the logo (model source, experiment enrolment,
  // settings hint) draw nothing.
  on('ui.render', { component: 'InfoNotice' }, ($, e) => {
    const { Box } = $.ui.resolve(e)
    return h(Box, {})
  })

  // The `(ctrl+b to run in background)` pill under a tool call draws
  // nothing: the user does not drive the agent's pane by key.
  on('ui.render', { component: 'ToolProgress', props: { kind: 'background_hint' } }, ($, e, next) =>
    next({ ...e, props: { ...e.props, hint: '' } }),
  )

  // The spinner says `Working` in place of the sampled flavour word; the
  // message, suffix and mode stay the engine's, and so do the elapsed time
  // and token count drawn after them.
  on('ui.render', { component: 'Spinner' }, ($, e, next) =>
    next({ ...e, props: { ...e.props, word: 'Working' } }),
  )

  // The waiting flag gnat's star, dock badge and Active rail read, set and
  // cleared at the moments the engine itself knows are a wait on the user.
  // The agent's own `nat agent-waiting` stays for what the engine cannot
  // see — a question asked in prose at the end of a turn — so a plain
  // finished turn (`answer`) and an `idle_prompt` notification mark nothing:
  // that is a hand-back or a planning agent between prompts.
  on('turn.start', async ($, e, next) => {
    await mark($, null)
    return next(e)
  })

  on('prompt.submit', async ($, e, next) => {
    await mark($, null)
    if (typed.has(e.origin.kind)) await resume($, e.text)
    return next(e)
  })

  // An AskUserQuestion dialog waits from before it is drawn until it is
  // answered, however it ends.
  on('tool.call', { tool: 'AskUserQuestion' }, async ($, e, next) => {
    await mark($, 'ask')
    try {
      return await next(e)
    } finally {
      await mark($, null, ['ask'])
    }
  })

  // A permission prompt is over once a tool call settles or the model is
  // asked again.
  on('tool.call', async ($, e, next) => {
    try {
      return await next(e)
    } finally {
      await mark($, null, ['permission'])
    }
  })

  on('turn.step', async function* ($, e, next) {
    await mark($, null, ['permission'])
    return yield* next(e)
  })

  // A permission dialog is shown only where nothing beneath decided it.
  on('classic.PermissionRequest', async ($, e, next) => {
    const decided = await next(e)
    if (!decided.decision) await mark($, 'permission')
    return decided
  })

  on('classic.Notification', async ($, e, next) => {
    if (e.notification_type === 'permission_prompt') await mark($, 'permission')
    return next(e)
  })

  on('classic.Elicitation', async ($, e, next) => {
    await mark($, 'elicitation')
    return next(e)
  })

  on('classic.ElicitationResult', async ($, e, next) => {
    await mark($, null, ['elicitation'])
    return next(e)
  })

  // A main-loop turn that died on an API error or a refusal leaves the agent
  // stuck until the user steps in; a subagent's is its spawner's to handle.
  on('turn.complete', async ($, e, next) => {
    if (!e.agentId && (e.reason === 'error' || e.reason === 'refusal')) await mark($, 'stuck')
    return next(e)
  })
}
