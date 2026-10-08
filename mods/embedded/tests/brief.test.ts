import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

// Each test's own hooks sit beneath the plugin, standing for the engine: the
// context blocks core computed, `$.fs.read` answered from a map of files, and
// `$.ui.log` recorded. `env` is what nat set on the launch.

const path = '/state/prompts/nat-b4463d8f.md'
const engine = [
  { name: 'claudeMd', text: 'Contents of CLAUDE.md' },
  { name: 'currentDate', text: "Today's date is 2026-10-08." },
]

function context(on: On, env: Record<string, string>, files: Record<string, string> = {}): string[] {
  const lines: string[] = []
  mock.env(on, env)
  on('prompt.context', ($, e) => ({ blocks: e.blocks }))
  on('fs.read', ($, e) => {
    const text = files[e.path]
    if (text === undefined) return { deny: `ENOENT: ${e.path}` }
    return { value: text }
  })
  on('ui.log', ($, e) => {
    lines.push(`${e.to}: ${e.text}`)
    return { value: undefined }
  })
  return lines
}

test('the brief NAT_BRIEF names is appended after the engine blocks, in order', async ($, on) => {
  context(on, { NAT_BRIEF: path }, { [path]: 'You are a Claude Code agent working exactly one slice.' })
  const { blocks } = await $.prompt.context({ blocks: engine })
  expect(blocks).toEqual([...engine, { name: 'natBrief', text: 'You are a Claude Code agent working exactly one slice.' }])
})

test('a session nat launched with no brief passes the blocks through', async ($, on) => {
  const lines = context(on, {})
  const { blocks } = await $.prompt.context({ blocks: engine })
  expect(blocks).toEqual(engine)
  expect(lines).toEqual([])
})

test('a brief that cannot be read is logged and the blocks pass through', async ($, on) => {
  const lines = context(on, { NAT_BRIEF: path })
  const { blocks } = await $.prompt.context({ blocks: engine })
  expect(blocks).toEqual(engine)
  expect(lines).toEqual([expect.stringMatching(/^debug: agent brief not read: .*ENOENT/)])
})
