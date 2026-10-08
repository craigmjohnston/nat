import type { Register } from 'claude-code'

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
}
