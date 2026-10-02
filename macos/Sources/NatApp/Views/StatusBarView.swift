import SwiftUI
import NatKit

/// The window's status bar, the design's own line: the gnat mark, how many
/// agents are running and the attached agent's model, effort and context on
/// the left, and the Claude usage windows at the far right — all in the mono
/// `xs`, quietly.
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
            if let readout = buildAgentReadout(from: appModel.attachedAgent) {
                AgentReadoutView(readout: readout)
            }
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

/// The attached agent's readout: "Opus 5.5 · medium · ctx 37%", the context
/// in the warning tint once it runs high.
private struct AgentReadoutView: View {
    let readout: AgentReadout
    @Environment(\.ground) private var ground

    var body: some View {
        HStack(spacing: 4) {
            if let label = readout.label {
                Text(label).ink(.secondary)
            }
            if let context = readout.context {
                if readout.label != nil { Text("·").ink(.secondary) }
                Text(context.text)
                    .foregroundStyle(context.warning ? DesignTokens.hotInk(on: ground) : DesignTokens.ink(.secondary, on: ground))
            }
        }
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
