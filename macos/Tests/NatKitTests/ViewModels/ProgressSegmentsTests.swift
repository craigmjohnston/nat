import XCTest
@testable import NatKit

final class ProgressSegmentsTests: XCTestCase {
    private var testProject: Project!

    override func setUp() {
        super.setUp()
        testProject = Project(id: "proj-1", name: "Test", conventions: "")
    }

    func testBuildProgressSegments_empty() {
        let projectInfo = ProjectInfo(project: testProject, milestones: [], slices: [])
        let segments = buildProgressSegments(from: projectInfo)

        XCTAssertTrue(segments.isEmpty)
    }

    func testBuildProgressSegments_allDone() {
        let milestones = [
            Milestone(id: "m-1", name: "Setup", order: 1, status: "Done"),
            Milestone(id: "m-2", name: "Core", order: 2, status: "Done")
        ]
        let slices = [
            Slice(
                id: "s-1", name: "S1", status: "Done", milestoneID: "m-1",
                assignee: "", pr: "", url: "", blocked: false, handedBack: false
            ),
            Slice(
                id: "s-2", name: "S2", status: "Done", milestoneID: "m-2",
                assignee: "", pr: "", url: "", blocked: false, handedBack: false
            )
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: slices)
        let segments = buildProgressSegments(from: projectInfo)

        // Done milestones fold into one combined segment.
        XCTAssertEqual(segments.count, 1)
        XCTAssertEqual(segments[0].title, "Setup, Core")
        XCTAssertEqual(segments[0].weight, 2)
        XCTAssertEqual(segments[0].fraction, 1.0)
        XCTAssertTrue(segments[0].isComplete)
    }

    /// Done milestones fold left into one combined segment whatever their
    /// plan order, weighted by all their slices together; the open ones
    /// follow in plan order, each weighted by its own slice count.
    func testBuildProgressSegments_doneMilestonesCombineOnTheLeft() {
        let milestones = [
            Milestone(id: "m-1", name: "Setup", order: 1, status: "Done"),
            Milestone(id: "m-2", name: "Core", order: 2, status: "Active"),
            Milestone(id: "m-3", name: "Polish", order: 3, status: "Done"),
            Milestone(id: "m-4", name: "Ship", order: 4, status: "Queued")
        ]
        var slices: [Slice] = []
        func addSlices(_ count: Int, milestoneID: String, status: String) {
            for n in 0..<count {
                slices.append(Slice(
                    id: "\(milestoneID)-s\(n)", name: "S", status: status, milestoneID: milestoneID,
                    assignee: "", pr: "", url: "", blocked: false, handedBack: false
                ))
            }
        }
        addSlices(3, milestoneID: "m-1", status: "Done")
        addSlices(4, milestoneID: "m-2", status: "Todo")
        addSlices(2, milestoneID: "m-3", status: "Done")
        addSlices(5, milestoneID: "m-4", status: "Todo")

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: slices)
        let segments = buildProgressSegments(from: projectInfo)

        XCTAssertEqual(segments.count, 3)

        XCTAssertEqual(segments[0].title, "Setup, Polish")
        XCTAssertEqual(segments[0].weight, 5)
        XCTAssertEqual(segments[0].fraction, 1.0)
        XCTAssertTrue(segments[0].isComplete)

        XCTAssertEqual(segments[1].title, "Core")
        XCTAssertEqual(segments[1].weight, 4)
        XCTAssertFalse(segments[1].isComplete)

