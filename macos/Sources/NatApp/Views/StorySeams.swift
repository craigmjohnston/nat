import SwiftUI
import NatKit

/// The two places a story cannot draw the real thing, and what it draws
/// instead.
///
/// Everything else in the gallery is the app's own view over a fixture value:
/// the rail, the brief, the diff and the pull request are all drawn by the
/// same code the running app draws them with, reading a canned answer instead
/// of a live one. Two regions have no answer to can, because what they show
/// is not data at all — the agent terminal is a tmux session attached to over
/// a pseudo-terminal, and the onboarding checklist is the machine the app is
/// running on. Both are declared here as environment values with the real
/// thing as their default, so nothing changes for the app and a story says
/// which of the two it is pinning.

// MARK: - The terminal region

private struct TerminalStubbedKey: EnvironmentKey {
    static let defaultValue = false
}

private struct ToolStatusKey: EnvironmentKey {
    static let defaultValue: @Sendable (String) -> BinaryLocator.Status = {
        BinaryLocator.status(of: $0)
    }
}

extension EnvironmentValues {
    /// Whether an agent terminal draws a placeholder instead of attaching.
    /// False everywhere but a story: there is no tmux server inside a
    /// gallery run, and a view that tried to attach to a session that does
    /// not exist would render the attach's own failure.
    var terminalStubbed: Bool {
        get { self[TerminalStubbedKey.self] }
        set { self[TerminalStubbedKey.self] = newValue }
    }

    /// How the onboarding checklist finds out about a binary. The real
    /// locator by default; a story pins the answers, since a pane that reads
    /// the machine renders differently on every machine and a reference
    /// nobody can compare against is no reference.
    var toolStatus: @Sendable (String) -> BinaryLocator.Status {
        get { self[ToolStatusKey.self] }
        set { self[ToolStatusKey.self] = newValue }
    }
}

/// What a story draws where the agent terminal goes: the session's own name,
/// said plainly on the terminal's own ground, so the region reads as the
/// terminal it stands for rather than as a pane that failed to load.
struct TerminalStubView: View {
    let session: String

    var body: some View {
        VStack(spacing: 8) {
            Image(systemName: "terminal")
                .font(.system(size: 28, weight: .regular))
                .ink(.secondary)

            Text(session)
                .font(Typo.mono(size: Typo.code, weight: .regular))
                .ink(.primary)

            Text("The agent's terminal draws here.")
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.tertiary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/// What a story draws to show a click-drag selection in the agent terminal:
/// a few fixed rows of mock output with a run of characters, spanning more
/// than one row, washed in the same `terminalSelection` colour
/// `TerminalTheme.apply` gives a live view's `selectedTextBackgroundColor` —
/// there is no tmux session in a gallery run for a real drag to select
/// against, so this stands in for one exactly as `TerminalStubView` stands in
/// for the terminal itself.
struct TerminalSelectionStubView: View {
    /// The rows drawn, and the selection over them: `git status`'s own
    /// output, dragged from partway through the branch line to partway
    /// through the line after it — a selection that spans a row boundary,
    /// which is the case worth a picture.
    private static let rows = [
        "$ git status",
        "On branch slice/copy-and-paste-in-the-embedded-terminal",
        "nothing to commit, working tree clean",
    ]
    private static let start = TerminalGridPosition(row: 1, col: 11)
    private static let end = TerminalGridPosition(row: 2, col: 17)

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            ForEach(Array(Self.rows.enumerated()), id: \.offset) { index, row in
                line(row, index: index)
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .background(DesignTokens.fill(.terminal))
    }

    @ViewBuilder
    private func line(_ row: String, index: Int) -> some View {
        let characters = Array(row)
        let inSelection = index >= Self.start.row && index <= Self.end.row
        let lowerBound = index == Self.start.row ? min(Self.start.col, characters.count) : 0
        let upperBound = index == Self.end.row ? min(Self.end.col, characters.count) : characters.count

        HStack(spacing: 0) {
            if inSelection {
                if lowerBound > 0 {
                    Text(String(characters[0..<lowerBound]))
                }
                if lowerBound < upperBound {
                    Text(String(characters[lowerBound..<upperBound]))
                        .background(DesignTokens.terminalSelectionWash())
                }
                if upperBound < characters.count {
                    Text(String(characters[upperBound...]))
                }
            } else {
                Text(row)
            }
            Spacer(minLength: 0)
        }
        .font(Typo.mono(size: Typo.code, weight: .regular))
        .foregroundStyle(DesignTokens.terminalText())
    }
}
