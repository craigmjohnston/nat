import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

// Each test's own hooks sit beneath the plugin, standing for the engine: an
// inbox in memory answers `$.fs`, `rm` removes from it through
// `$.process.run`, and `$.prompt.submit` records what it was handed. The clock
// is mocked, so the poller ticks only as a test advances it. `own` names the
// events a test answers itself.

const dir = '/state/agent-inbox/nat-b4463d8f'
const start = { cwd: '/work', surface: 'terminal', isInteractive: true } as const

type Inbox = {
  files: Map<string, string>
  submitted: { text: string; asUser?: true }[]
  removed: string[]
}

function inbox(
  on: On,
  files: Record<string, string> = {},
  { env = { NAT_INBOX: dir }, own = [] }: { env?: Record<string, string>; own?: readonly string[] } = {},
): Inbox {
  const box: Inbox = { files: new Map(Object.entries(files).map(([name, text]) => [`${dir}/${name}`, text])), submitted: [], removed: [] }
  mock.env(on, env)
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  if (!own.includes('fs.exists')) on('fs.exists', ($, e) => ({ value: e.path === dir && box.files.size > 0 }))
  on('fs.list', ($, e) => ({
    value: [...box.files.keys()]
      .filter(path => path.startsWith(`${e.path}/`))
      .map(path => ({ name: path.slice(e.path.length + 1), kind: 'file' as const, size: 1, mtimeMs: 0, isLink: false })),
  }))
  if (!own.includes('fs.read'))
    on('fs.read', ($, e) => {
      const text = box.files.get(e.path)
      if (text === undefined) return { deny: `ENOENT: ${e.path}` }
      return { value: text }
    })
  on('process.run', ($, e) => {
    const [command, path] = e.argv
    const gone = command !== 'rm' || !box.files.delete(path)
    if (!gone) box.removed.push(path)
    return { value: { exitCode: gone ? 1 : 0, stdout: '', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('prompt.submit', ($, e) => {
    box.submitted.push({ text: e.text, ...(e.origin?.kind === 'plugin' && e.origin.asUser ? { asUser: true as const } : {}) })
    return { text: e.text }
  })
  if (!own.includes('ui.log')) on('ui.log', () => ({ value: undefined }))
  return box
}

test('a file present at start is delivered on the first tick, as the user, and removed', async ($, on) => {
  const clock = mock.clock(on)
  const box = inbox(on, { '1700000000000000001.md': 'a note arrived' })
  await $.session.start(start)
  expect(box.submitted).toEqual([])
  await clock.advance(1000)
  expect(box.submitted).toEqual([{ text: 'a note arrived', asUser: true }])
  expect(box.removed).toEqual([`${dir}/1700000000000000001.md`])
  expect(box.files.size).toBe(0)
})

test('files are delivered in name order, which is send order', async ($, on) => {
  const clock = mock.clock(on)
  const box = inbox(on, {
    '1700000000000000003.md': 'third',
    '1700000000000000001.md': 'first',
    '1700000000000000002.md': 'second',
  })
  await $.session.start(start)
  await clock.advance(1000)
  expect(box.submitted.map(s => s.text)).toEqual(['first', 'second', 'third'])
})

test('a file sent later is delivered on a later tick', async ($, on) => {
  const clock = mock.clock(on)
  const box = inbox(on)
  await $.session.start(start)
  await clock.advance(1000)
  expect(box.submitted).toEqual([])
  box.files.set(`${dir}/1700000000000000005.md`, 'go on')
  await clock.advance(1000)
  expect(box.submitted.map(s => s.text)).toEqual(['go on'])
  await clock.advance(1000)
  expect(box.submitted.map(s => s.text)).toEqual(['go on'])
})

test('a temp file still being written is left alone', async ($, on) => {
  const clock = mock.clock(on)
  const box = inbox(on, { '.1700000000000000001.tmp': 'half a note' })
  await $.session.start(start)
  await clock.advance(1000)
  expect(box.submitted).toEqual([])
  expect(box.files.size).toBe(1)
})

test('a file nat took back before the mod could remove it is never submitted', async ($, on) => {
  const clock = mock.clock(on)
  const box = inbox(on, { '1700000000000000001.md': 'pasted instead' }, { own: ['fs.read'] })
  // nat removes the file between the mod's read and its rm.
  on('fs.read', ($, e) => {
    const text = box.files.get(e.path) ?? ''
    box.files.delete(e.path)
    return { value: text }
  })
  await $.session.start(start)
  await clock.advance(1000)
  expect(box.submitted).toEqual([])
})

test('a missing or empty inbox submits nothing', async ($, on) => {
  const clock = mock.clock(on)
  const box = inbox(on)
  await $.session.start(start)
  await clock.advance(3000)
  expect(box.submitted).toEqual([])
  expect(box.removed).toEqual([])
})

test('a session nat launched with no inbox polls nothing', async ($, on) => {
  const clock = mock.clock(on)
  let looked = 0
  const box = inbox(on, { '1700000000000000001.md': 'never' }, { env: {}, own: ['fs.exists'] })
  on('fs.exists', () => {
    looked += 1
    return { value: true }
  })
  await $.session.start(start)
  await clock.advance(3000)
  expect(looked).toBe(0)
  expect(box.submitted).toEqual([])
})

test('a read that fails is logged and tried again on the next tick', async ($, on) => {
  const clock = mock.clock(on)
  const lines: string[] = []
  const box = inbox(on, { '1700000000000000001.md': 'second try' }, { own: ['fs.read', 'ui.log'] })
  let reads = 0
  on('fs.read', ($, e) => {
    reads += 1
    if (reads === 1) return { deny: 'EIO' }
    return { value: box.files.get(e.path) ?? '' }
  })
  on('ui.log', ($, e) => {
    lines.push(`${e.to}: ${e.text}`)
    return { value: undefined }
  })
  await $.session.start(start)
  await clock.advance(1000)
  expect(box.submitted).toEqual([])
  expect(lines).toEqual([expect.stringMatching(/^debug: agent inbox not read: .*EIO/)])
  await clock.advance(1000)
  expect(box.submitted.map(s => s.text)).toEqual(['second try'])
})
