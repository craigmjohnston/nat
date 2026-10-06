import XCTest
@testable import NatKit

/// CI failure feedback in the app: nat's words decoded, the notice decided,
/// the rail marked and the task log drawn.
final class ChecksFailingTests: XCTestCase {
    private func slice(
        _ id: String = "s-1", status: String = "In progress", pr: String = "https://pr/1", resumed: Bool = false,
        handedBack: Bool = false, branch: String? = nil
    ) -> Slice {
        Slice(
            id: id, name: "Slice \(id)", status: status, milestoneID: "M1", assignee: "", pr: pr, url: "",
            branch: branch, blocked: false, handedBack: handedBack, resumed: resumed)
    }

    // MARK: - Decoding

    func testPRStatusDecodesTheChecks() throws {
        let json = """
        {"slices": [
          {"slice_id": "s-1", "name": "A", "pr": "u", "readiness": "checks failing",
           "checks": {"verdict": "failing", "failing": [{"name": "test", "url": "https://ci/1"}]}},
          {"slice_id": "s-2", "name": "B", "pr": "u", "readiness": "ready to merge", "checks": {"verdict": "passing"}},
          {"slice_id": "s-3", "name": "C", "pr": "u", "readiness": "unread"}
        ]}
        """
        let doc = try JSONDecoder().decode(PRStatusDoc.self, from: Data(json.utf8))
        XCTAssertEqual(doc.slices[0].checks, PRStatusChecks(verdict: "failing", failing: [PRStatusCheck(name: "test", url: "https://ci/1")]))
        XCTAssertTrue(doc.slices[0].isOpen)
        XCTAssertEqual(doc.slices[1].checks?.failing, [])
        XCTAssertNil(doc.slices[2].checks)
    }

    func testSliceDecodesResumedAndDefaultsItFalse() throws {
        let base = #""id":"s","name":"n","status":"In progress","milestone_id":"m","assignee":"","pr":"u","url":"","blocked":false,"handed_back":false"#
        XCTAssertTrue(try JSONDecoder().decode(Slice.self, from: Data("{\(base),\"resumed\":true}".utf8)).resumed)
        XCTAssertFalse(try JSONDecoder().decode(Slice.self, from: Data("{\(base)}".utf8)).resumed)
        // An older nat's key means nothing now.
        XCTAssertFalse(try JSONDecoder().decode(Slice.self, from: Data("{\(base),\"fixing\":true}".utf8)).resumed)
    }

