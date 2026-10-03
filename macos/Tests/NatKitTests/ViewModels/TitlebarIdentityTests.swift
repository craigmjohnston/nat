import XCTest
@testable import NatKit
@testable import NatFixtures

/// The navigator titlebar's tag, dot and title: read off the selection's
/// Active row where it has one, the project's own tag otherwise.
final class TitlebarIdentityTests: XCTestCase {
    private func row(
        _ kind: SidebarActiveKind, _ targetID: String, project: String = "p", tag: String = "GNA",
        state: SliceDisplayState, live: Bool
    ) -> SidebarActiveRow {
        SidebarActiveRow(
            kind: kind, targetID: targetID, projectID: project, projectName: "gnat", projectTag: tag,
            title: "row title", state: state, live: live)
    }

    private let tags = ["p": "GNA", "q": "QUI"]

    func testASliceWithAnActiveRowReadsTheRow() {
        let identity = titlebarIdentity(
            for: .slice(id: "s1", name: "Draw the box", state: .working), projectID: "p",
            active: [row(.slice, "s1", tag: "GN1", state: .waiting, live: true)], tags: tags)
        XCTAssertEqual(identity, TitlebarIdentity(tag: "GN1", state: .waiting, live: true, title: "Draw the box"))
    }

    func testASliceWithNoActiveRowTakesTheProjectsTagAndItsOwnStateNotLive() {
        let identity = titlebarIdentity(
            for: .slice(id: "s1", name: "Draw the box", state: .done), projectID: "p",
            active: [row(.slice, "s1", project: "q", state: .working, live: true)], tags: tags)
        XCTAssertEqual(identity, TitlebarIdentity(tag: "GNA", state: .done, live: false, title: "Draw the box"))
    }

    func testALiveWorkshopReadsItsRow() {
        let identity = titlebarIdentity(
            for: .workshop, projectID: "p",
            active: [row(.slice, "p", state: .todo, live: false), row(.workshop, "p", state: .working, live: true)],
            tags: tags)
        XCTAssertEqual(identity, TitlebarIdentity(tag: "GNA", state: .working, live: true, title: workshopRowTitle))
    }

    func testAWorkshopNothingRunsForIsTodo() {
        let identity = titlebarIdentity(for: .workshop, projectID: "q", active: [], tags: tags)
        XCTAssertEqual(identity, TitlebarIdentity(tag: "QUI", state: .todo, live: false, title: workshopRowTitle))
    }

    func testASessionReadsItsRowAndAnEndedOneIsDone() {
        let live = titlebarIdentity(
            for: .session(id: "x", title: "Ad hoc session · fix"), projectID: "p",
            active: [row(.session, "x", state: .waiting, live: true)], tags: tags)
        XCTAssertEqual(live, TitlebarIdentity(tag: "GNA", state: .waiting, live: true, title: "Ad hoc session · fix"))

        let ended = titlebarIdentity(for: .session(id: "y", title: "t"), projectID: "p", active: [], tags: [:])
        XCTAssertEqual(ended, TitlebarIdentity(tag: "", state: .done, live: false, title: "t"))
    }

    func testTheLastCrumbDropsTheTagAfterAProjectCrumb() {
        let identity = TitlebarIdentity(tag: "GNA", state: .waiting, live: true, title: "Draw the box")
        XCTAssertEqual(
            identity.lastCrumb(afterProjectCrumb: true),
            TitlebarIdentity(tag: "", state: .waiting, live: true, title: "Draw the box"))
    }

    func testTheLastCrumbKeepsTheTagWithNoProjectCrumbBeforeIt() {
        let identity = TitlebarIdentity(tag: "GNA", state: .waiting, live: true, title: "Draw the box")
        XCTAssertEqual(identity.lastCrumb(afterProjectCrumb: false), identity)
    }

    func testAContainersLastCrumbKeepsItsIconWhenItsTagIsDropped() {
        let icon = SourceIcon(symbol: "rectangle.stack")
        let crumb = TitlebarIdentity.container(title: "Billing", tag: "SC", icon: icon)
            .lastCrumb(afterProjectCrumb: true)
        XCTAssertEqual(crumb, TitlebarIdentity(tag: "", state: .todo, live: false, title: "Billing", icon: icon))
    }

    @MainActor
    func testTheAppModelReadsTheActiveProjectsRows() async {
        let model = await Fixtures.startedAppModel(client: FixtureNatClient(agents: []))
        model.openWorkshop()

        let identity = model.titlebarIdentity(for: .workshop)

        let pinned = model.sidebarModel.active.first { $0.kind == .workshop }
        XCTAssertEqual(identity.tag, pinned?.projectTag)
        XCTAssertFalse(identity.tag.isEmpty)
        XCTAssertEqual(identity.state, .todo)
        XCTAssertFalse(identity.live)
    }
}
