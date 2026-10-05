import Foundation
import NatKit

// MARK: - Resumed work and what has changed since it was seen

extension Fixtures {
    /// The approved slice resumed, with the second hand-in of images on it
    /// (`visualChangesWithNews`) — a slice with Changes, Visual changes and
    /// PR all to carry the resumed notice and their badges.
    public static var resumedVisualsSliceDetails: [String: SliceDetail] {
        sliceDetails.merging([approveSliceID: approvedSliceDetail(
            last: TaskLogEvent(.resumed, note: resumedNote, at: now.addingTimeInterval(-25 * 60)),
            visuals: visualChangesWithNews)]) { _, new in new }
    }

    /// The pull request's head as the user last saw it, before the resumed
    /// agent pushed — so the green pull request's head reads moved.
    public static let seenPRHeadSHA = "9e8d7c6b5a41"

    /// What the user saw of the approved slice before it was resumed: its
    /// Changes without the branch's first file (New now) and with the second
    /// as it was before (Updated now), its first hand-in of images (the
    /// second hand-in's re-render Updated, its added render New), and its
    /// pull request at an older head (Updated). In memory, as every fixture
    /// memory is.
    public static func seenBeforeResume() -> SeenMemory {
        let memory = SeenMemory.inMemory()
        let files = buildDiffModel(from: sliceDiff, expandable: true).files
        var changes = Dictionary(files.map { ($0.path, $0.seenFingerprint) }, uniquingKeysWith: { first, _ in first })
        if let first = files.first { changes[first.path] = nil }
        if files.count > 1 { changes[files[1].path] = "before-the-resume" }
        memory.baseline(projectID: projectID, sliceID: approveSliceID, .changes, changes)
        memory.baseline(
            projectID: projectID, sliceID: approveSliceID, .visuals,
            Dictionary(visualChanges.map { ($0.name, $0.identity) }, uniquingKeysWith: { first, _ in first }))
        memory.baseline(projectID: projectID, sliceID: approveSliceID, .pr, [PRStore.seenHead: seenPRHeadSHA])
        return memory
    }
}
