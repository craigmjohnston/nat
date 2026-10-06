import XCTest
@testable import NatKit
@testable import NatFixtures

/// A source project's tasks in Active and the titlebar: no badge of the
/// project's own, each task nested under its card, the card named by its
/// badge.
final class SourceActiveTests: XCTestCase {
    private let work = Fixtures.sourceProjectID

    /// The Work plan with the second card's task under way too, so both
    /// cards have a task in Active.
    private var plan: ProjectInfo {
        let base = Fixtures.sourceProjectInfo()
        let started = base.slices.map { slice -> Slice in
            guard slice.id == Fixtures.sourceSecondCardTaskID else { return slice }
            return Slice(
                id: slice.id, name: slice.name, status: "In progress", milestoneID: slice.milestoneID,
                assignee: "Craig Johnston", pr: "", url: slice.url, blocked: false, handedBack: false, state: .working)
        }
        return ProjectInfo(project: base.project, milestones: base.milestones, slices: started, source: base.source)
    }

    private func model() -> SidebarModel {
        buildSidebarModel(
            projects: [
                SidebarProjectInput(id: Fixtures.projectID, name: "notion-agent-tracker", plan: Fixtures.projectInfo),
                SidebarProjectInput(id: work, name: "Work", plan: plan),
            ],
            liveAgents: [:])
    }

    func testASourcesActiveTasksNestUnderTheirCardWhichCarriesItsBadge() throws {
        let model = model()
        let rows = model.active.filter { $0.projectID == work }
        XCTAssertEqual(
            Set(rows.map(\.targetID)),
            [Fixtures.sourceWorkingTaskID, Fixtures.sourceReviewTaskID, Fixtures.sourceSecondCardTaskID])
        XCTAssertTrue(rows.allSatisfy { $0.projectTag.isEmpty }, "a source project takes no badge")
        XCTAssertEqual(model.sources.first?.tag, "")

        let cards = model.activeEntries.compactMap { entry -> (SidebarActiveCard, [String])? in
            guard case .card(let card, let rows) = entry else { return nil }
            return (card, rows.map(\.targetID))
        }
        XCTAssertEqual(cards.map(\.0.id), [Fixtures.sourceCardID, Fixtures.sourceSecondCardID])
        let first = try XCTUnwrap(cards.first)
        XCTAssertEqual(first.0.title, "Improve diff review ergonomics")
        XCTAssertEqual(first.0.badge, Fixtures.sourceMobileApp)
        XCTAssertEqual(first.0.icon, Fixtures.sourceInfo().icon)
        XCTAssertEqual(Set(first.1), [Fixtures.sourceWorkingTaskID, Fixtures.sourceReviewTaskID])
        XCTAssertNil(cards[1].0.badge, "a card with no project has no badge")
        XCTAssertEqual(cards[1].1, [Fixtures.sourceSecondCardTaskID])
        XCTAssertEqual(cards.first?.0.id, model.activeEntries.compactMap {
            if case .card(let card, _) = $0 { card.id } else { nil }
        }.first)

        // Every other row is its own entry, in Active's order.
        let flat = model.activeEntries.compactMap { entry -> String? in
            if case .row(let row) = entry { row.id } else { nil }
        }
        XCTAssertEqual(flat, model.active.filter { $0.card == nil }.map(\.id))
        XCTAssertEqual(model.activeEntries.flatMap(\.rows).count, model.active.count)
        XCTAssertTrue(model.activeEntries.allSatisfy { !$0.id.isEmpty })
    }