        XCTAssertEqual(segments[2].title, "Ship")
        XCTAssertEqual(segments[2].weight, 5)
        XCTAssertFalse(segments[2].isComplete)
    }

    /// A Done milestone with no slices still counts the minimum weight of 1
    /// into the combined segment, matching what it weighed on its own.
    func testBuildProgressSegments_emptyDoneMilestoneWeighsOne() {
        let milestones = [
            Milestone(id: "m-1", name: "Prep", order: 1, status: "Done"),
            Milestone(id: "m-2", name: "Core", order: 2, status: "Active")
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: [])
        let segments = buildProgressSegments(from: projectInfo)

        XCTAssertEqual(segments.count, 2)
        XCTAssertEqual(segments[0].title, "Prep")
        XCTAssertEqual(segments[0].weight, 1)
        XCTAssertTrue(segments[0].isComplete)
    }

    func testBuildProgressSegments_partialProgress() {
        let milestones = [
            Milestone(id: "m-1", name: "Foundations", order: 1, status: "Active")
        ]
        let slices = [
            Slice(id: "s-1", name: "S1", status: "Done", milestoneID: "m-1", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "s-2", name: "S2", status: "In progress", milestoneID: "m-1", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "s-3", name: "S3", status: "Todo", milestoneID: "m-1", assignee: "", pr: "", url: "", blocked: false, handedBack: false)
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: slices)
        let segments = buildProgressSegments(from: projectInfo)

        XCTAssertEqual(segments.count, 1)
        XCTAssertEqual(segments[0].title, "Foundations")
        XCTAssertEqual(segments[0].weight, 3)
        XCTAssertEqual(segments[0].fraction, 1.0 / 3.0)
        XCTAssertFalse(segments[0].isComplete)
    }

    /// A Done slice whose pull request is still open is not progress yet:
    /// the work is not on main until the merge, so it neither fills its
    /// milestone's fraction nor lets the milestone fold into the combined
    /// Done run.
    func testBuildProgressSegments_openPRHoldsADoneSliceBack() {
        let milestones = [
            Milestone(id: "m-1", name: "Setup", order: 1, status: "Done"),
            Milestone(id: "m-2", name: "Core", order: 2, status: "Active")
        ]
        let slices = [
            Slice(id: "s-1", name: "S1", status: "Done", milestoneID: "m-1",
                  assignee: "", pr: "https://github.com/o/r/pull/1", url: "", blocked: false, handedBack: false),
            Slice(id: "s-2", name: "S2", status: "Done", milestoneID: "m-1",
                  assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "s-3", name: "S3", status: "Todo", milestoneID: "m-2",
                  assignee: "", pr: "", url: "", blocked: false, handedBack: false)
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: slices)
        let segments = buildProgressSegments(from: projectInfo, openPRSliceIDs: ["s-1"])

        // Nothing folds: the Done milestone still has a merge outstanding.
        XCTAssertEqual(segments.count, 2)
        XCTAssertEqual(segments[0].title, "Setup")
        XCTAssertFalse(segments[0].isComplete)
        XCTAssertEqual(segments[0].fraction, 0.5)
        XCTAssertEqual(segments[1].title, "Core")
    }

    /// The same plan with no reading taken (or the merge landed) folds as it
    /// always did — an empty set changes nothing.
    func testBuildProgressSegments_noReadingCountsEveryDoneSlice() {
        let milestones = [
            Milestone(id: "m-1", name: "Setup", order: 1, status: "Done")
        ]
        let slices = [
            Slice(id: "s-1", name: "S1", status: "Done", milestoneID: "m-1",
                  assignee: "", pr: "https://github.com/o/r/pull/1", url: "", blocked: false, handedBack: false)
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: slices)
        let segments = buildProgressSegments(from: projectInfo)

        XCTAssertEqual(segments.count, 1)
        XCTAssertTrue(segments[0].isComplete)
    }

    func testBuildProgressSegments_emptyMilestone() {
        let milestones = [
            Milestone(id: "m-1", name: "Empty", order: 1, status: "Active")
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: [])
        let segments = buildProgressSegments(from: projectInfo)

        XCTAssertEqual(segments.count, 1)
        XCTAssertEqual(segments[0].weight, 1) // Minimum weight
        XCTAssertEqual(segments[0].fraction, 0.0)
    }

    func testBuildProgressSegments_order() {
        let milestones = [
            Milestone(id: "m-1", name: "Third", order: 3, status: "Active"),
            Milestone(id: "m-2", name: "First", order: 1, status: "Active"),
            Milestone(id: "m-3", name: "Second", order: 2, status: "Active")
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: [])
        let segments = buildProgressSegments(from: projectInfo)

        XCTAssertEqual(segments[0].title, "First")
        XCTAssertEqual(segments[1].title, "Second")
        XCTAssertEqual(segments[2].title, "Third")
    }

    func testProgressSegment_fractionClamped() {
        let segment1 = ProgressSegment(title: "Test", weight: 1, fraction: 1.5, isComplete: false)
        XCTAssertEqual(segment1.fraction, 1.0)

        let segment2 = ProgressSegment(title: "Test", weight: 1, fraction: -0.5, isComplete: false)
        XCTAssertEqual(segment2.fraction, 0.0)

        let segment3 = ProgressSegment(title: "Test", weight: 1, fraction: 0.5, isComplete: false)
        XCTAssertEqual(segment3.fraction, 0.5)
    }

    func testProgressSegment_weightMinimum() {
        let segment = ProgressSegment(title: "Test", weight: 0, fraction: 0, isComplete: false)
        XCTAssertEqual(segment.weight, 1)
    }
}
