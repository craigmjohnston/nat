import AppKit
import SwiftUI
import NatKit

/// Settings ▸ GitHub's artifact storage: the bar, the total against the
/// allowance and the days left, then the legend — a row per project and one
/// for other repositories, each with its figure. `StorageUsageModel` works
/// all of it out; this only draws it, or the wait, or nat's refusal.
struct StorageUsageSection: View {
    let model: StorageUsageModel

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            switch model.state {
            case .loading:
                Label("Reading GitHub\u{2019}s billing report\u{2026}", systemImage: "hourglass")
                    .ink(.secondary)
            case .failed(let message):
                Label(message, systemImage: "exclamationmark.triangle.fill")
                    .ink(.danger)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
            case .loaded(let usage):
                loaded(usage)
            case .needsScope(let command):
                needsScope(command)
            }
            refreshRow
        }
    }

    private func loaded(_ usage: StorageUsage) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline) {
                Text(StorageUsageModel.summary(usage))
                    .font(.body.monospacedDigit())
                    .ink(StorageUsageModel.isOver(usage) ? .warning : .primary)
                Spacer(minLength: 12)
                Text(StorageUsageModel.daysLeft(usage))
                    .ink(.secondary)
            }
            StorageBar(segments: StorageUsageModel.segments(usage))
                .frame(height: 12)
            VStack(alignment: .leading, spacing: 6) {
                ForEach(StorageUsageModel.legend(usage)) { row in
                    StorageLegendRow(row: row)
                }
            }
            .padding(.top, 4)
        }
    }

    /// gh can't read the billing report yet: what it would take, the command
    /// to copy, and that nothing else needs it — a choice, never an error.
    private func needsScope(_ command: String) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("GitHub only shares this month\u{2019}s storage with gh once gh has the \u{201C}user\u{201D} "
                + "permission, which also lets it read your plan and billing. To give it, run this in "
                + "Terminal and sign in when GitHub asks, then check again.")
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 8) {
                Text(command)
                    .font(Typo.mono(size: Typo.input))
                    .textSelection(.enabled)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 4)
                    .wash(.chip, tone: .neutral)
                    .clipShape(RoundedRectangle(cornerRadius: 4))
                Button("Copy") {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(command, forType: .string)
                }
            }
            Text("Nothing else in gnat needs this permission. Without it, this section stays as it is.")
                .ink(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private var refreshRow: some View {
        HStack(spacing: 8) {
            Button(model.state.isNeedsScope ? "Check Again" : "Refresh") { Task { await model.refresh() } }
                .disabled(model.isReading)
            if model.isReading {
                ProgressView().controlSize(.small)
            }
        }
    }
}

/// The bar: a track the allowance's width, each segment its share of it.
private struct StorageBar: View {
    let segments: [StorageUsageModel.Segment]

    var body: some View {
        GeometryReader { proxy in
            ZStack(alignment: .leading) {
                Capsule().fill(DesignTokens.labelQuaternary)
                HStack(spacing: 1) {
                    ForEach(segments) { segment in
                        Rectangle()
                            .fill(StorageSwatch.fill(color: segment.color, isOther: segment.isOther))
                            .frame(width: max(1, proxy.size.width * segment.fraction - 1))
                            .help(segment.name)
                    }
                }
                .clipShape(Capsule())
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Artifact storage by project")
    }
}

/// One legend line: the swatch, the name (the repositories its tooltip),
/// the figure at the trailing edge.
private struct StorageLegendRow: View {
    let row: StorageUsageModel.LegendRow

    var body: some View {
        HStack(spacing: 8) {
            RoundedRectangle(cornerRadius: 2.5)
                .fill(StorageSwatch.fill(color: row.color, isOther: row.isOther))
                .frame(width: 10, height: 10)
            Text(row.name)
                .lineLimit(1)
                .truncationMode(.tail)
                .help(row.repos.isEmpty ? "No GitHub repository" : row.repos.joined(separator: "\n"))
            Spacer(minLength: 12)
            Text(row.figure)
                .font(.body.monospacedDigit())
                .ink(.secondary)
        }
        .frame(maxWidth: 420)
    }
}

/// What a segment and its swatch are filled with: the project's colour, a
/// quiet grey for a project that takes none, and the system grey for other
/// repositories.
private enum StorageSwatch {
    static func fill(color: ProjectColor?, isOther: Bool) -> Color {
        if isOther { return DesignTokens.systemGray }
        if let color { return DesignTokens.projectInk(color, on: .window) }
        return DesignTokens.labelTertiary
    }
}
