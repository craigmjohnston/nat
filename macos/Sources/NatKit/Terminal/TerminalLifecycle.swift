import Foundation

/// Why an attached terminal's process ended.
///
/// `tmux attach-session` exits zero whether its session ended under it or
/// the user detached — the Go TUI's own `agentview.go` says as much of its
/// two ways an attach ends — so the reason is never read off the process's
/// own exit status. It is answered separately, by asking tmux afterwards
/// whether the session still exists, which is why [TerminalLifecycleEvent
/// .processTerminated] carries that answer as a plain `Bool` rather than the
/// state machine trying to infer it.
public enum TerminalExitReason: Equatable, Sendable {
    /// The session is still there; only the client went away — a `detach()`
    /// call ahead of tearing the terminal view down, or the user's own
    /// `ctrl+\`-equivalent gesture landing on tmux instead of the agent.
    case detached
    /// The session itself is gone: the agent's process ended, or something
    /// killed the session out from under the client.
    case sessionGone
}

/// The terminal host's state, driven by [TerminalLifecycleEvent]s.
///
/// Modelled after the Go TUI's `agentview.go`: one viewer is attached to one
/// session at a time, it opens unfocused and stays alive until its client's
/// pseudo-terminal ends — the same shape, without anything about focus,
/// which the state machine has no business knowing.
public enum TerminalLifecycleState: Equatable, Sendable {
    /// No attach has been requested yet.
    case idle
    /// A start has been requested but the attach process has not reported
    /// back as launched yet.
    case attaching
    /// The attach process is running and the terminal is live.
    case attached
    /// The attach process ended, for the reason carried.
    case exited(TerminalExitReason)
}

/// An input the terminal host's state machine reacts to.
public enum TerminalLifecycleEvent: Equatable, Sendable {
    /// The host wants a terminal attached — the view appearing, or a
    /// reattach after one exited.
    case startRequested
    /// The attach process has been spawned and is running.
    case processLaunched
    /// The attach process ended. `sessionStillExists` is the caller's own
    /// answer, asked of tmux after the process ended rather than guessed
    /// from its exit status.
    case processTerminated(sessionStillExists: Bool)
}

/// A small state machine for the embedded terminal's lifecycle: idle,
/// attaching, attached, or exited or exited for a reason.
///
/// It knows nothing about tmux, SwiftTerm, or processes — only the shape of
/// the transitions — so `AgentTerminalHostView` can drive it from real
/// callbacks while every transition is exercised here without a window
/// server.
public struct TerminalLifecycle: Equatable, Sendable {
    public private(set) var state: TerminalLifecycleState

    public init(state: TerminalLifecycleState = .idle) {
        self.state = state
    }

    /// Applies event, updating and returning the new state.
    @discardableResult
    public mutating func handle(_ event: TerminalLifecycleEvent) -> TerminalLifecycleState {
        state = Self.transition(from: state, on: event)
        return state
    }

    /// The transition table itself, as a pure function of the current state
    /// and an event — split out from `handle` so a transition can be
    /// asserted without needing a mutable instance around it.
    ///
    /// Anything not named here is not a transition the host ever asks for
    /// and leaves the state exactly as it was: a duplicate `processLaunched`
    /// once attached, a `startRequested` while already attaching, or a
    /// `processTerminated` reaching a machine that was never told anything
    /// had started.
    public static func transition(
        from state: TerminalLifecycleState,
        on event: TerminalLifecycleEvent
    ) -> TerminalLifecycleState {
        switch (state, event) {
        case (.idle, .startRequested):
            return .attaching

        case (.attaching, .processLaunched):
            return .attached

        case (.attaching, .processTerminated(let sessionStillExists)):
            return .exited(sessionStillExists ? .detached : .sessionGone)

        case (.attached, .processTerminated(let sessionStillExists)):
            return .exited(sessionStillExists ? .detached : .sessionGone)

        case (.exited, .startRequested):
            return .attaching

        default:
            return state
        }
    }
}