    func testTaskLogDecodesChecksFailed() throws {
        let event = try JSONDecoder().decode(TaskLogEvent.self, from: Data(#"{"kind":"checks_failed","note":"- test"}"#.utf8))
        XCTAssertEqual(event, TaskLogEvent(.checksFailed, note: "- test"))
    }

    // MARK: - The stage

    func testResumedIsReadOffTheSlice() {
        XCTAssertEqual(stage(for: slice(resumed: true), agent: nil), .working)
        XCTAssertEqual(stage(for: slice(), agent: .working), .pr, "a live session moves nothing")
        XCTAssertEqual(displayState(for: slice(resumed: true), agent: nil), .working, "no agent: drawn as working, relaunchable")
        XCTAssertEqual(displayState(for: slice(resumed: true), agent: .waiting), .waiting)
        XCTAssertTrue(NavigatorModel(slice: slice(resumed: true), agent: nil).showsLaunch)
    }

    // MARK: - The notice

    func testNoticeOffersSendBackWithNoAgent() {
        let notice = checksNotice(slice: slice(), failing: ["test", "lint"], hasLiveAgent: false, events: nil)
        XCTAssertEqual(notice, ChecksNotice(checks: ["test", "lint"], action: .sendBack))
        XCTAssertEqual(notice?.text, "Checks failing: test, lint.")
    }

    func testNoticeSaysTheAgentWasToldWhenTheNudgeIsTheLatestEvent() {
        let told: [TaskLogEvent] = [TaskLogEvent(.handedBack), TaskLogEvent(.sentBack, note: "x"), TaskLogEvent(.approved, pr: "u")]
        let notice = checksNotice(slice: slice(), failing: ["test"], hasLiveAgent: true, events: told)
        XCTAssertEqual(notice?.action, .sentToAgent)
        XCTAssertEqual(notice?.text, "Checks failing: test — sent to the agent to fix.")

        let untold: [TaskLogEvent] = [TaskLogEvent(.sentBack, note: "x"), TaskLogEvent(.handedBack), TaskLogEvent(.approved, pr: "u")]
        XCTAssertEqual(checksNotice(slice: slice(), failing: ["test"], hasLiveAgent: true, events: untold)?.action, ChecksNotice.Action.none)
        XCTAssertEqual(checksNotice(slice: slice(), failing: ["test"], hasLiveAgent: true, events: nil)?.action, ChecksNotice.Action.none)
    }

    func testNoticeIsDrawnOnlyAtThePRStage() {
        XCTAssertNil(checksNotice(slice: slice(), failing: nil, hasLiveAgent: false, events: nil), "green, pending or unread")
        XCTAssertNil(checksNotice(slice: slice(status: "Done"), failing: ["test"], hasLiveAgent: false, events: nil))
        XCTAssertNil(checksNotice(slice: slice(pr: "", handedBack: true, branch: "b"), failing: ["test"], hasLiveAgent: false, events: nil))
        XCTAssertNil(
            checksNotice(slice: slice(resumed: true), failing: ["test"], hasLiveAgent: false, events: nil),
            "resumed: the red reading is of a commit its agent is replacing")
        XCTAssertEqual(ChecksNotice(checks: [], action: .none).text, "Checks failing.")
    }

    // MARK: - The rail

    func testActiveRowCarriesTheMarkerAtThePRStageOnly() {
        let plan = ProjectInfo(
            project: Project(id: "p", name: "P", conventions: ""),
            milestones: [Milestone(id: "M1", name: "M1", order: 0, status: "Active")],
            slices: [slice("a"), slice("b"), slice("c", pr: ""), slice("d", resumed: true)])
        let model = buildSidebarModel(
            projects: [SidebarProjectInput(id: "p", name: "P", plan: plan)],
            liveAgents: ["b": .waiting, "c": .working, "d": .working],
            prMarks: ["a": PRMarks(failingChecks: ["test"]), "b": PRMarks(failingChecks: ["lint"]),
                      "c": PRMarks(failingChecks: ["stale"]), "d": PRMarks(failingChecks: ["old"])])
        let marks = Dictionary(uniqueKeysWithValues: model.active.map { ($0.targetID, $0.marks.failingChecks) })
        XCTAssertEqual(marks["a"], ["test"])
        XCTAssertEqual(marks["b"], ["lint"], "at its pull request, its agent waiting")
        XCTAssertEqual(marks["c"], .some(nil), "a working slice has no pull request to mark")
        XCTAssertEqual(marks["d"], ["old"], "a resumed one keeps it while its agent fixes them")
    }

    // MARK: - The task log

    func testChecksFailedDrawsAsItsOwnCard() {
        let log = buildThreadEvents(
            slice: slice(), agent: nil, brief: nil,
            events: [TaskLogEvent(.checksFailed, note: "- test: https://ci/1"), TaskLogEvent(.approved, pr: "u")])
        let card = log.first { $0.kind == .checksFailed }
        XCTAssertEqual(card?.who, "Checks failed")
        XCTAssertNil(card?.meta)
        XCTAssertEqual(card?.body, "- test: https://ci/1")
        XCTAssertEqual(card?.tone, .hot)
    }

    /// A nudge's Sent back says the failing checks went to the agent; a
    /// review's own is yours.
    func testANudgeSentBackIsAttributedToTheChecksReading() {
        let log = buildThreadEvents(
            slice: slice(), agent: nil, brief: nil,
            events: [
                TaskLogEvent(.sentBack, note: "Rename the helper."),
                TaskLogEvent(.sentBack, note: "- test: https://ci/1", by: "CI"),
            ])
        let sentBack = log.filter { $0.kind == .sentBack }
        XCTAssertEqual(sentBack.map(\.title), ["You sent back with comments", "Checks failed"])
        XCTAssertNil(sentBack[1].meta)
        XCTAssertEqual(sentBack[1].tone, .accent)
        XCTAssertEqual(sentBack[1].body, "- test: https://ci/1\n\nSent to the agent to fix.")
    }

    /// A nudge with no note recorded is the sending alone.
    func testANudgeWithNoNoteSaysOnlyItWasSent() {
        let log = buildThreadEvents(
            slice: slice(), agent: nil, brief: nil, events: [TaskLogEvent(.sentBack, note: "", by: "CI")])
        XCTAssertEqual(log.first { $0.kind == .sentBack }?.body, "Sent to the agent to fix.")
    }
}
