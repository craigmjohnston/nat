// What the agent is waiting on the user for, as the engine itself knows it:
// an AskUserQuestion dialog, a permission prompt, an MCP elicitation, or a
// turn that ended stuck (an API error, a refusal).
export type Wait = 'ask' | 'permission' | 'elicitation' | 'stuck'

declare module 'claude-code' {
  interface PluginState {
    // `wait` is the flag the mod last wrote on the pane: null for working.
    'nat-embedded': { wait: Wait | null }
  }
}
