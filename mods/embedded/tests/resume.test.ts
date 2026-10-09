import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

// Each test's own hooks sit beneath the plugin, standing for the engine: its
// `process.run` records each command the plugin ran with what it was handed
// on stdin, and answers with `exitCode`; the waiting flag's own `nat
// agent-*` runs are left out, being waiting.test.ts's. `own` names the events
// a test answers itself.

const slice = '3f338308-f654-81bc-af29-d65871bf969d'
const project = '3b738308-f654-811c-948d-e1fb36f71df3'
const both = { NAT_SLICE: slice, NAT_PROJECT: project }

type Ran = { argv: string[]; stdin?: string }

function engine(
  on: On,
  { env = both, exitCode = 0, own = [] }: { env?: Record<string, string>; exitCode?: number; own?: readonly string[] } = {},
): { ran: Ran[]; submitted: string[] } {
  const ran: Ran[] = []
  const submitted: string[] = []
  mock.env(on, env)
  if (!own.includes('process.run'))
    on('process.run', ($, e) => {
      if (e.argv[1] !== 'agent-working' && e.argv[1] !== 'agent-waiting') ran.push({ argv: [...e.argv], stdin: e.init?.stdin })
      return { value: { exitCode, stdout: '', stderr: exitCode ? 'refused' : '', isStdoutTruncated: false, isStderrTruncated: false } }
    })
  on('prompt.submit', ($, e) => {
    submitted.push(e.text)
    return { text: e.text }
  })
  if (!own.includes('ui.log')) on('ui.log', () => ({ value: undefined }))
  return { ran, submitted }
}

const typed = (text: string) => ({ text, wait: false, origin: { kind: 'composer' } }) as never
const resumed = (text: string): Ran => ({
  argv: ['nat', 'slice-resume', slice, '--project', project, '--note', '-'],
  stdin: text,
})

test('a prompt typed at a slice agent runs slice-resume with its text on stdin, before it submits', async ($, on) => {
  const ran: (Ran & { submittedBefore: number })[] = []
  const { submitted } = engine(on, { own: ['process.run'] })
  on('process.run', ($, e) => {
    if (e.argv[1] === 'slice-resume') ran.push({ argv: [...e.argv], stdin: e.init?.stdin, submittedBefore: submitted.length })
    return { value: { exitCode: 0, stdout: '', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  await $.prompt.submit(typed('Add a footer too.'))
  expect(ran).toEqual([{ ...resumed('Add a footer too.'), submittedBefore: 0 }])
  expect(submitted).toEqual(['Add a footer too.'])
})

test('a prompt sent through Remote Control is the user asking too', async ($, on) => {
  const { ran } = engine(on)
  await $.prompt.submit({ text: 'And the header.', wait: false, origin: { kind: 'bridge' } } as never)
  expect(ran).toEqual([resumed('And the header.')])
})

test('a prompt the inbox poller submitted runs nothing', async ($, on) => {
  const { ran, submitted } = engine(on)
  await $.prompt.submit({ text: 'a note arrived', wait: false, origin: { kind: 'plugin', name: 'nat-embedded', asUser: true } } as never)
  expect(ran).toEqual([])
  expect(submitted).toEqual(['a note arrived'])
})

test("a background task's notification runs nothing", async ($, on) => {
  const { ran } = engine(on)
  await $.prompt.submit({ text: '<task-notification>', wait: false, origin: { kind: 'task-notification' } } as never)
  expect(ran).toEqual([])
})

test('a session with no NAT_SLICE runs nothing', async ($, on) => {
  const { ran, submitted } = engine(on, { env: { NAT_PROJECT: project } })
  await $.prompt.submit(typed('Plan a milestone.'))
  expect(ran).toEqual([])
  expect(submitted).toEqual(['Plan a milestone.'])
})

test('a session with no NAT_PROJECT runs nothing', async ($, on) => {
  const { ran } = engine(on, { env: { NAT_SLICE: slice } })
  await $.prompt.submit(typed('hi'))
  expect(ran).toEqual([])
})

test('a run that exits non-zero is logged to debug and the prompt still submits', async ($, on) => {
  const lines: string[] = []
  const { submitted } = engine(on, { exitCode: 1, own: ['ui.log'] })
  on('ui.log', ($, e) => {
    lines.push(`${e.to}: ${e.text}`)
    return { value: undefined }
  })
  await $.prompt.submit(typed('Add a footer.'))
  expect(lines).toEqual(['debug: nat slice-resume exited 1: refused'])
  expect(submitted).toEqual(['Add a footer.'])
})

test('a run that throws is logged to debug and the prompt still submits', async ($, on) => {
  const lines: string[] = []
  const { submitted } = engine(on, { own: ['ui.log', 'process.run'] })
  on('process.run', ($, e) => {
    if (e.argv[1] === 'slice-resume') return { deny: 'nat: not found' }
    return { value: { exitCode: 0, stdout: '', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('ui.log', ($, e) => {
    lines.push(`${e.to}: ${e.text}`)
    return { value: undefined }
  })
  await $.prompt.submit(typed('Add a footer.'))
  expect(lines).toEqual([expect.stringMatching(/^debug: resume not recorded: .*nat: not found/)])
  expect(submitted).toEqual(['Add a footer.'])
})
