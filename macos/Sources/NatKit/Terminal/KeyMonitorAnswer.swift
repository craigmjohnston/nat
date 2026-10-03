/// What a local `NSEvent` monitor owned by a view hands back to AppKit: the
/// event to go on delivering, or nil to swallow it.
///
/// Spelled out because the obvious closure gets it wrong. A monitor that
/// holds its view weakly reads `self?.handle(event) ?? event` — and since
/// `handle` itself answers nil to swallow, `?? event` turns every swallow
/// back into a delivery. That was the agent pane's shift+enter: the monitor
/// sent the CSI-u and answered nil, and the same event still reached
/// SwiftTerm's `keyDown`, which sent a return after it — a line break and a
/// submit. `docs/debugging/agent-pane-keys.md` has the trace.
///
/// Only a view that is gone passes the event on untouched; a live view's
/// answer stands as given, nil included.
public enum KeyMonitorAnswer {
    public static func answer<Owner: AnyObject, Event>(
        _ event: Event,
        owner: Owner?,
        handle: (Owner, Event) -> Event?
    ) -> Event? {
        guard let owner else { return event }
        return handle(owner, event)
    }
}
