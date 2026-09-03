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
/// Returns one segment per milestone (including done ones), in plan order.
public func buildProgressSegments(from projectInfo: ProjectInfo) -> [ProgressSegment] {
    let sortedMilestones = projectInfo.milestones.sorted { $0.order < $1.order }

    return sortedMilestones.map { milestone in
        let milestoneDone = milestone.status == "Done"
        let slices = projectInfo.slices.filter { $0.milestoneID == milestone.id }
        let doneCount = slices.filter { $0.status == "Done" }.count
        let totalCount = slices.count

        let fraction: Double
        if totalCount == 0 {
            fraction = milestoneDone ? 1.0 : 0.0
        } else {
            fraction = Double(doneCount) / Double(totalCount)
        }

        return ProgressSegment(
            title: milestone.name,
            weight: max(1, totalCount),
            fraction: fraction,
            isComplete: milestoneDone
        )
    }
}
