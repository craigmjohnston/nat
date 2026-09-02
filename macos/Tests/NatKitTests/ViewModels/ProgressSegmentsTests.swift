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

        XCTAssertEqual(segments.count, 2)
        XCTAssert(segments.allSatisfy { $0.isComplete })
        XCTAssert(segments.allSatisfy { $0.fraction == 1.0 })
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