    func testACardTheTreeDoesNotListIsTitledFromThePlanAndOneNeitherNamesHasNone() {
        let bare = SourceInfo(name: "demo", title: "", tag: "", iconSymbol: "star", containerNoun: "card", taskNoun: "task")
        let base = Fixtures.sourceProjectInfo()
        let plan = ProjectInfo(project: base.project, milestones: base.milestones, slices: base.slices, source: bare)
        let card = activeCard(Fixtures.sourceCardID, projectID: work, plan: plan)
        XCTAssertEqual(card?.title, "Improve diff review ergonomics")
        XCTAssertNil(card?.badge)
        XCTAssertNil(activeCard("nope", projectID: work, plan: plan))
        XCTAssertNil(activeCard("", projectID: work, plan: plan))
        XCTAssertNil(activeCard(Fixtures.sourceCardID, projectID: work, plan: Fixtures.projectInfo), "not a source")
    }

    func testSourceInfoFindsAContainerAtAnyDepth() {
        let badge = SourceBadge(text: "DEP", color: "#123456")
        let info = SourceInfo(
            name: "demo", title: "", tag: "", iconSymbol: "star", iconSVG: "<svg/>", containerNoun: "card",
            taskNoun: "task",
            groups: [
                SourceGroup(id: "a", label: "A", containers: [SourceContainer(id: "1", title: "Top")]),
                SourceGroup(id: "b", label: "B", children: [
                    SourceGroup(id: "c", label: "C", containers: [SourceContainer(id: "9", title: "Deep", badges: [badge])]),
                ]),
            ])
        XCTAssertEqual(info.container(withID: "9")?.title, "Deep")
        XCTAssertEqual(info.badge(ofContainer: "9"), badge)
        XCTAssertNil(info.badge(ofContainer: "1"))
        XCTAssertNil(info.container(withID: "x"))
        XCTAssertEqual(info.icon, SourceIcon(symbol: "star", svg: "<svg/>"))
    }

    func testTheTitlebarNamesASourceTaskByItsCardNeverATag() {
        let model = model()
        let icon = Fixtures.sourceInfo().icon
        let live = titlebarIdentity(
            for: .slice(id: Fixtures.sourceWorkingTaskID, name: "x", state: .working), projectID: work,
            active: model.active, tags: [work: "DM"], plan: plan)
        XCTAssertEqual(live.tag, "")
        XCTAssertEqual(live.cardBadge, Fixtures.sourceMobileApp)
        XCTAssertEqual(live.cardIcon, icon)

        // A row with no card still leads with the source's icon.
        let uncarded = SidebarActiveRow(
            kind: .slice, targetID: "t", projectID: work, projectName: "Work", projectTag: "", title: "t",
            state: .working, live: true)
        let alone = titlebarIdentity(
            for: .slice(id: "t", name: "t", state: .working), projectID: work, active: [uncarded], tags: [:], plan: plan)
        XCTAssertNil(alone.cardBadge)
        XCTAssertEqual(alone.cardIcon, icon)
        XCTAssertNil(titlebarIdentity(
            for: .slice(id: "t", name: "t", state: .working), projectID: work, active: [uncarded], tags: [:]).cardIcon)

        // Out of Active: the plan's card, and nothing but the icon for a workshop.
        let workshop = titlebarIdentity(for: .workshop, projectID: work, active: [], tags: [work: "DM"], plan: plan)
        XCTAssertEqual(workshop.tag, "")
        XCTAssertNil(workshop.cardBadge)
        XCTAssertEqual(workshop.cardIcon, icon)
        XCTAssertEqual(
            titlebarIdentity(for: .workshop, projectID: Fixtures.projectID, active: [], tags: [Fixtures.projectID: "NOT"],
                             plan: Fixtures.projectInfo).tag,
            "NOT", "not a source: the project's own tag")
    }

    func testOnlyASliceRowCarriesACard() {
        let card = SidebarActiveCard(id: "c", projectID: work, title: "Card", badge: nil, icon: SourceIcon(symbol: "star"))
        let session = SidebarActiveRow(
            kind: .session, targetID: "s", projectID: work, projectName: "Work", projectTag: "", title: "s",
            state: .working, live: true, card: card)
        XCTAssertNil(session.card)
    }
}
