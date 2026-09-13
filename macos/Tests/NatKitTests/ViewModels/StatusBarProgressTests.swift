import XCTest
@testable import NatKit

final class StatusBarProgressTests: XCTestCase {
    private var testProject: Project!

    override func setUp() {
        super.setUp()
        testProject = Project(id: "proj-1", name: "Test", conventions: "")
    }

    func testBuildStatusBarProgress_empty() {
        let projectInfo = ProjectInfo(project: testProject, milestones: [], slices: [])
        let progress = buildStatusBarProgress(from: projectInfo)

        XCTAssertEqual(progress.done, 0)
        XCTAssertEqual(progress.total, 0)
        XCTAssertEqual(progress.doneStub, 0)
        XCTAssertTrue(progress.milestones.isEmpty)
        XCTAssertEqual(progress.countLabel, "0/0")
    }

    func testBuildStatusBarProgress_allDone() {
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
        let progress = buildStatusBarProgress(from: projectInfo)

        XCTAssertEqual(progress.doneStub, 2)
        XCTAssertEqual(progress.done, 2)
        XCTAssertEqual(progress.total, 2)
        XCTAssertTrue(progress.milestones.isEmpty)
        XCTAssertEqual(progress.doneTooltip, "Done — 2")
    }

    /// Done milestones fold into the stub whatever their plan order; the open
    /// ones follow in plan order, each keeping its own slice count.
    func testBuildStatusBarProgress_doneMilestonesFoldIntoTheStub() {
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
        let progress = buildStatusBarProgress(from: projectInfo)

        XCTAssertEqual(progress.doneStub, 5)
        XCTAssertEqual(progress.done, 5)
        XCTAssertEqual(progress.total, 14)
        XCTAssertEqual(progress.milestones.count, 2)

        XCTAssertEqual(progress.milestones[0].title, "Core")
        XCTAssertEqual(progress.milestones[0].total, 4)
        XCTAssertFalse(progress.milestones[0].started)

        XCTAssertEqual(progress.milestones[1].title, "Ship")
        XCTAssertEqual(progress.milestones[1].total, 5)
        XCTAssertFalse(progress.milestones[1].started)
    }

    /// A Done milestone with no slices still counts the minimum weight of 1
    /// into the stub, matching what it weighed on its own.
    func testBuildStatusBarProgress_emptyDoneMilestoneWeighsOne() {
        let milestones = [
            Milestone(id: "m-1", name: "Prep", order: 1, status: "Done"),
            Milestone(id: "m-2", name: "Core", order: 2, status: "Active")
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: [])
        let progress = buildStatusBarProgress(from: projectInfo)

        XCTAssertEqual(progress.doneStub, 1)
        XCTAssertEqual(progress.milestones.count, 1)
        XCTAssertEqual(progress.milestones[0].title, "Core")
    }

    /// A milestone with some but not all slices done draws as a started,
    /// proportional segment.
    func testBuildStatusBarProgress_startedMilestone() {
        let milestones = [
            Milestone(id: "m-1", name: "Foundations", order: 1, status: "Active")
        ]
        let slices = [
            Slice(id: "s-1", name: "S1", status: "Done", milestoneID: "m-1", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "s-2", name: "S2", status: "In progress", milestoneID: "m-1", assignee: "", pr: "", url: "", blocked: false, handedBack: false),
            Slice(id: "s-3", name: "S3", status: "Todo", milestoneID: "m-1", assignee: "", pr: "", url: "", blocked: false, handedBack: false)
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: slices)
        let progress = buildStatusBarProgress(from: projectInfo)

        XCTAssertEqual(progress.milestones.count, 1)
        let m = progress.milestones[0]
        XCTAssertEqual(m.title, "Foundations")
        XCTAssertEqual(m.done, 1)
        XCTAssertEqual(m.total, 3)
        XCTAssertTrue(m.started)
        XCTAssertEqual(m.fraction, 1.0 / 3.0, accuracy: 0.0001)
        XCTAssertEqual(m.tooltip, "Foundations — 1/3")
    }

    /// A milestone with nothing done yet collapses to an unstarted circle.
    func testBuildStatusBarProgress_unstartedMilestone() {
        let milestones = [
            Milestone(id: "m-1", name: "Ship", order: 1, status: "Queued")
        ]
        let slices = [
            Slice(id: "s-1", name: "S1", status: "Todo", milestoneID: "m-1", assignee: "", pr: "", url: "", blocked: false, handedBack: false)
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: slices)
        let progress = buildStatusBarProgress(from: projectInfo)

        XCTAssertEqual(progress.milestones.count, 1)
        let m = progress.milestones[0]
        XCTAssertFalse(m.started)
        XCTAssertEqual(m.fraction, 0.0)
        XCTAssertEqual(m.tooltip, "Ship — 0/1")
    }

    /// A Done slice counts as progress the moment its page says so — see
    /// `buildStatusBarProgress`'s own note on why nothing here asks whether a
    /// pull request is still open.
    func testBuildStatusBarProgress_doneIsReadStraightOffTheStatus() {
        let milestones = [
            Milestone(id: "m-1", name: "Setup", order: 1, status: "Done")
        ]
        let slices = [
            Slice(id: "s-1", name: "S1", status: "Done", milestoneID: "m-1",
                  assignee: "", pr: "https://github.com/o/r/pull/1", url: "", blocked: false, handedBack: false)
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: slices)
        let progress = buildStatusBarProgress(from: projectInfo)

        XCTAssertEqual(progress.doneStub, 1)
        XCTAssertTrue(progress.milestones.isEmpty)
    }

    func testBuildStatusBarProgress_emptyMilestone() {
        let milestones = [
            Milestone(id: "m-1", name: "Empty", order: 1, status: "Active")
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: [])
        let progress = buildStatusBarProgress(from: projectInfo)

        XCTAssertEqual(progress.milestones.count, 1)
        XCTAssertEqual(progress.milestones[0].weight, 1) // Minimum weight
        XCTAssertEqual(progress.milestones[0].fraction, 0.0)
        XCTAssertFalse(progress.milestones[0].started)
    }

    func testBuildStatusBarProgress_order() {
        let milestones = [
            Milestone(id: "m-1", name: "Third", order: 3, status: "Active"),
            Milestone(id: "m-2", name: "First", order: 1, status: "Active"),
            Milestone(id: "m-3", name: "Second", order: 2, status: "Active")
        ]

        let projectInfo = ProjectInfo(project: testProject, milestones: milestones, slices: [])
        let progress = buildStatusBarProgress(from: projectInfo)

        XCTAssertEqual(progress.milestones[0].title, "First")
        XCTAssertEqual(progress.milestones[1].title, "Second")
        XCTAssertEqual(progress.milestones[2].title, "Third")
    }

    func testMilestoneStatus_weightMinimum() {
        let status = MilestoneStatus(title: "Test", done: 0, total: 0, started: false)
        XCTAssertEqual(status.weight, 1)
    }

    func testMilestoneStatus_fractionZeroForEmptyMilestone() {
        let status = MilestoneStatus(title: "Test", done: 0, total: 0, started: false)
        XCTAssertEqual(status.fraction, 0.0)
    }
}
