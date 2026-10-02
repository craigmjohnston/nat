import XCTest
@testable import NatKit

final class CrumbTreeTests: XCTestCase {
    private func row(_ id: String, project: String = "p") -> SidebarSliceRow {
        SidebarSliceRow(sliceID: id, projectID: project, title: id, state: .todo, live: false)
    }

    private var model: SidebarModel {
        SidebarModel(
            active: [],
            projects: [
                SidebarProject(
                    id: "p", name: "P", kind: .project, status: .loaded,
                    milestones: [SidebarMilestone(name: "M1", done: 0, total: 1, slices: [row("a")])],
                    doneMilestones: [SidebarMilestone(name: "M0", done: 1, total: 1, slices: [row("z")])],
                    needsYou: 0),
                SidebarProject(id: "u", name: "Untitled", kind: .untitled, status: .none, milestones: [], needsYou: 0),
            ],
            scratch: SidebarProject(
                id: "s", name: "Scratch", kind: .scratch, status: .loaded,
                milestones: [SidebarMilestone(name: "Try", done: 0, total: 1, slices: [row("t", project: "s")])],
                needsYou: 0, loose: [row("l", project: "s")]))
    }

    func testTheProjectsColumnIsEveryProjectButAnUntitledOneScratchLast() {
        XCTAssertEqual(CrumbTree(model: model, projectID: "p").projects.map(\.id), ["p", "s"])
    }

    func testAProjectCrumbOpensItsMilestonesWithNoThirdColumn() {
        let tree = CrumbTree(model: model, projectID: "p")
        XCTAssertEqual(tree.entries.map(\.id), ["m:M1", "m:M0"])
        XCTAssertNil(tree.slices)
    }

    func testAMilestoneCrumbOpensItsSlicesToo() {
        let tree = CrumbTree(model: model, projectID: "p", milestone: "M0")
        XCTAssertEqual(tree.slices?.map(\.sliceID), ["z"])
    }

    func testScratchsLooseSlicesLeadItsColumn() {
        XCTAssertEqual(CrumbTree(model: model, projectID: "s").entries.map(\.id), ["s:l", "m:Try"])
    }

    func testOpeningAnotherProjectClosesTheMilestone() {
        var tree = CrumbTree(model: model, projectID: "p", milestone: "M1")
        tree.open(project: "p")
        XCTAssertEqual(tree.milestone, "M1", "the same project again changes nothing")
        tree.open(project: "s")
        XCTAssertEqual(tree.projectID, "s")
        XCTAssertNil(tree.milestone)
    }

    func testAnUnknownProjectOrMilestoneIsEmpty() {
        XCTAssertEqual(CrumbTree(model: model, projectID: "gone").entries, [])
        XCTAssertEqual(CrumbTree(model: model, projectID: "gone", milestone: "M1").slices, nil)
        XCTAssertEqual(CrumbTree(model: model, projectID: "p", milestone: "gone").slices, [])
    }
}
