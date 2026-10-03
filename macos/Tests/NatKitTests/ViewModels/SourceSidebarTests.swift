import SwiftUI
import XCTest
@testable import NatKit
@testable import NatFixtures

/// A source project as the sidebar, the breadcrumb picker, the titlebar and
/// the container navigator read it — all over the fixture Work project.
final class SourceSidebarTests: XCTestCase {
    private func input(_ plan: ProjectInfo? = Fixtures.sourceProjectInfo(), isSource: Bool = false) -> SidebarProjectInput {
        SidebarProjectInput(id: Fixtures.sourceProjectID, name: "Work", plan: plan, isSource: isSource)
    }

    private func model(
        _ inputs: [SidebarProjectInput]? = nil, agents: [String: AgentActivity] = [:]
    ) -> SidebarModel {
        buildSidebarModel(
            projects: inputs ?? [
                SidebarProjectInput(id: Fixtures.projectID, name: "notion-agent-tracker", plan: Fixtures.projectInfo),
                input(),
            ],
            liveAgents: agents)
    }

    // MARK: - The fold

    func testASourceProjectIsItsOwnFoldNotAProject() throws {
        let model = model()
        XCTAssertEqual(model.projects.map(\.id), [Fixtures.projectID])
        XCTAssertEqual(model.sources.map(\.id), [Fixtures.sourceProjectID])
        let source = try XCTUnwrap(model.sources[0].source)
        XCTAssertEqual(source.title, "Demo source")
        XCTAssertEqual(source.tag, "DM")
        XCTAssertEqual(source.icon, SourceIcon(symbol: "rectangle.on.rectangle.angled"))
        XCTAssertEqual(source.containerNoun, "card")
        XCTAssertEqual(source.menu.map(\.id), ["refresh", "new-segment", "filter"])
        XCTAssertTrue(model.sources[0].milestones.isEmpty)
    }

    func testConfigFilesAProjectUnderItsFoldBeforeItsPlanLands() {
        let model = model([input(nil, isSource: true)])
        XCTAssertEqual(model.sources.map(\.id), [Fixtures.sourceProjectID])
        XCTAssertEqual(model.sources[0].status, .loading)
        XCTAssertNil(model.sources[0].source)
        XCTAssertEqual(self.model([input(nil)]).projects.map(\.id), [Fixtures.sourceProjectID])
    }

    func testGroupsKeepThePluginsShapeAndCounts() throws {
        let source = try XCTUnwrap(model().sources[0].source)
        XCTAssertEqual(source.groups.map(\.id), ["doing", "ready/mine", "ready/board", "done"])
        XCTAssertEqual(source.groups.map(\.count), [2, 1, 2, 4])
        XCTAssertEqual(source.groups.map(\.lazy), [false, false, false, true])
        XCTAssertEqual(source.groups[1].menu.map(\.id), ["rename", "filter", "remove"])
        XCTAssertEqual(source.groups[0].containers.map(\.id), [Fixtures.sourceCardID, Fixtures.sourceSecondCardID])
        XCTAssertEqual(source.groups[3].containers, [], "a lazy group lists nothing until expanded")
    }

    func testAContainersTasksAreThePlansSlicesFiledUnderItInPlanOrder() throws {
        let source = try XCTUnwrap(model().sources[0].source)
        let card = try XCTUnwrap(source.container(withID: Fixtures.sourceCardID))
        XCTAssertEqual(card.tasks.map(\.sliceID), [
            Fixtures.sourceTodoTaskID, Fixtures.sourceWorkingTaskID, Fixtures.sourceReviewTaskID,
        ])
        XCTAssertEqual(card.tasks.map(\.state), [.todo, .working, .pr])
        XCTAssertEqual(card.needsYou, 1)
        XCTAssertEqual(card.badges.map(\.text), ["NA"])
        XCTAssertEqual(card.meta, "3")
        XCTAssertEqual(source.container(withID: Fixtures.sourceMineCardID)?.tasks, [])
        XCTAssertNil(source.container(withID: "nope"))
    }

