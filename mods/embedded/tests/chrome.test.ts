import { expect, test } from 'claude-code/testing'

// Each test's own hook sits beneath the plugin, standing for the engine: it
// records what the plugin handed on and draws that, as the engine's own line
// would. Where the plugin answers with a tree of its own, it never runs.

test('the turn duration line draws nothing', async ($, on) => {
  const props = { word: 'Baked', durationMs: 3000 }
  let seen: unknown
  on('ui.render', { component: 'TurnDuration' }, ($, e) => {
    seen = e.props
    const { Text } = $.ui.resolve(e)
    return h(Text, {}, `${e.props.word} for 3s`)
  })
  const ui = await $.ui.mount({ plugin: 'nat-embedded', surface: 'terminal', component: 'TurnDuration', props })
  expect(seen).toBeUndefined()
  expect(await ui.drawn()).toMatchObject({ type: 'Box' })
  expect(await ui.findAll({ type: 'Text' })).toHaveLength(0)
  await ui.unmount()
})

test('the notices under the logo draw nothing', async ($, on) => {
  const props = { text: 'Using model from settings', command: '/model' }
  let seen: unknown
  on('ui.render', { component: 'InfoNotice' }, ($, e) => {
    seen = e.props
    const { Text } = $.ui.resolve(e)
    return h(Text, {}, e.props.text)
  })
  const ui = await $.ui.mount({ plugin: 'nat-embedded', surface: 'terminal', component: 'InfoNotice', props })
  expect(seen).toBeUndefined()
  expect(await ui.drawn()).toMatchObject({ type: 'Box' })
  expect(await ui.findAll({ type: 'Text' })).toHaveLength(0)
  await ui.unmount()
})

test('the run-in-background pill draws empty and keeps every other prop', async ($, on) => {
  const props = { tool_use_id: 'toolu_1', kind: 'background_hint', hint: '(ctrl+b to run in background)' } as const
  let seen: unknown
  on('ui.render', { component: 'ToolProgress' }, ($, e) => {
    seen = e.props
    const { Text } = $.ui.resolve(e)
    return h(Text, {}, e.props.hint)
  })
  const ui = await $.ui.mount({ plugin: 'nat-embedded', surface: 'terminal', component: 'ToolProgress', props })
  expect(seen).toEqual({ ...props, hint: '' })
  await ui.unmount()
})

test('the spinner says Working and keeps every other prop', async ($, on) => {
  const props = { word: 'Sauteing', message: null, suffix: '…', mode: 'responding' } as const
  let seen: unknown
  on('ui.render', { component: 'Spinner' }, ($, e) => {
    seen = e.props
    const { Text } = $.ui.resolve(e)
    return h(Text, {}, e.props.word)
  })
  for (const surface of ['terminal', 'desktop'] as const) {
    seen = undefined
    const ui = await $.ui.mount({ plugin: 'nat-embedded', surface, component: 'Spinner', props })
    expect(seen).toEqual({ ...props, word: 'Working' })
    await ui.unmount()
  }
})

test('the session modes are left as the engine hands them', async ($, on) => {
  const props = { modes: ['plan', 'focus'] }
  let seen: unknown
  on('ui.render', { component: 'SessionMode' }, ($, e) => {
    seen = e.props
    const { Text } = $.ui.resolve(e)
    return h(Text, {}, e.props.modes.join(' & '))
  })
  for (const surface of ['terminal', 'desktop'] as const) {
    seen = undefined
    const ui = await $.ui.mount({ plugin: 'nat-embedded', surface, component: 'SessionMode', props })
    expect(seen).toEqual(props)
    await ui.unmount()
  }
})
