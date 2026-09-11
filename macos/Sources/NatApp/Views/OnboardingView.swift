import SwiftUI
import NatKit

/// The first-run welcome pane: what the window shows in place of the board
/// when `AppModel.start()` finds no config file, or one naming no projects.
///
/// With `nat` on the machine it is a way in rather than a dead end: the same
/// sheet the "+" tab opens, which creates a project or opens one the
/// workspace already has, and either lands the board's first tab. Without it
/// there is nothing for the sheet to run, so the pane says what to install
/// and offers the check again — the state it was in before there was a sheet
/// at all.
struct OnboardingView: View {
    @Bindable var appModel: AppModel

    /// Opens the "+" tab's own sheet, which the window presents.
    let onNewProject: () -> Void

    @State private var isChecking = false

    private let binaries = ["nat", "tmux", "gh", "ntn"]

    var body: some View {
        ZStack {
            DesignTokens.fill(.window)
                .ignoresSafeArea()

            VStack(spacing: 20) {
                Text("nat")
                    .font(.system(size: 32, weight: .semibold))
                    .ink(.primary)

                Text("A native board over the notion-agent-tracker project, for launching and reviewing agent work.")
                    .font(.system(size: Typo.body, weight: .regular))
                    .ink(.secondary)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: 360)

                VStack(alignment: .leading, spacing: 8) {
                    ForEach(binaries, id: \.self) { binary in
                        binaryRow(binary)
                    }
                }
                .padding(16)
                .surface(.card)
                .cornerRadius(10)

                Text(natFound
                    ? "Add a project to get started — one the workspace already has, or a new one."
                    : "Install nat and run it once in a terminal to set up your workspace, then check again.")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.tertiary)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: 360)

                // The prominent button is whichever one is the thing to do:
                // adding a project where that is possible, and checking again
                // where the only way forward is a terminal.
                HStack(spacing: 10) {
                    if natFound {
                        Button(action: onNewProject) {
                            Text("Add a Project…")
                                .frame(width: 120)
                        }
                        .buttonStyle(PrimaryButtonStyle())

                        checkAgainButton.buttonStyle(SecondaryButtonStyle())
                    } else {
                        checkAgainButton.buttonStyle(PrimaryButtonStyle())
                    }
                }
            }
            .padding(40)
        }
    }

    private var checkAgainButton: some View {
        Button(action: checkAgain) {
            AsyncActionLabel(isBusy: isChecking) {
                Text("Check Again")
                    .frame(width: 100)
            }
        }
        .disabled(isChecking)
    }

    /// Whether the sheet has anything to run: `nat` is what both of its
    /// paths are, so a machine without it is offered neither.
    private var natFound: Bool { BinaryLocator.isFound("nat") }

    private func binaryRow(_ binary: String) -> some View {
        let found = BinaryLocator.isFound(binary)
        return HStack(spacing: 8) {
            Image(systemName: found ? "checkmark.circle.fill" : "xmark.circle")
                .foregroundStyle(found ? DesignTokens.systemGreen : DesignTokens.systemRed)
                .font(.system(size: 13, weight: .medium))

            Text(binary)
                .font(.system(size: Typo.code, weight: .regular, design: .monospaced))
                .ink(.primary)

            Spacer()

            Text(found ? "Found" : "Missing")
                .font(.system(size: Typo.subhead, weight: .regular))
                .foregroundStyle(found ? DesignTokens.labelSecondary : DesignTokens.systemRed)
        }
        .frame(width: 220)
    }

    private func checkAgain() {
        Task {
            isChecking = true
            await appModel.start()
            isChecking = false
        }
    }
}

#Preview {
    OnboardingView(appModel: AppModel(), onNewProject: {})
        .frame(width: 720, height: 520)
}
