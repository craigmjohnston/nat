import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

// Each test's own hooks sit beneath the plugin, standing for the engine: a
// stubbed `$.session.id()` and clock, `$.fs.write` and `mv` (through
// `$.process.run`) recorded as they are asked, and the debug log kept.

const path = '/state/agent-status/nat-b4463d8f.session.json'
const start = { cwd: '/work/slice', surface: 'terminal', isInteractive: true } as const
const now = Date.UTC(2026, 9, 10, 11, 22, 33, 456)

type World = { written: { path: string; text: string }[]; ran: string[][]; logged: string[]; asked: number }

function world(
  on: On,
  { env = { NAT_SESSION_RECORD: path }, writeFails = false, mvExit = 0 }: { env?: Record<string, string>; writeFails?: boolean; mvExit?: number } = {},
): World {
  const w: World = { written: [], ran: [], logged: [], asked: 0 }
  mock.env(on, env)
  on('session.id', () => {
    w.asked++
    return { value: '9c4e357d-3e20-44dc-a693-cf42861a7e04' }
  })
  on('clock.now', () => ({ value: now }))
  on('fs.write', ($, e) => {
    if (writeFails) return { deny: `EACCES: ${e.path}` }
    w.written.push({ path: e.path, text: e.text })
    return { value: undefined }
  })
  on('process.run', ($, e) => {
    w.ran.push([...e.argv])
    return { value: { exitCode: mvExit, stdout: '', stderr: mvExit ? 'mv: refused' : '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('ui.log', ($, e) => {
    w.logged.push(e.text)
    return { value: undefined }
  })
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  return w
}

test('session.start records the session id, its directory and when it started, through a temp file moved into place', async ($, on) => {
  const w = world(on)
  expect(await $.session.start(start)).toEqual({ cwd: '/work/slice' })
  expect(w.written).toEqual([
    {
      path: `${path}.tmp`,
      text: JSON.stringify({
        session_id: '9c4e357d-3e20-44dc-a693-cf42861a7e04',
        cwd: '/work/slice',
        started_at: '2026-10-10T11:22:33.456Z',
      }),
    },
  ])
  expect(w.ran).toEqual([['mv', `${path}.tmp`, path]])
})

test('a session nat named no record file for writes none', async ($, on) => {
  const w = world(on, { env: {} })
  expect(await $.session.start(start)).toEqual({ cwd: '/work/slice' })
  expect(w.asked).toBe(0)
  expect(w.written).toEqual([])
  expect(w.ran).toEqual([])
})

test('a record that cannot be written is logged and the session starts all the same', async ($, on) => {
  const w = world(on, { writeFails: true })
  expect(await $.session.start(start)).toEqual({ cwd: '/work/slice' })
  expect(w.ran).toEqual([])
  expect(w.logged.some(line => line.startsWith('session record not written'))).toBe(true)
})

test('a record that cannot be moved into place is logged', async ($, on) => {
  const w = world(on, { mvExit: 1 })
  expect(await $.session.start(start)).toEqual({ cwd: '/work/slice' })
  expect(w.logged).toContain('session record not moved into place: mv: refused')
})
