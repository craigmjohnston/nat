import Foundation

/// One milestone still open, drawn in the status bar's left cell — either a
/// proportional bar (`started`) or a collapsed circle (not `started`). A
/// milestone folded into `StatusBarProgress.doneStub` never appears here.
public struct MilestoneStatus: Equatable {
    public let title: String
    public let done: Int
    public let total: Int
    public let started: Bool

    public init(title: String, done: Int, total: Int, started: Bool) {
        self.title = title
        self.done = done
        self.total = total
        self.started = started
    }

    /// The share of the started segments' combined width this one gets —
    /// its own slice count, floored at 1 so an empty milestone still draws
    /// a sliver rather than vanishing.
    public var weight: Int { max(1, total) }

    /// How far the started segment's fill reaches, 0 for a milestone with no
    /// slices at all.
    public var fraction: Double { total == 0 ? 0.0 : Double(done) / Double(total) }

    /// What the segment's tooltip reads, e.g. "M33 — 4/15".
    public var tooltip: String { "\(title) — \(done)/\(total)" }
}

/// The status bar's left cell, built once from a project's plan: the overall
/// done/total count, the fixed-width stub standing in for every finished
/// milestone, and the milestones still open in plan order.
public struct StatusBarProgress: Equatable {
    /// Slices done across the whole project — folded into the stub or still
    /// counted on an open milestone, either way.
    public let done: Int
    /// Every slice in the project.
    public let total: Int
    /// All completed milestones' slices, collapsed into the one stub. Zero
    /// when nothing is finished yet.
    public let doneStub: Int
    /// The milestones still open, in plan order.
    public let milestones: [MilestoneStatus]

    public init(done: Int, total: Int, doneStub: Int, milestones: [MilestoneStatus]) {
        self.done = done
        self.total = total
        self.doneStub = doneStub
        self.milestones = milestones
    }

    /// What the done stub's tooltip reads, e.g. "Done — 175".
    public var doneTooltip: String { "Done — \(doneStub)" }

    /// The `done/total` count drawn beside the bar, e.g. "187/233".
    public var countLabel: String { "\(done)/\(total)" }
}

/// Builds the status bar's progress from a project's milestones.
///
/// Every Done milestone folds into `doneStub`, weighted by all their slices
/// together, the same rule the old progress strip folded them by — finished
/// work reads as one stub rather than stripes scattered through the plan.
/// Everything else keeps its own row: `started` once any of its slices are
/// Done, so it still draws a proportional bar even after its own status
/// column falls behind (a milestone is "Active" by hand, not by formula);
/// otherwise it collapses to a bare circle, since an untouched milestone has
/// no fraction worth a bar's width.
///
/// Done-ness is Notion's own status, read directly — see
/// `buildProgressSegments`'s note on the same rule, which this replaces.
public func buildStatusBarProgress(from projectInfo: ProjectInfo) -> StatusBarProgress {
    let sortedMilestones = projectInfo.milestones.sorted { $0.order < $1.order }

    var doneStub = 0
    var milestones: [MilestoneStatus] = []
    var totalDone = 0
    var totalAll = 0

    for milestone in sortedMilestones {
        let slices = projectInfo.slices.filter { $0.milestoneID == milestone.id }
        let doneCount = slices.filter { $0.status == "Done" }.count
        let totalCount = slices.count
        totalDone += doneCount
        totalAll += totalCount

        if milestone.status == "Done" && doneCount == totalCount {
            doneStub += max(1, totalCount)
            continue
        }

        milestones.append(MilestoneStatus(
            title: milestone.name,
            done: doneCount,
            total: totalCount,
            started: doneCount > 0
        ))
    }

    return StatusBarProgress(done: totalDone, total: totalAll, doneStub: doneStub, milestones: milestones)
}
