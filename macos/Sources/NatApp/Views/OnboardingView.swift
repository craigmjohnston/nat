import SwiftUI
import NatKit

/// The first-run welcome pane: what the window shows in place of the board
/// when `AppModel.start()` finds no config file, or one naming no projects.
/// There is no wizard of this app's own — project creation stays with the
/// TUI/CLI — so this pane only says what to do next and lets a person check
/// again once they have.
struct OnboardingView: View {
    @Bindable var appModel: AppModel
    @State private var isChecking = false

    private let binaries = ["nat", "tmux", "gh", "ntn"]

    var body: some View {
        ZStack {
            DesignTokens.windowBg
                .ignoresSafeArea()

            VStack(spacing: 20) {
                Text("nat")
                    .font(.system(size: 32, weight: .semibold))
                    .foregroundStyle(DesignTokens.label)

                Text("A native board over the notion-agent-tracker project, for launching and reviewing agent work.")
                    .font(.system(size: Typo.body, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: 360)

                VStack(alignment: .leading, spacing: 8) {
                    ForEach(binaries, id: \.self) { binary in
                        binaryRow(binary)
                    }
                }
                .padding(16)
                .background(DesignTokens.controlBg)
                .cornerRadius(10)

                Text("Run nat in a terminal to set up your workspace, then relaunch.")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.labelTertiary)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: 360)

                Button(action: checkAgain) {
                    if isChecking {
                        ProgressView()
                            .scaleEffect(0.7, anchor: .center)
                            .frame(width: 100)
                    } else {
                        Text("Check Again")
                            .frame(width: 100)
                    }
                }
                .buttonStyle(.borderedProminent)
                .disabled(isChecking)
            }
            .padding(40)
        }
    }

    private func binaryRow(_ binary: String) -> some View {
        let found = BinaryLocator.isFound(binary)
        return HStack(spacing: 8) {
            Image(systemName: found ? "checkmark.circle.fill" : "xmark.circle")
                .foregroundStyle(found ? DesignTokens.systemGreen : DesignTokens.systemRed)
                .font(.system(size: 13, weight: .medium))

            Text(binary)
                .font(.system(size: Typo.code, weight: .regular, design: .monospaced))
                .foregroundStyle(DesignTokens.label)

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
    OnboardingView(appModel: AppModel())
        .frame(width: 720, height: 520)
}
