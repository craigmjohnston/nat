import type { On } from 'claude-code'
import { expect, test } from 'claude-code/testing'

// Each test's own hooks sit beneath the plugin, standing for the engine: its
// `process.run` records each argv the plugin ran and answers as nat would, so
// a test reads the waiting flag as the list of commands that wrote it; the
// events the plugin passes on are answered as plainly as the engine can.

const ok = { exitCode: 0, stdout: '', stderr: '', isStdoutTruncated: false, isStderrTruncated: false }

// `own` names the events the test answers itself.
function engine(on: On, own: readonly string[] = []): void {
  const answer = (event: string, hook: () => void) => own.includes(event) || hook()
  answer('turn.start', () => on('turn.start', ($, e) => ({ turnId: e.turnId })))
  answer('turn.step', () =>
    on('turn.step', async function* ($, e) {
      return { turnId: e.turnId, index: e.index, answer: '', toolUses: [] } as never
    }),
  )
  answer('turn.complete', () => on('turn.complete', () => ({ text: '' })))
  answer('prompt.submit', () => on('prompt.submit', ($, e) => ({ text: e.text })))
  answer('classic.PermissionRequest', () => on('classic.PermissionRequest', () => ({})))
  answer('classic.Notification', () => on('classic.Notification', () => ({})))
  answer('classic.Elicitation', () => on('classic.Elicitation', () => ({})))
  answer('classic.ElicitationResult', () => on('classic.ElicitationResult', () => ({})))
  answer('ui.log', () => on('ui.log', () => ({ value: undefined })))
}

function runs(on: On, own: readonly string[] = []): string[][] {
  const ran: string[][] = []
  on('process.run', ($, e) => {
    ran.push([...e.argv])
    return { value: ok }
  })
  engine(on, own)
  return ran
}

const waiting = ['nat', 'agent-waiting']
const working = ['nat', 'agent-working']

const ask = {
  tool: 'AskUserQuestion',
  tool_use_id: 'toolu_ask',
  questions: [{ question: 'Which?', header: 'Pick', multiSelect: false, options: [{ label: 'A', description: 'a' }, { label: 'B', description: 'b' }] }],
} as never

const finished = (reason: 'answer' | 'aborted' | 'error', agentId?: string) =>
  ({ answer: '', durationMs: 1, isAborted: reason === 'aborted', turnId: 't1', reason, ...(agentId ? { agentId } : {}) }) as never

test('an AskUserQuestion answered in the engine marks waiting, then working once answered', async ($, on) => {
  const ran = runs(on)
  let seen: string[][] = []
  on('tool.call', { tool: 'AskUserQuestion' }, () => {
    seen = ran.map(argv => argv)
    return { result: { questions: [], answers: { 'Which?': 'A' } } } as never
  })
  await $.tool.call(ask)
  expect(seen).toEqual([waiting])
  expect(ran).toEqual([waiting, working])
})

// As Claude Code 2.1.294 raises it live: the dialog is a permission request,
// the tool call runs once it is answered.
test('an AskUserQuestion dialog raised as a permission request writes once each way', async ($, on) => {
  const ran = runs(on)
  on('tool.call', { tool: 'AskUserQuestion' }, () => ({ result: { questions: [], answers: { 'Which?': 'A' } } }) as never)
  await $.classic.PermissionRequest({ tool_name: 'AskUserQuestion', tool_input: {} })
  expect(ran).toEqual([waiting])
  await $.tool.call(ask)
  expect(ran).toEqual([waiting, working])
})

test('an AskUserQuestion that fails still clears', async ($, on) => {
  const ran = runs(on)
  on('tool.call', { tool: 'AskUserQuestion' }, () => {
    throw new Error('dialog gone')
  })
  await $.tool.call(ask).catch(() => undefined)
  expect(ran).toEqual([waiting, working])
})

test('a permission request marks waiting, cleared by the next tool call to settle', async ($, on) => {
  const ran = runs(on)
  on('tool.call', () => ({ result: { stdout: '', stderr: '', interrupted: false } }) as never)
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'ls' } })
  expect(ran).toEqual([waiting])
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'ls' } })
  expect(ran).toEqual([waiting])
  await $.tool.call({ tool: 'Bash', tool_use_id: 'toolu_1', command: 'ls' } as never)
  expect(ran).toEqual([waiting, working])
})

