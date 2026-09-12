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
/// `openPRSliceIDs` is the PR-readiness reading — the slices whose pull
/// request is positively read as open. A Done slice among them does not count
/// as progress yet: the board marks a slice Done as it opens the pull request,
/// and the work is not on main until that merges — the same rule that keeps
/// such a slice among the rail's review entries. With no reading taken the
/// set is empty and every Done slice counts, which is what every finished
/// project must go on reading as.
public func buildProgressSegments(
    from projectInfo: ProjectInfo,
    openPRSliceIDs: Set<String> = []
) -> [ProgressSegment] {
    let sortedMilestones = projectInfo.milestones.sorted { $0.order < $1.order }

    var doneTitles: [String] = []
    var doneWeight = 0
    var openSegments: [ProgressSegment] = []

    for milestone in sortedMilestones {
        let slices = projectInfo.slices.filter { $0.milestoneID == milestone.id }
        let doneCount = slices.filter { sliceWorkDone($0, openPRSliceIDs: openPRSliceIDs) }.count
        let totalCount = slices.count

        // A milestone Notion reads as Done still holds moving work while any
        // of its slices waits on a merge, so it stays an open segment until
        // the reading lets every slice count.
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
