import Foundation

/// Whether Sparkle's updater should start at all.
///
/// Only a bundled `gnat.app` has the Info.plist (feed URL, public key) an
/// update check needs. A bare dev executable — `swift run`,
/// `macos/.build/debug/gnat` — has none, and Sparkle answers that with an
/// app-modal alert at launch; under it nothing in the main run loop's
/// default mode runs and key presses go to the alert, which is what stalled
/// the `NAT_KEY_DEBUG` harness (`docs/debugging/agent-pane-keys.md`).
public enum UpdaterGate {
    public static func shouldStart(bundleURL: URL) -> Bool {
        bundleURL.pathExtension == "app"
    }
}
