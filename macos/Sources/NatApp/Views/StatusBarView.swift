import SwiftUI
import NatKit

/// The window's status bar: the gnat mark and how many agents are running,
/// then each Claude usage window, set apart by faint dividers at the leading
/// edge, all quietly in the system sans at `xs`; at the trailing edge
/// (`trailing`), the selection's live agent's model, effort and context in
/// small mono (`AgentModelHeading`) — a slice's, a session's, the planning
/// agent's — or nothing. Where the selection sits is the titlebar band's
/// breadcrumb.
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
        .font(.system(size: GnatMetrics.xs))
        .monospacedDigit()
        .lineLimit(1)
        .padding(.horizontal, 10)
        // Centred in the band under the top rule, which draws over the bar.
        .frame(height: Self.height - 1)
        .padding(.top, 1)
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
