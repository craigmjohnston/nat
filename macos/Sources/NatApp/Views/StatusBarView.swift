import SwiftUI
import NatKit

/// The window's full-width status bar: the plan's progress on the left, at
/// the sidebar's own width, and the live agent count on the right — split at
/// the same x-position as the sidebar/content divider above it, with a
/// hairline of its own carrying that line on down.
struct StatusBarView: View {
    @Bindable var appModel: AppModel
    let railWidth: CGFloat

    static let height: CGFloat = 32

    private var progress: StatusBarProgress {
        guard let projectInfo = appModel.projectStore?.state.projectInfo else {
            return StatusBarProgress(done: 0, total: 0, doneStub: 0, milestones: [])
        }
        return buildStatusBarProgress(from: projectInfo)
    }

    private var agentCount: Int {
        appModel.activityStore?.agents.count ?? 0
    }

    var body: some View {
        HStack(spacing: 0) {
            PlanProgressCell(progress: progress)
                .frame(width: railWidth)

            AgentCountCell(count: agentCount)
                .frame(maxWidth: .infinity)
        }
        .frame(height: Self.height)
        .surface(.header)
        .rule(.separator, edges: [.top], width: 0.5)
        // The sidebar/content divider carried down through the bar below it,
        // at the same x-position rather than the rail's own trailing rule,
        // which the rail draws only over its own height.
        .overlay(alignment: .leading) {
            Rectangle()
                .fill(DesignTokens.rule(.separator, on: .header))
                .frame(width: 0.5, height: Self.height)
                .offset(x: railWidth)
        }
    }
}

/// The left cell: the plan progress bar and its `done/total` count.
private struct PlanProgressCell: View {
    let progress: StatusBarProgress

    private static let horizontalPadding: CGFloat = 20

    var body: some View {
        HStack(spacing: 10) {
            PlanProgressBar(progress: progress)
            Text(progress.countLabel)
                .font(.system(size: Typo.caption, weight: .regular))
                .monospacedDigit()
                .ink(.tertiary)
        }
        .padding(.horizontal, Self.horizontalPadding)
    }
}

/// The bar itself: a fixed-width done stub, one proportional segment per
/// started milestone sharing what width is left, and a bare circle for every
/// milestone nothing has touched yet.
private struct PlanProgressBar: View {
    let progress: StatusBarProgress

    static let barHeight: CGFloat = 5
    static let doneStubWidth: CGFloat = 40
    static let circleDiameter: CGFloat = 5
    static let gap: CGFloat = 6

    private var started: [MilestoneStatus] { progress.milestones.filter(\.started) }
    private var unstarted: [MilestoneStatus] { progress.milestones.filter { !$0.started } }
    private var hasStub: Bool { progress.doneStub > 0 }

    var body: some View {
        GeometryReader { geometry in
            let elementCount = (hasStub ? 1 : 0) + progress.milestones.count
            let gapsWidth = Self.gap * CGFloat(max(0, elementCount - 1))
            let fixedWidth = (hasStub ? Self.doneStubWidth : 0)
                + CGFloat(unstarted.count) * Self.circleDiameter
            let totalWeight = max(1, started.reduce(0) { $0 + $1.weight })
            let flexibleWidth = max(0, geometry.size.width - fixedWidth - gapsWidth)

            HStack(spacing: Self.gap) {
                if hasStub {
                    doneStub
                }
                ForEach(Array(progress.milestones.enumerated()), id: \.offset) { _, milestone in
                    if milestone.started {
                        startedSegment(
                            milestone,
                            width: flexibleWidth * CGFloat(milestone.weight) / CGFloat(totalWeight)
                        )
                    } else {
                        unstartedCircle(milestone)
                    }
                }
            }
        }
        .frame(height: Self.barHeight)
    }

    private var doneStub: some View {
        RoundedRectangle(cornerRadius: Self.barHeight / 2)
            .fill(DesignTokens.accentMuted(on: .header))
            .frame(width: Self.doneStubWidth)
            .help(progress.doneTooltip)
    }

    private func startedSegment(_ milestone: MilestoneStatus, width: CGFloat) -> some View {
        ZStack(alignment: .leading) {
            RoundedRectangle(cornerRadius: Self.barHeight / 2)
                .fill(DesignTokens.rule(.border, on: .header))
            GeometryReader { geometry in
                RoundedRectangle(cornerRadius: Self.barHeight / 2)
                    .fill(DesignTokens.accent)
                    .frame(width: geometry.size.width * milestone.fraction)
            }
        }
        .frame(width: width)
        .help(milestone.tooltip)
    }

    private func unstartedCircle(_ milestone: MilestoneStatus) -> some View {
        Circle()
            .fill(DesignTokens.rule(.border, on: .header))
            .frame(width: Self.circleDiameter, height: Self.circleDiameter)
            .help(milestone.tooltip)
    }
}

/// The right cell: a quiet, live count of running agents. The far right is
/// left deliberately empty — a follow-up slice's Claude usage readout.
private struct AgentCountCell: View {
    let count: Int

    private static let horizontalPadding: CGFloat = 20

    private var label: String {
        "\(count) agent\(count == 1 ? "" : "s") running"
    }

    var body: some View {
        HStack {
            Text(label)
                .font(.system(size: Typo.caption, weight: .regular))
                .ink(.tertiary)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, Self.horizontalPadding)
    }
}

#Preview {
    let appModel = AppModel()
    StatusBarView(appModel: appModel, railWidth: 372)
        .frame(width: 1360)
}
