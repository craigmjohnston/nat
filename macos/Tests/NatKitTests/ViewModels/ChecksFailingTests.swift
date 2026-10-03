import XCTest
@testable import NatKit

/// CI failure feedback in the app: nat's words decoded, the notice decided,
/// the rail marked and the task log drawn.
final class ChecksFailingTests: XCTestCase {
    private func slice(
        _ id: String = "s-1", status: String = "In progress", pr: String = "https://pr/1", fixing: Bool = false,
        handedBack: Bool = false, branch: String? = nil
    ) -> Slice {
        Slice(
            id: id, name: "Slice \(id)", status: status, milestoneID: "M1", assignee: "", pr: pr, url: "",
            branch: branch, blocked: false, handedBack: handedBack, fixing: fixing)
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

    func testSliceDecodesFixingAndDefaultsItFalse() throws {
        let base = #""id":"s","name":"n","status":"In progress","milestone_id":"m","assignee":"","pr":"u","url":"","blocked":false,"handed_back":false"#
        XCTAssertTrue(try JSONDecoder().decode(Slice.self, from: Data("{\(base),\"fixing\":true}".utf8)).fixing)
        XCTAssertFalse(try JSONDecoder().decode(Slice.self, from: Data("{\(base)}".utf8)).fixing)
    }

    func testTaskLogDecodesChecksFailed() throws {
        let event = try JSONDecoder().decode(TaskLogEvent.self, from: Data(#"{"kind":"checks_failed","note":"- test"}"#.utf8))
        XCTAssertEqual(event, TaskLogEvent(.checksFailed, note: "- test"))
    }

    // MARK: - The stage

    func testFixingIsReadOffTheSlice() {
        XCTAssertEqual(stage(for: slice(fixing: true), agent: nil), .fixing)
        XCTAssertEqual(stage(for: slice(), agent: .working), .pr, "a live session moves nothing")
        XCTAssertEqual(displayState(for: slice(fixing: true), agent: nil), .fixing, "no agent: drawn as fixing, relaunchable")
        XCTAssertTrue(NavigatorModel(slice: slice(fixing: true), agent: nil).showsLaunch)
    }

    // MARK: - The notice

    func testNoticeOffersTheFixLaunchWithNoAgent() {
        let notice = checksNotice(slice: slice(), failing: ["test", "lint"], hasLiveAgent: false, events: nil)
        XCTAssertEqual(notice, ChecksNotice(checks: ["test", "lint"], action: .launchFix))
        XCTAssertEqual(notice?.text, "Failing: test, lint.")
    }

    func testNoticeSaysTheAgentWasToldWhenTheNudgeIsTheLatestEvent() {
        let told: [TaskLogEvent] = [TaskLogEvent(.handedBack), TaskLogEvent(.sentBack, note: "x"), TaskLogEvent(.approved, pr: "u")]
        let notice = checksNotice(slice: slice(fixing: true), failing: ["test"], hasLiveAgent: true, events: told)
        XCTAssertEqual(notice?.action, .agentTold)
        XCTAssertEqual(notice?.text, "Failing: test — the agent has been told.")

        let untold: [TaskLogEvent] = [TaskLogEvent(.sentBack, note: "x"), TaskLogEvent(.handedBack), TaskLogEvent(.approved, pr: "u")]
        XCTAssertEqual(checksNotice(slice: slice(), failing: ["test"], hasLiveAgent: true, events: untold)?.action, ChecksNotice.Action.none)
        XCTAssertEqual(checksNotice(slice: slice(), failing: ["test"], hasLiveAgent: true, events: nil)?.action, ChecksNotice.Action.none)
    }

    func testNoticeIsDrawnOnlyAtThePRStageOrUnderAFix() {
        XCTAssertNil(checksNotice(slice: slice(), failing: nil, hasLiveAgent: false, events: nil), "green, pending or unread")
        XCTAssertNil(checksNotice(slice: slice(status: "Done"), failing: ["test"], hasLiveAgent: false, events: nil))
        XCTAssertNil(checksNotice(slice: slice(pr: "", handedBack: true, branch: "b"), failing: ["test"], hasLiveAgent: false, events: nil))
        XCTAssertNotNil(checksNotice(slice: slice(fixing: true), failing: ["test"], hasLiveAgent: false, events: nil))
        XCTAssertEqual(ChecksNotice(checks: [], action: .none).text, "Checks are failing.")
    }

    // MARK: - The rail

    func testActiveRowCarriesTheMarkerAtThePRStageOnly() {
        let plan = ProjectInfo(
            project: Project(id: "p", name: "P", conventions: ""),
            milestones: [Milestone(id: "M1", name: "M1", order: 0, status: "Active")],
            slices: [slice("a"), slice("b", fixing: true), slice("c", pr: "")])
        let model = buildSidebarModel(
            projects: [SidebarProjectInput(id: "p", name: "P", plan: plan)],
            liveAgents: ["b": .waiting, "c": .working],
            failingChecks: ["a": ["test"], "b": ["lint"], "c": ["stale"]])
        let marks = Dictionary(uniqueKeysWithValues: model.active.map { ($0.targetID, $0.failingChecks) })
        XCTAssertEqual(marks["a"], ["test"])
        XCTAssertEqual(marks["b"], ["lint"], "under a fix, its agent waiting")
        XCTAssertEqual(marks["c"], [], "a working slice has no pull request to mark")
    }

    // MARK: - The task log

    func testChecksFailedDrawsAsItsOwnCard() {
        let log = buildThreadEvents(
            slice: slice(), agent: nil, brief: nil,
            events: [TaskLogEvent(.checksFailed, note: "- test: https://ci/1"), TaskLogEvent(.approved, pr: "u")])
        let card = log.first { $0.kind == .checksFailed }
        XCTAssertEqual(card?.title, "Checks failed")
        XCTAssertEqual(card?.body, "- test: https://ci/1")
        XCTAssertEqual(card?.tone, .hot)
    }
}
