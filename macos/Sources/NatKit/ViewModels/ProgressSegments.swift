import Foundation

/// A single segment in the progress bar.
public struct ProgressSegment: Equatable {
    public let title: String
    public let weight: Int // Slice count for this milestone
    public let fraction: Double // 0.0 to 1.0
    public let isComplete: Bool

    public init(title: String, weight: Int, fraction: Double, isComplete: Bool) {
        self.title = title
        self.weight = max(1, weight) // Minimum weight of 1
        self.fraction = min(max(fraction, 0.0), 1.0) // Clamp to 0.0-1.0
        self.isComplete = isComplete
    }
}

/// Builds progress segments from a project's milestones.
///
/// One segment per milestone still open, in plan order — each weighted by its
/// slice count, which is what the bar divides its width by — with every Done
/// milestone folded into a single complete segment at the head of the list,
/// weighted by all their slices together: finished work reads as one solid
/// run growing from the left rather than stripes scattered through the plan.
/// The combined segment's title names the milestones it holds, since it is
/// what the tooltip shows.
///
/// Done-ness is Notion's own status, read directly: a slice counts as done
/// once its page says so, which is written only by a real event now — a
/// merge, or completing a slice with no pull request — never derived here.
public func buildProgressSegments(from projectInfo: ProjectInfo) -> [ProgressSegment] {
    let sortedMilestones = projectInfo.milestones.sorted { $0.order < $1.order }

    var doneTitles: [String] = []
    var doneWeight = 0
    var openSegments: [ProgressSegment] = []

    for milestone in sortedMilestones {
        let slices = projectInfo.slices.filter { $0.milestoneID == milestone.id }
        let doneCount = slices.filter { $0.status == "Done" }.count
        let totalCount = slices.count

        if milestone.status == "Done" && doneCount == totalCount {
            doneTitles.append(milestone.name)
            doneWeight += max(1, totalCount)
            continue
        }

        openSegments.append(ProgressSegment(
            title: milestone.name,
            weight: max(1, totalCount),
            fraction: totalCount == 0 ? 0.0 : Double(doneCount) / Double(totalCount),
            isComplete: false
        ))
    }

    guard !doneTitles.isEmpty else { return openSegments }
    let combined = ProgressSegment(
        title: doneTitles.joined(separator: ", "),
        weight: doneWeight,
        fraction: 1.0,
        isComplete: true
    )
    return [combined] + openSegments
}
