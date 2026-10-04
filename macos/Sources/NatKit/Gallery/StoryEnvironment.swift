import SwiftUI

private struct PulsesPausedKey: EnvironmentKey {
    static let defaultValue = false
}

extension EnvironmentValues {
    /// Set by the gallery over every story so a capture never lands
    /// mid-animation: a live dot draws without its pulse and a skeleton block
    /// flat, without its sweep. Here rather than beside the dot, since the
    /// skeleton is NatKit's.
    public var pulsesPaused: Bool {
        get { self[PulsesPausedKey.self] }
        set { self[PulsesPausedKey.self] = newValue }
    }
}
