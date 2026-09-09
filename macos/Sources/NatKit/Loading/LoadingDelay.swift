import Foundation

/// Whether a loading indicator has been showing long enough to be worth
/// showing at all — the "show-after-delay" half of "cache it and load in the
/// background so spinners are rare": a cache hit or a fast read finishes
/// before the delay elapses and the indicator it would have needed never
/// appears, so nothing flashes on screen for it.
///
/// `QuietLoadingView` is the one caller — it starts this in a `.task` the
/// moment it is mounted, and SwiftUI cancels that task the moment the view
/// it belongs to is removed (because whatever it was waiting on has already
/// finished), which is what makes a fast load silent.
public struct LoadingDelay: Sendable {
    /// How long a load has to run before it is worth admitting to — the
    /// same "not worth a spinner" threshold most native macOS progress UI
    /// uses.
    public static let defaultDuration: Duration = .milliseconds(250)

    public let duration: Duration

    public init(duration: Duration = LoadingDelay.defaultDuration) {
        self.duration = duration
    }

    /// Waits out `duration` and answers true — unless cancelled first, in
    /// which case it answers false without the delay ever having elapsed.
    /// Cancellation is exactly what a `.task` still in this wait gets when
    /// the view hosting it disappears (or is replaced by fresh content), so
    /// "false" means precisely "don't bother revealing anything — it's
    /// already over."
    public func shouldReveal() async -> Bool {
        do {
            try await Task.sleep(for: duration)
        } catch {
            return false
        }
        return !Task.isCancelled
    }
}
