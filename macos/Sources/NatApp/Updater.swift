import Combine
import Sparkle
import SwiftUI

/// Sparkle's own update flow — the alert, "Install Update", "Install and
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
        controller = SPUStandardUpdaterController(
            startingUpdater: true,
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

/// The "Check for Updates…" item NatApp adds to `CommandGroup(after:
/// .appInfo)`.
struct CheckForUpdatesView: View {
    @ObservedObject var model: UpdaterViewModel

    var body: some View {
        Button("Check for Updates…") {
            model.checkForUpdates()
        }
        .disabled(!model.canCheckForUpdates)
    }
}
