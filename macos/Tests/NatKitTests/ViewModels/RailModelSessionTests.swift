import XCTest
@testable import NatKit

/// Ad hoc sessions' own rail rows: listed after the workshop entry and
/// before slices while there is something to say about them, and as flat
/// DONE rows once there is nothing left open.
final class RailModelSessionTests: XCTestCase {
    private var project: Project!
    private var emptyInfo: ProjectInfo!

    override func setUp() {
        super.setUp()
        project = Project(id: "proj-1", name: "Test Project", conventions: "")
        emptyInfo = ProjectInfo(project: project, milestones: [], slices: [])
    }

    private func session(
        id: String, tag: String, live: Bool = false, branch: String = "session/fixture",
        prs: [SessionPR] = [], startedAt: Date = Date()
    ) -> Session {
        Session(id: id, tag: tag, live: live, startedAt: startedAt, dir: "/tmp/fixture", branch: branch, prs: prs)
    }

    // MARK: - Membership

    func testSessionIsActive_liveAgentWins() {
        let s = session(id: "s1", tag: "session:proj-1:s1")
        XCTAssertTrue(sessionIsActive(s, liveAgents: ["session:proj-1:s1": .working]))
        XCTAssertFalse(sessionIsActive(s, liveAgents: [:]))
    }

    func testSessionNeedsReview_goneWithAnOpenPR() {
        let s = session(id: "s1", tag: "session:proj-1:s1", prs: [
            SessionPR(number: 1, title: "x", url: "https://x", state: "OPEN"),
        ])
        XCTAssertTrue(sessionNeedsReview(s, liveAgents: [:]))
        XCTAssertFalse(sessionNeedsReview(s, liveAgents: ["session:proj-1:s1": .working]),
                       "a live agent is never in review, whatever its PRs read")
    }

    func testSessionNeedsReview_noOpenPRIsNotReview() {
        let s = session(id: "s1", tag: "session:proj-1:s1", prs: [
            SessionPR(number: 1, title: "x", url: "https://x", state: "MERGED"),
        ])
        XCTAssertFalse(sessionNeedsReview(s, liveAgents: [:]))
    }

    func testSessionIsDone_goneWithNothingOpen() {
        let merged = session(id: "s1", tag: "session:proj-1:s1", prs: [
            SessionPR(number: 1, title: "x", url: "https://x", state: "MERGED"),
        ])
        XCTAssertTrue(sessionIsDone(merged, liveAgents: [:]))

        let none = session(id: "s2", tag: "session:proj-1:s2")
        XCTAssertTrue(sessionIsDone(none, liveAgents: [:]), "no PR at all is done too, once the agent is gone")

        XCTAssertFalse(sessionIsDone(merged, liveAgents: ["session:proj-1:s1": .waiting]),
                       "a live agent is never done, however its PRs read")
    }

    // MARK: - ACTIVE rows

    func testBuildRailModel_liveSessionIsActiveAndPulses() {
        let s = session(id: "s1", tag: "session:proj-1:s1", branch: "session/live")
        let model = buildRailModel(
            from: emptyInfo, liveAgents: ["session:proj-1:s1": .working], sessions: [s]
        )

        XCTAssertEqual(model.active.count, 1)
        let entry = model.active[0]
        XCTAssertEqual(entry.kind, .session)
        XCTAssertEqual(entry.sliceID, "s1")
        XCTAssertEqual(entry.name, "Ad hoc session")
        XCTAssertEqual(entry.displayState, "Working")
        XCTAssertEqual(entry.tintRole, .working)
        XCTAssertTrue(entry.tintRole.pulses)
        XCTAssertEqual(entry.detail, ["session/live"])
        XCTAssertTrue(model.doneSessions.isEmpty)
    }

    func testBuildRailModel_waitingSessionDoesNotPulse() {
        let s = session(id: "s1", tag: "session:proj-1:s1")
        let model = buildRailModel(from: emptyInfo, liveAgents: ["session:proj-1:s1": .waiting], sessions: [s])

        XCTAssertEqual(model.active[0].displayState, "Waiting for input")
        XCTAssertEqual(model.active[0].tintRole, .waiting)
        XCTAssertFalse(model.active[0].tintRole.pulses)
    }