test('a permission request something beneath decided marks nothing', async ($, on) => {
  const ran = runs(on, ['classic.PermissionRequest'])
  on('classic.PermissionRequest', () => ({ decision: { behavior: 'allow' } }))
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'ls' } })
  expect(ran).toEqual([])
})

test('a permission prompt notification marks waiting, cleared by the next model request', async ($, on) => {
  const ran = runs(on)
  await $.classic.Notification({ message: 'Claude needs your permission', notification_type: 'permission_prompt' })
  expect(ran).toEqual([waiting])
  for await (const _ of $.turn.step({ turnId: 't1', index: 1, model: 'm', messageCount: 2 })) void _
  expect(ran).toEqual([waiting, working])
})

test('an idle prompt notification marks nothing', async ($, on) => {
  const ran = runs(on)
  await $.classic.Notification({ message: 'Claude is waiting for your input', notification_type: 'idle_prompt' })
  expect(ran).toEqual([])
})

test('an elicitation waits until its result', async ($, on) => {
  const ran = runs(on)
  await $.classic.Elicitation({ mcp_server_name: 'srv', message: 'Sign in?' })
  expect(ran).toEqual([waiting])
  await $.classic.ElicitationResult({ mcp_server_name: 'srv', action: 'accept' })
  expect(ran).toEqual([waiting, working])
})

test('a wait is cleared only by what ends it', async ($, on) => {
  const ran = runs(on)
  on('tool.call', () => ({ result: { stdout: '', stderr: '', interrupted: false } }) as never)
  await $.classic.Elicitation({ mcp_server_name: 'srv', message: 'Sign in?' })
  await $.tool.call({ tool: 'Bash', tool_use_id: 'toolu_1', command: 'ls' } as never)
  for await (const _ of $.turn.step({ turnId: 't1', index: 1, model: 'm', messageCount: 2 })) void _
  expect(ran).toEqual([waiting])
  // A wait becoming a wait for something else writes nothing.
  await $.classic.Notification({ message: 'permission', notification_type: 'permission_prompt' })
  expect(ran).toEqual([waiting])
})

test('a turn that ends on an error or a refusal waits, cleared by the next turn or prompt', async ($, on) => {
  const ran = runs(on)
  await $.turn.complete(finished('error'))
  expect(ran).toEqual([waiting])
  await $.turn.start({ text: 'go on', turnId: 't2' } as never)
  expect(ran).toEqual([waiting, working])
  await $.turn.complete({ ...(finished('answer') as object), reason: 'refusal', refusal: { category: null, explanation: null } } as never)
  expect(ran).toEqual([waiting, working, waiting])
  await $.prompt.submit({ text: 'try again' } as never)
  expect(ran).toEqual([waiting, working, waiting, working])
})

test('a finished turn, an interrupted one and a subagent error mark nothing', async ($, on) => {
  const ran = runs(on)
  await $.turn.complete(finished('answer'))
  await $.turn.complete(finished('aborted'))
  await $.turn.complete(finished('error', 'agent_1'))
  expect(ran).toEqual([])
})

test('working while already working runs nothing', async ($, on) => {
  const ran = runs(on)
  await $.turn.start({ text: 'hi', turnId: 't1' } as never)
  await $.prompt.submit({ text: 'hi' } as never)
  expect(ran).toEqual([])
})

test('a run that throws or exits non-zero is swallowed into the debug log', async ($, on) => {
  const lines: string[] = []
  on('ui.log', ($, e) => {
    lines.push(`${e.to}: ${e.text}`)
    return { value: undefined }
  })
  let calls = 0
  on('process.run', () => {
    calls += 1
    if (calls === 1) return { deny: 'nat: not found' }
    return { value: { ...ok, exitCode: 1, stderr: 'not an agent pane\n' } }
  })
  engine(on, ['ui.log'])
  await $.turn.complete(finished('error'))
  await $.turn.start({ text: 'go on', turnId: 't2' } as never)
  expect(calls).toBe(2)
  expect(lines).toEqual([
    expect.stringMatching(/^debug: waiting flag \(stuck\) not written: .*nat: not found/),
    'debug: nat agent-working exited 1: not an agent pane',
  ])
})