    func testAContainerInTwoGroupsIsOneContainer() throws {
        let plan = ProjectInfo(
            project: Fixtures.sourceProject, milestones: Fixtures.sourceMilestones,
            slices: Fixtures.sourceTasks + [Slice(
                id: "t-mine", name: "Columns", status: "Todo", milestoneID: Fixtures.sourceMineCardID,
                assignee: "", pr: "", url: "", blocked: false, handedBack: false)],
            source: Fixtures.sourceInfo())
        let source = try XCTUnwrap(model([input(plan)]).sources[0].source)
        let mine = source.groups[1].containers[0]
        let board = source.groups[2].containers[1]
        XCTAssertEqual(mine.id, Fixtures.sourceMineCardID)
        XCTAssertEqual(mine, board)
        XCTAssertEqual(mine.tasks.map(\.sliceID), ["t-mine"])
    }

    func testNeedsYouRollsUpToTheFoldAndActiveRowsCarryTheSourceTag() {
        let model = model(agents: [Fixtures.sourceWorkingTaskID: .waiting])
        XCTAssertEqual(model.sources[0].needsYou, 2, "the PR open and the waiting agent")
        let rows = model.active.filter { $0.projectID == Fixtures.sourceProjectID }
        XCTAssertEqual(Set(rows.map(\.targetID)), [Fixtures.sourceWorkingTaskID, Fixtures.sourceReviewTaskID])
        XCTAssertEqual(Set(rows.map(\.projectTag)), ["DM"])
        XCTAssertEqual(sidebarTags([input()])[Fixtures.sourceProjectID], "DM")
        // A plugin that gave no tag leaves the project's own.
        XCTAssertEqual(sidebarTags([input(nil)])[Fixtures.sourceProjectID], "WOR")
    }

    func testHideDoneDropsDoneTasksUnderContainers() throws {
        let done = Fixtures.sourceTasks.map { slice in
            slice.id == Fixtures.sourceTodoTaskID
                ? Slice(id: slice.id, name: slice.name, status: "Done", milestoneID: slice.milestoneID,
                        assignee: "", pr: "", url: "", blocked: false, handedBack: false)
                : slice
        }
        let plan = ProjectInfo(
            project: Fixtures.sourceProject, milestones: Fixtures.sourceMilestones, slices: done,
            source: Fixtures.sourceInfo())
        let project = model([input(plan)]).sources[0]
        XCTAssertEqual(project.source?.container(withID: Fixtures.sourceCardID)?.tasks.last?.state, .done)
        let hidden = try XCTUnwrap(project.hidingDone().source)
        XCTAssertEqual(hidden.container(withID: Fixtures.sourceCardID)?.tasks.map(\.sliceID), [
            Fixtures.sourceWorkingTaskID, Fixtures.sourceReviewTaskID,
        ])
        XCTAssertEqual(hidden.groups.count, 4, "the tree itself is kept")
    }

    func testAFailedPluginDrawsItsErrorOverTheUnlistedGroupWithDefaults() throws {
        let plan = ProjectInfo(
            project: Fixtures.sourceProject, milestones: Fixtures.sourceMilestones, slices: Fixtures.sourceTasks,
            source: Fixtures.sourceInfoFailed)
        let source = try XCTUnwrap(model([input(plan)]).sources[0].source)
        XCTAssertEqual(source.error, Fixtures.sourceInfoFailed.error)
        XCTAssertEqual(source.title, "demo")
        XCTAssertEqual(source.icon.symbol, SourceIcon.fallbackSymbol)
        XCTAssertEqual(source.groups.map(\.id), [SourceGroup.unlistedID])
        XCTAssertEqual(source.groups[0].containers.first?.tasks.count, 3)
    }