    func testBuildRailModel_liveSessionCarriesElapsedFromAgentStarts() {
        let started = Date().addingTimeInterval(-3600)
        let s = session(id: "s1", tag: "session:proj-1:s1")
        let model = buildRailModel(
            from: emptyInfo, liveAgents: ["session:proj-1:s1": .working],
            agentStarts: ["session:proj-1:s1": started], sessions: [s], now: Date()
        )

        XCTAssertEqual(model.active[0].meta, "1h 0m")
        XCTAssertEqual(model.active[0].metaRole, .elapsed)
    }

    func testBuildRailModel_goneSessionWithOpenPRNeedsReview() {
        let s = session(id: "s1", tag: "session:proj-1:s1", prs: [
            SessionPR(number: 5, title: "x", url: "https://x", state: "OPEN"),
            SessionPR(number: 6, title: "y", url: "https://y", state: "OPEN"),
        ])
        let model = buildRailModel(from: emptyInfo, liveAgents: [:], sessions: [s])

        XCTAssertEqual(model.active.count, 1)
        let entry = model.active[0]
        XCTAssertEqual(entry.displayState, "Needs review")
        XCTAssertEqual(entry.tintRole, .needsReview)
        XCTAssertEqual(entry.meta, "2 open")
        XCTAssertEqual(entry.metaRole, .stat)
        XCTAssertEqual(entry.detail.count, 2, "the label and when it started")
        XCTAssertTrue(entry.detail[1].hasPrefix("started "))
    }

    func testBuildRailModel_sessionsOrderedNewestFirst() {
        let older = session(id: "old", tag: "session:proj-1:old", startedAt: Date().addingTimeInterval(-1000))
        let newer = session(id: "new", tag: "session:proj-1:new", startedAt: Date())
        let model = buildRailModel(
            from: emptyInfo, liveAgents: ["session:proj-1:old": .working, "session:proj-1:new": .working],
            sessions: [older, newer]
        )

        XCTAssertEqual(model.active.map(\.sliceID), ["new", "old"])
    }

    func testBuildRailModel_sessionsListedAfterWorkshopAndBeforeSlices() {
        let workshop = ActiveEntry(kind: .workshop, name: "Workshop", displayState: "Working", tintRole: .working)
        let slice = Slice(
            id: "s-review", name: "Handed back", status: "In progress", milestoneID: "",
            assignee: "", pr: "", url: "", blocked: false, handedBack: true
        )
        let info = ProjectInfo(project: project, milestones: [], slices: [slice])
        let s = session(id: "s1", tag: "session:proj-1:s1", prs: [
            SessionPR(number: 1, title: "x", url: "https://x", state: "OPEN"),
        ])

        let model = buildRailModel(from: info, liveAgents: [:], workshop: workshop, sessions: [s])

        XCTAssertEqual(model.active.map(\.kind), [.workshop, .session, .slice])
    }

    // MARK: - DONE rows

    func testBuildRailModel_endedSessionMovesToDone() {
        let s = session(id: "s1", tag: "session:proj-1:s1", prs: [
            SessionPR(number: 1, title: "x", url: "https://x", state: "MERGED"),
        ])
        let model = buildRailModel(from: emptyInfo, liveAgents: [:], sessions: [s])

        XCTAssertTrue(model.active.isEmpty)
        XCTAssertEqual(model.doneSessions.count, 1)
        let entry = model.doneSessions[0]
        XCTAssertEqual(entry.kind, .session)
        XCTAssertEqual(entry.sliceID, "s1")
        XCTAssertEqual(entry.displayState, "Ended")
        XCTAssertEqual(entry.tintRole, .done)
        XCTAssertEqual(entry.detail.count, 2, "the label and when it started")
        XCTAssertTrue(entry.detail[1].hasPrefix("started "))
    }

    func testBuildRailModel_doneSessionsDoNotRequireADoneSlice() {
        // No milestones, no slices at all — a done session alone should
        // still surface (the rail's own `drawnSections`/call-site decides
        // whether to draw the section at all; this is the model's own
        // contribution to that decision).
        let s = session(id: "s1", tag: "session:proj-1:s1", prs: [
            SessionPR(number: 1, title: "x", url: "https://x", state: "MERGED"),
        ])
        let model = buildRailModel(from: emptyInfo, liveAgents: [:], sessions: [s])
        XCTAssertNil(model.doneSummary, "no slice is done, so the slice-only summary stays nil")
        XCTAssertEqual(model.doneSessions.count, 1, "but the session itself is still reported as done")
    }
}
