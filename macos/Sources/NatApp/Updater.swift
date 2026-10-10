import Combine
import NatKit
import Sparkle
import SwiftUI

/// Sparkle's own update flow — the alert, "Install update", "Install and
/// Relaunch" — needs nothing from this app beyond starting it and offering a
/// menu item to trigger a check by hand; `SUEnableAutomaticChecks` in the
/// Info.plist is what makes it also check on Sparkle's own schedule.
/// `SUAutomaticallyUpdate` is deliberately never set, so an update is never
/// installed without the user clicking through it.
@MainActor
final class UpdaterViewModel: ObservableObject {
    private let controller: SPUStandardUpdaterController

    /// Mirrors the updater's own `canCheckForUpdates`, which is false while
    /// a check or an update is already under way — exactly the span the
    /// menu item should be disabled for.
    @Published private(set) var canCheckForUpdates = false

    private var cancellable: AnyCancellable?

    init() {
        // A dev executable never starts it: see `UpdaterGate`. The menu
        // item then stays disabled, since `canCheckForUpdates` never rises.
        controller = SPUStandardUpdaterController(
            startingUpdater: UpdaterGate.shouldStart(bundleURL: Bundle.main.bundleURL),
            updaterDelegate: nil,
            userDriverDelegate: nil
        )
        cancellable = controller.updater.publisher(for: \.canCheckForUpdates)
            .receive(on: DispatchQueue.main)
            .sink { [weak self] canCheck in
                self?.canCheckForUpdates = canCheck
            }
    }

    func checkForUpdates() {
        controller.checkForUpdates(nil)
    }
}

/// The app menu's About gnat: Settings opened (or brought to the front) on
/// its About section, in place of the standard About panel. The request is
/// left before the window opens, so a window built for it finds it waiting.
struct AboutGnatButton: View {
    let appModel: AppModel
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        Button("About gnat") {
            appModel.requestSettingsAbout()
            openSettings()
        }
    }
}

/// The "Check for updates…" item NatApp adds to `CommandGroup(after:
/// .appInfo)`, and Settings ▸ About's button (titled as a button is).
struct CheckForUpdatesView: View {
    @ObservedObject var model: UpdaterViewModel
    var title = "Check for updates…"

    var body: some View {
        Button(title) {
            model.checkForUpdates()
        }
        .disabled(!model.canCheckForUpdates)
    }
}
