import SwiftUI
import NatKit

/// The window's status bar, the design's own line: the gnat mark and how
/// many agents are running on the left, and the Claude usage windows at the
/// far right — all in the mono `xs`, quietly. The attached agent's model,
/// effort and context are the terminal's own heading (`AgentModelHeading`).
struct StatusBarView: View {
    @Bindable var appModel: AppModel

    static let height: CGFloat = GnatMetrics.statusBarHeight

    private var agentCount: Int {
        appModel.activityStore?.agents.count ?? 0
    }

    var body: some View {
        HStack(spacing: 16) {
            GnatMark(color: DesignTokens.accent)
                .frame(width: 14, height: 14)
            Text("\(agentCount) agent\(agentCount == 1 ? "" : "s") running")
                .ink(.secondary)
            Spacer(minLength: 0)
            UsageReadoutView(usage: buildUsageDisplay(from: appModel.usageStore?.reading))
        }
        .monoXS()
        .monospacedDigit()
        .lineLimit(1)
        .padding(.horizontal, 10)
        .frame(height: Self.height)
        .surface(.chrome)
        .rule(.separator, edges: [.top], width: 1)
    }
}

/// The Claude usage readout: each window still worth showing as its own
/// clause, the way the design sets them apart. Draws nothing for an empty
/// reading.
private struct UsageReadoutView: View {
    let usage: UsageDisplay
    @Environment(\.ground) private var ground

    var body: some View {
        HStack(spacing: 16) {
            ForEach(Array(usage.windows.enumerated()), id: \.offset) { _, window in
                Text(window.text)
                    .foregroundStyle(window.warning ? DesignTokens.hotInk(on: ground) : DesignTokens.ink(.secondary, on: ground))
            }
        }
    }
}

#Preview {
    StatusBarView(appModel: AppModel())
        .frame(width: 1320)
}
