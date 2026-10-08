import { expect, test } from 'claude-code/testing'

const props = { isDraft: false, isWorking: true, hint: '? for shortcuts' }

test('the prompt hint draws empty and keeps every other prop', async ($, on) => {
  // Beneath the plugin, standing for the engine: it records what the plugin
  // handed on and draws that, as the engine's own line would.
  let seen: unknown
  on('ui.render', { component: 'PromptHint' }, ($, e) => {
    seen = e.props
    const { Text } = $.ui.resolve(e)
    return h(Text, {}, e.props.hint)
  })
  for (const surface of ['terminal', 'desktop'] as const) {
    seen = undefined
    const ui = await $.ui.mount({ plugin: 'nat-embedded', surface, component: 'PromptHint', props })
    expect(seen).toEqual({ ...props, hint: '' })
    await ui.unmount()
  }
})
