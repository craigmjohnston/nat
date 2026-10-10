// What the agent is waiting on the user for, as the engine itself knows it:
// an AskUserQuestion dialog, a permission prompt, an MCP elicitation, a turn
// that ended stuck (an API error, a refusal), or a turn that ended with
// nothing handed in (`idle`) — a question in prose, or the agent stopped short.
export type Wait = 'ask' | 'permission' | 'elicitation' | 'stuck' | 'idle'

declare module 'claude-code' {
  interface PluginState {
    // `wait` is the flag the mod last wrote on the pane: null for working.
    // `handedIn` is whether the main agent's current turn has run a hand-in.
    'nat-embedded': { wait: Wait | null; handedIn: boolean }
  }
}