    func testAGrandchildGroupsContainersAreItsParentsAndDefaultNouns() throws {
        let info = SourceInfo(
            name: "x", title: "", tag: "", iconSymbol: "", containerNoun: "", taskNoun: "",
            groups: [SourceGroup(id: "a", label: "A", children: [
                SourceGroup(id: "b", label: "B", children: [
                    SourceGroup(id: "c", label: "C", containers: [SourceContainer(id: "k", title: "K")]),
                ]),
            ])])
        let plan = ProjectInfo(project: Fixtures.sourceProject, milestones: [], slices: [], source: info)
        let source = try XCTUnwrap(model([input(plan)]).sources[0].source)
        XCTAssertEqual(source.groups[0].children[0].containers.map(\.id), ["k"])
        XCTAssertEqual(source.containerNoun, "container")
        XCTAssertEqual(source.taskNoun, "task")
        XCTAssertEqual(source.containerGroups.map(\.group.label), ["A", "A \u{00B7} B"])
    }

    // MARK: - The breadcrumb picker

    func testTheCrumbTreeOpensASourceProjectOnItsGroupsContainersAndTasks() throws {
        let model = model()
        var tree = CrumbTree(model: model, projectID: Fixtures.sourceProjectID)
        XCTAssertEqual(tree.projects.map(\.id), [Fixtures.projectID, Fixtures.sourceProjectID])
        XCTAssertEqual(tree.entries.map(\.id), ["g:doing", "g:ready/mine", "g:ready/board", "g:done"])
        XCTAssertNil(tree.containers)
        XCTAssertNil(tree.slices)

        tree.open(group: "doing")
        XCTAssertEqual(tree.containers?.map(\.id), [Fixtures.sourceCardID, Fixtures.sourceSecondCardID])
        XCTAssertNil(tree.slices)
        tree.container = Fixtures.sourceSecondCardID
        XCTAssertEqual(tree.slices?.map(\.sliceID), [Fixtures.sourceSecondCardTaskID])

        tree.open(group: "doing")
        XCTAssertEqual(tree.container, Fixtures.sourceSecondCardID, "reopening the same group keeps its container")
        tree.open(group: "ready/board")
        XCTAssertNil(tree.container)
        XCTAssertEqual(tree.containers?.map(\.id), [Fixtures.sourceBoardCardID, Fixtures.sourceMineCardID])

        tree.open(project: Fixtures.projectID)
        XCTAssertNil(tree.group)
        XCTAssertNil(tree.containers)
    }

    func testAContainerCrumbOpensOnTheFirstGroupListingIt() {
        let tree = CrumbTree(model: model(), projectID: Fixtures.sourceProjectID, container: Fixtures.sourceMineCardID)
        XCTAssertEqual(tree.group, "ready/mine")
        XCTAssertEqual(tree.slices, [])
        let gone = CrumbTree(model: model(), projectID: Fixtures.sourceProjectID, container: "nope")
        XCTAssertNil(gone.group)
        XCTAssertEqual(gone.slices, [])
    }

    // MARK: - The titlebar

    func testAContainerIsNamedByItsSourceNotAnActiveRow() {
        let icon = SourceIcon(symbol: "star", svg: "<svg/>")
        let identity = titlebarIdentity(
            for: .container(id: "4821", title: "Card", tag: "DM", icon: icon),
            projectID: "p", active: [], tags: ["p": "WOR"])
        XCTAssertEqual(identity, TitlebarIdentity(tag: "DM", state: .todo, live: false, title: "Card", icon: icon))
        XCTAssertEqual(identity, .container(title: "Card", tag: "DM", icon: icon))
        let untagged = titlebarIdentity(
            for: .container(id: "4821", title: "Card", tag: "", icon: icon), projectID: "p", active: [], tags: ["p": "WOR"])
        XCTAssertEqual(untagged.tag, "WOR")
        XCTAssertEqual(SourceIcon(symbol: "").symbol, SourceIcon.fallbackSymbol)
    }

