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

    private var usageDisplay: UsageDisplay {
        buildUsageDisplay(from: appModel.usageStore?.reading)
    }

    private var readout: AgentReadout? {
        buildAgentReadout(from: appModel.attachedAgent)
    }

    var body: some View {
        HStack(spacing: 0) {
            PlanProgressCell(progress: progress)
                .frame(width: railWidth)

            AgentCountCell(count: agentCount, usage: usageDisplay, readout: readout)
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

/// The left cell: the plan progress bar.
private struct PlanProgressCell: View {
    let progress: StatusBarProgress

    private static let horizontalPadding: CGFloat = 20

    var body: some View {
        PlanProgressBar(progress: progress)
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
    static let checkSize = CGSize(width: 11, height: 8)
    static let checkWidth: CGFloat = 2.5
    static let checkOutline: CGFloat = 4

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

    /// A bold, text-coloured checkmark over the stub, ringed by an outer
    /// stroke in the header's fill: the same path stroked wider underneath, so
    /// the ring sits outside the glyph's own edge rather than eating into it.
    private var doneStub: some View {
        RoundedRectangle(cornerRadius: Self.barHeight / 2)
            .fill(DesignTokens.accentMuted(on: .header))
            .frame(width: Self.doneStubWidth)
            .overlay {
                ZStack {
                    CheckmarkShape()
                        .stroke(
                            DesignTokens.fill(.header),
                            style: StrokeStyle(
                                lineWidth: Self.checkWidth + 2 * Self.checkOutline,
                                lineCap: .round, lineJoin: .round
                            )
                        )
                    CheckmarkShape()
                        .stroke(
                            DesignTokens.ink(.primary, on: .header),
                            style: StrokeStyle(lineWidth: Self.checkWidth, lineCap: .round, lineJoin: .round)
                        )
                }
                .frame(width: Self.checkSize.width, height: Self.checkSize.height)
            }
            .help(progress.doneTooltip)
    }

    private func startedSegment(_ milestone: MilestoneStatus, width: CGFloat) -> some View {
        RoundedRectangle(cornerRadius: Self.barHeight / 2)
            .fill(DesignTokens.progressTrack(on: .header))
            .frame(width: width)
            .overlay(alignment: .leading) {
                Rectangle()
                    .fill(DesignTokens.accent)
                    .frame(width: milestone.fillWidth(inTrack: width))
            }
            .clipShape(RoundedRectangle(cornerRadius: Self.barHeight / 2))
            .help(milestone.tooltip)
    }

    private func unstartedCircle(_ milestone: MilestoneStatus) -> some View {
        Circle()
            .fill(DesignTokens.progressTrack(on: .header))
            .frame(width: Self.circleDiameter, height: Self.circleDiameter)
            .help(milestone.tooltip)
    }
}

/// A checkmark path filling its rect, for stroking twice (see `doneStub`).
private struct CheckmarkShape: Shape {
    func path(in rect: CGRect) -> Path {
        var path = Path()
        path.move(to: CGPoint(x: rect.minX, y: rect.midY + rect.height * 0.1))
        path.addLine(to: CGPoint(x: rect.minX + rect.width * 0.36, y: rect.maxY))
        path.addLine(to: CGPoint(x: rect.maxX, y: rect.minY))
        return path
    }
}

/// The right cell: a quiet, live count of running agents, and — at the
/// bar's own far right edge — the Claude usage readout, as
/// `{usage} | N agents running`.
private struct AgentCountCell: View {
    let count: Int
    let usage: UsageDisplay
    let readout: AgentReadout?

    private static let horizontalPadding: CGFloat = 20
    private static let separatorGap: CGFloat = 10

    private var label: String {
        "\(count) agent\(count == 1 ? "" : "s") running"
    }

    var body: some View {
        HStack(spacing: Self.separatorGap) {
            if let readout {
                AgentReadoutView(readout: readout)
            }
            Spacer(minLength: 0)
            if !usage.isEmpty {
                UsageReadoutView(usage: usage)
                Text("|")
                    .font(.system(size: Typo.caption, weight: .regular))
                    .ink(.tertiary)
            }
            Text(label)
                .font(.system(size: Typo.caption, weight: .regular))
                .ink(.tertiary)
        }
        .padding(.horizontal, Self.horizontalPadding)
    }
}

/// The attached agent's readout, at the left of the content side: "Sonnet 5 ·
/// high · 42%", the context percent in the warning tint once it runs high.
private struct AgentReadoutView: View {
    let readout: AgentReadout
    @Environment(\.ground) private var ground

    var body: some View {
        HStack(spacing: 4) {
            if let label = readout.label {
                Text(label).ink(.tertiary)
            }
            if let context = readout.context {
                if readout.label != nil { Text("·").ink(.tertiary) }
                Text(context.text)
                    .foregroundStyle(
                        context.warning
                            ? DesignTokens.systemOrangeInk(on: ground)
                            : DesignTokens.ink(.tertiary, on: ground)
                    )
            }
        }
        .font(.system(size: Typo.caption, weight: .regular))
        .monospacedDigit()
    }
}

/// The Claude usage readout itself: the windows still worth showing, joined by `·`. Draws nothing at all when `usage` is
/// empty — the caller checks that, but the view is safe called on an empty
/// one regardless.
private struct UsageReadoutView: View {
    let usage: UsageDisplay

    private static let clauseGap: CGFloat = 4

    var body: some View {
        if !usage.isEmpty {
            HStack(spacing: Self.clauseGap) {
                ForEach(Array(usage.windows.enumerated()), id: \.offset) { index, window in
                    if index > 0 {
                        Text("·").ink(.tertiary)
                    }
                    UsageClauseText(window: window)
                }
            }
            .font(.system(size: Typo.caption, weight: .regular))
            .monospacedDigit()
        }
    }
}

/// One window's clause, in the warning tint (system orange) once it has
/// crossed the threshold, tertiary otherwise — the tint the plain `.ink`
/// vocabulary has no role for, so this reads the ground directly the same
/// way `InkModifier` does.
private struct UsageClauseText: View {
    let window: UsageWindowDisplay
    @Environment(\.ground) private var ground

    var body: some View {
        Text(window.text)
            .foregroundStyle(
                window.warning ? DesignTokens.systemOrangeInk(on: ground) : DesignTokens.ink(.tertiary, on: ground)
            )
    }
}

#Preview {
    let appModel = AppModel()
    StatusBarView(appModel: appModel, railWidth: 372)
        .frame(width: 1360)
}
