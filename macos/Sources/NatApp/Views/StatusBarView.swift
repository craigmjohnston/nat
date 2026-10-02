import SwiftUI
import NatKit

/// The window's status bar: the gnat mark and how many agents are running,
/// then each Claude usage window, set apart by faint dividers at the leading
/// edge; where the selection sits (`trailing`, the shell's breadcrumb) at the
/// trailing edge — all quietly, in the mono `xs`. The attached agent's
/// model, effort and context are the terminal's own heading
/// (`AgentModelHeading`).
struct StatusBarView<Trailing: View>: View {
    @Bindable var appModel: AppModel
    @ViewBuilder var trailing: () -> Trailing

    static var height: CGFloat { GnatMetrics.statusBarHeight }

    private var agentCount: Int {
        appModel.activityStore?.agents.count ?? 0
    }

    var body: some View {
        let usage = buildUsageDisplay(from: appModel.usageStore?.reading)
        HStack(spacing: 10) {
            HStack(spacing: 12) {
                GnatMark(color: DesignTokens.mark)
                    .frame(width: 14, height: 14)
                Text("\(agentCount) agent\(agentCount == 1 ? "" : "s")")
                    .ink(.secondary)
            }
            ForEach(Array(usage.windows.enumerated()), id: \.offset) { _, window in
                StatusBarDivider()
                UsageWindowText(window: window)
            }
            Spacer(minLength: 16)
            trailing()
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

extension StatusBarView where Trailing == EmptyView {
    init(appModel: AppModel) {
        self.init(appModel: appModel, trailing: { EmptyView() })
    }
}

/// The faint upright line between the status bar's clauses.
private struct StatusBarDivider: View {
    var body: some View {
        DesignTokens.rule(.separator, on: .chrome)
            .frame(width: 1, height: 12)
    }
}

/// One Claude usage window's clause, in the warning tint once it runs high.
private struct UsageWindowText: View {
    let window: UsageWindowDisplay
    @Environment(\.ground) private var ground

    var body: some View {
        Text(window.text)
            .foregroundStyle(window.warning ? DesignTokens.hotInk(on: ground) : DesignTokens.ink(.secondary, on: ground))
    }
}

#Preview {
    StatusBarView(appModel: AppModel())
        .frame(width: 1320)
}
