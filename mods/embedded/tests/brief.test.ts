import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

// Each test's own hooks sit beneath the plugin, standing for the engine: the
// prompt and context that entered recorded, the context blocks core computed,
// `$.session.turns()` answered with the turns the conversation already has,
// `$.fs.read` answered from a map of files, and `$.ui.log` recorded. `env` is
// what nat set on the launch.

const path = '/state/prompts/nat-b4463d8f.md'
const brief = 'You are a Claude Code agent working exactly one slice.'
const opening = 'Work the slice "Tidy the footer": your brief is the natBrief block of this message.'
const blocks = [
  { name: 'claudeMd', text: 'Contents of CLAUDE.md' },
  { name: 'currentDate', text: "Today's date is 2026-10-08." },
]

type Entered = { text: string; context?: readonly string[] }

function engine(
  on: On,
  env: Record<string, string>,
  { files = {}, turns = 0 }: { files?: Record<string, string>; turns?: number | string } = {},
): { entered: Entered[]; lines: string[] } {
  const entered: Entered[] = []
  const lines: string[] = []
  mock.env(on, env)
  on('prompt.submit', ($, e) => {
    entered.push(e.context ? { text: e.text, context: e.context } : { text: e.text })
    return { text: e.text, context: e.context }
  })
  on('prompt.context', ($, e) => ({ blocks: e.blocks }))
  on('session.turns', () => (typeof turns === 'number' ? { value: turns } : { deny: turns }))
  on('fs.read', ($, e) => {
    const text = files[e.path]
    if (text === undefined) return { deny: `ENOENT: ${e.path}` }
    return { value: text }
  })
  on('process.run', () => ({ value: { exitCode: 0, stdout: '', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }))
  on('ui.log', ($, e) => {
    lines.push(`${e.to}: ${e.text}`)
    return { value: undefined }
  })
  return { entered, lines }
}

const submit = (text: string, context?: readonly string[]) =>
  ({ text, wait: false, origin: { kind: 'composer' }, ...(context ? { context } : {}) }) as never

test('the opening prompt carries the brief NAT_BRIEF names as the natBrief block, after any context', async ($, on) => {
  const { entered } = engine(on, { NAT_BRIEF: path }, { files: { [path]: brief } })
  await $.prompt.submit(submit(opening, ['earlier']))
  expect(entered).toEqual([{ text: opening, context: ['earlier', `<natBrief>\n${brief}\n</natBrief>`] }])
})

test("the opening prompt's own context blocks, computed with no turn yet, pass it over", async ($, on) => {
  engine(on, { NAT_BRIEF: path }, { files: { [path]: brief } })
  await $.prompt.submit(submit(opening))
  expect((await $.prompt.context({ blocks })).blocks).toEqual(blocks)
})

test("a prompt after the first turn, or a resumed conversation's, carries no brief", async ($, on) => {
  const { entered, lines } = engine(on, { NAT_BRIEF: path }, { files: { [path]: brief }, turns: 1 })
  await $.prompt.submit(submit('Continue from where you left off.'))
  expect(entered).toEqual([{ text: 'Continue from where you left off.' }])
  expect(lines).toEqual([])
})

test('the blocks computed afresh after a compaction or a /clear carry the brief after the engine blocks', async ($, on) => {
  engine(on, { NAT_BRIEF: path }, { files: { [path]: brief }, turns: 3 })
  expect((await $.prompt.context({ blocks })).blocks).toEqual([...blocks, { name: 'natBrief', text: brief }])
})

test('a session nat launched with no brief passes the prompt and the blocks through', async ($, on) => {
  const { entered, lines } = engine(on, {})
  await $.prompt.submit(submit(opening))
  expect(entered).toEqual([{ text: opening }])
  expect((await $.prompt.context({ blocks })).blocks).toEqual(blocks)
  expect(lines).toEqual([])
})

test('a brief that cannot be read is logged and the opening prompt passes through', async ($, on) => {
  const { entered, lines } = engine(on, { NAT_BRIEF: path })
  await $.prompt.submit(submit(opening))
  expect(entered).toEqual([{ text: opening }])
  expect(lines).toEqual([expect.stringMatching(/^debug: agent brief not read: .*ENOENT/)])
})

test("a brief that cannot be read is logged and a compaction's blocks pass through", async ($, on) => {
  const { lines } = engine(on, { NAT_BRIEF: path }, { turns: 3 })
  expect((await $.prompt.context({ blocks })).blocks).toEqual(blocks)
  expect(lines).toEqual([expect.stringMatching(/^debug: agent brief not read: .*ENOENT/)])
})

test('a turn count that cannot be read is logged: the prompt passes through and the blocks carry the brief', async ($, on) => {
  const { entered, lines } = engine(on, { NAT_BRIEF: path }, { files: { [path]: brief }, turns: 'no session' })
  await $.prompt.submit(submit(opening))
  expect(entered).toEqual([{ text: opening }])
  expect((await $.prompt.context({ blocks })).blocks).toEqual([...blocks, { name: 'natBrief', text: brief }])
  expect(lines).toEqual([
    expect.stringMatching(/^debug: turns not read for the agent brief: .*no session/),
    expect.stringMatching(/^debug: turns not read for the agent brief: .*no session/),
  ])
})
