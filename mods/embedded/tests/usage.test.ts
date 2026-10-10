import type { On, SessionRateLimit } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

// Each test's own hooks sit beneath the plugin, standing for the engine: the
// clock reads a fixed moment, `$.fs.write` and `$.process.run` record what
// they were handed, and `$.ui.log` what it was. `env` is what nat set on the
// launch.

const path = '/state/agent-status/nat-b4463d8f.usage.json'
const now = Date.UTC(2026, 9, 10, 12, 0, 0)
const context = { window: 200_000 }

type Recorded = { writes: { path: string; text: string }[]; runs: string[][]; lines: string[] }

function engine(on: On, env: Record<string, string> = { NAT_USAGE: path }, mvExit = 0, writeDeny?: string): Recorded {
  const rec: Recorded = { writes: [], runs: [], lines: [] }
  mock.env(on, env)
  on('session.measure', ($, e) => ({ changed: e.changed }))
  on('clock.now', () => ({ value: now }))
  on('fs.write', ($, e) => {
    if (writeDeny) return { deny: writeDeny }
    rec.writes.push({ path: e.path, text: e.text })
    return { value: undefined }
  })
  on('process.run', ($, e) => {
    rec.runs.push([...e.argv])
    return { value: { exitCode: mvExit, stdout: '', stderr: mvExit ? 'mv: rename failed' : '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('ui.log', ($, e) => {
    rec.lines.push(`${e.to}: ${e.text}`)
    return { value: undefined }
  })
  return rec
}

function written(rec: Recorded): unknown {
  expect(rec.writes.map(w => w.path)).toEqual([`${path}.tmp`])
  return JSON.parse(rec.writes[0].text)
}

const fiveHour: SessionRateLimit = { kind: 'five_hour', percentUsed: 23.5, resetsAt: '2026-10-10T15:00:00.000Z' }
const sevenDay: SessionRateLimit = { kind: 'seven_day', percentUsed: 61, resetsAt: '2026-10-14T09:00:00.000Z' }

test('a reading with both windows is written through a temp file and moved into place', async ($, on) => {
  const rec = engine(on)
  const result = await $.session.measure({ context, rateLimits: [fiveHour, sevenDay], changed: ['rateLimits'] })
  expect(result).toEqual({ changed: ['rateLimits'] })
  expect(written(rec)).toEqual({
    read_at: '2026-10-10T12:00:00.000Z',
    rate_limits: {
      five_hour: { used_percentage: 23.5, resets_at: '2026-10-10T15:00:00.000Z' },
      seven_day: { used_percentage: 61, resets_at: '2026-10-14T09:00:00.000Z' },
    },
  })
  expect(rec.runs).toEqual([['mv', `${path}.tmp`, path]])
  expect(rec.lines).toEqual([])
})

test('a reading with one window writes that window alone, and a spend limit is left out', async ($, on) => {
  const rec = engine(on)
  await $.session.measure({
    context,
    rateLimits: [{ kind: 'seven_day', percentUsed: 7 }, { kind: 'spend_limit', percentUsed: 120 }],
    changed: ['context'],
  })
  expect(written(rec)).toEqual({ read_at: '2026-10-10T12:00:00.000Z', rate_limits: { seven_day: { used_percentage: 7 } } })
  expect(rec.runs).toEqual([['mv', `${path}.tmp`, path]])
})

test('a reading with no window writes nothing', async ($, on) => {
  const rec = engine(on)
  const result = await $.session.measure({ context, rateLimits: [], changed: ['context'] })
  expect(result).toEqual({ changed: ['context'] })
  expect(rec.writes).toEqual([])
  expect(rec.runs).toEqual([])
})

test('a session nat launched with no usage file writes nothing', async ($, on) => {
  const rec = engine(on, {})
  await $.session.measure({ context, rateLimits: [fiveHour], changed: ['rateLimits'] })
  expect(rec.writes).toEqual([])
  expect(rec.runs).toEqual([])
})

test('a move that fails is logged, and the measurement passes on', async ($, on) => {
  const rec = engine(on, { NAT_USAGE: path }, 1)
  const result = await $.session.measure({ context, rateLimits: [fiveHour], changed: ['rateLimits'] })
  expect(result).toEqual({ changed: ['rateLimits'] })
  expect(rec.lines).toEqual(['debug: usage file not moved into place: mv exited 1: mv: rename failed'])
})

test('a write that fails is logged, and the measurement passes on', async ($, on) => {
  const rec = engine(on, { NAT_USAGE: path }, 0, 'EACCES')
  const result = await $.session.measure({ context, rateLimits: [fiveHour], changed: ['rateLimits'] })
  expect(result).toEqual({ changed: ['rateLimits'] })
  expect(rec.runs).toEqual([])
  expect(rec.lines).toEqual([expect.stringMatching(/^debug: usage file not written: .*EACCES/)])
})
