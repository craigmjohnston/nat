import type { Register } from 'claude-code'

// nat's hooks into the Claude Code sessions it launches. Written against the
// public mods reference and the declarations the installed build writes beside
// a loaded mod; a hook that fails is skipped and a tree that does not validate
// is replaced by Claude Code's own drawing, so a drift costs a feature, never
// a session.
export const register: Register = on => {
  // The dim `? for shortcuts` line under the prompt draws empty: an agent's
  // pane is driven from gnat, not learned key by key. It is also the one
  // visible mark that a session loaded this mod.
  on('ui.render', { component: 'PromptHint' }, ($, e, next) =>
    next({ ...e, props: { ...e.props, hint: '' } }),
  )
}