    // MARK: - The container navigator

    func testTheNavigatorsSectionsFollowTheStoryAndSkipUnknownKinds() {
        let detail = ContainerDetail(id: "c", title: "C", sections: [
            SourceSection(id: "comments", title: "Comments", kind: .comments,
                          comments: [SourceComment(by: "a", when: "1d", text: "x")]),
            SourceSection(id: "body", title: "Description", kind: .prose, body: "Hi"),
            SourceSection(id: "poll", title: "Poll", kind: .unknown("poll")),
            SourceSection(id: "notes", title: "Notes", kind: .prose, body: "More"),
            SourceSection(id: "links", title: "Links", kind: .links),
        ])
        let model = ContainerNavigatorModel(show: ContainerShow(container: detail, tasks: Fixtures.sourceTasks))
        XCTAssertEqual(model.storyID, "body")
        XCTAssertEqual(model.storyTitle, "Description")
        XCTAssertEqual(model.sections.map(\.id), ["comments", "notes", "links"])
        XCTAssertEqual(model.comments?.id, "comments")
        XCTAssertEqual(model.meta(for: model.sections[0]), "1")
        XCTAssertNil(model.meta(for: model.sections[1]))
        XCTAssertEqual(model.meta(for: model.sections[2]), "none")
        XCTAssertEqual(model.paneMode(for: "links"), .section("links"))
        XCTAssertEqual(model.paneMode(for: "comments"), .story)
        XCTAssertEqual(model.paneMode(for: "nope"), .story)
        XCTAssertEqual(model.tasksFact, "0/4 done")
        XCTAssertEqual(model.defaultFocus, ContainerFocus(open: ["body"], main: .story))
    }

    func testAContainerWithNoProseIsStillAStory() {
        let model = ContainerNavigatorModel(show: ContainerShow(container: ContainerDetail(id: "c", title: "C")))
        XCTAssertEqual(model.storyID, ContainerNavigatorModel.storyID)
        XCTAssertEqual(model.storyTitle, "Story")
        XCTAssertEqual(model.tasksFact, "none yet")
        XCTAssertNil(model.comments)
        let fixture = ContainerNavigatorModel(show: Fixtures.sourceContainerShow(id: Fixtures.sourceCardID))
        XCTAssertEqual(fixture.meta(for: fixture.sections[1]), "3")
    }

    func testTheContainerFocusFollowsTheNavigatorsHeaderRules() {
        let start = ContainerFocus(open: ["story"], main: .story)
        let links = start.clickingHead("links", shows: .section("links"))
        XCTAssertEqual(links, ContainerFocus(open: ["story", "links"], main: .section("links")))
        XCTAssertEqual(links.clickingHead("links", shows: .section("links")).open, ["story"], "a second click folds")
        XCTAssertEqual(links.clickingHead("links", shows: .section("links")).main, .section("links"))
        XCTAssertEqual(start.togglingFold("story"), ContainerFocus(open: [], main: .story))
        XCTAssertEqual(start.togglingFold("x").open, ["story", "x"])
    }

    // MARK: - A plugin's colour

    func testAWireColourIsSixHexDigitsAfterAHash() {
        XCTAssertEqual(DesignTokens.wireHex("#4F6BD8"), "4f6bd8")
        XCTAssertEqual(DesignTokens.wireHex(" #2a9d8f "), "2a9d8f")
        for bad in ["4f6bd8", "#4f6bd", "#4f6bd80", "#zzzzzz", "", "#"] {
            XCTAssertNil(DesignTokens.wireHex(bad), bad)
        }
        XCTAssertNotNil(DesignTokens.wireTint("#4f6bd8"))
        XCTAssertNil(DesignTokens.wireTint("blue"))
        XCTAssertNotNil(DesignTokens.wireBadge("#4f6bd8", on: .header))
        XCTAssertNil(DesignTokens.wireBadge("", on: .header))
    }
}
