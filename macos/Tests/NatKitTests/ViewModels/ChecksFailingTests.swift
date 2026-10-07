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

    /// Every check the reading rolled up decodes under `checks`; an older
    /// nat's reading, with no such list, decodes with none.
    func testPRStatusDecodesEveryCheck() throws {
        let json = """
        {"slices": [
          {"slice_id": "s-1", "name": "A", "pr": "u", "readiness": "awaiting review",
           "checks": {"verdict": "pending", "failing": [], "checks": [
             {"name": "CI / lint", "state": "SUCCESS", "url": "https://ci/1"},
             {"name": "CI / test", "state": "IN_PROGRESS", "url": "https://ci/2"},
             {"name": "deploy"}]}},
          {"slice_id": "s-2", "name": "B", "pr": "u", "readiness": "ready to merge", "checks": {"verdict": "passing"}}
        ]}
        """
        let doc = try JSONDecoder().decode(PRStatusDoc.self, from: Data(json.utf8))
        XCTAssertEqual(doc.slices[0].checks?.checks, [
            PRStatusCheckState(name: "CI / lint", state: "SUCCESS", url: "https://ci/1"),
            PRStatusCheckState(name: "CI / test", state: "IN_PROGRESS", url: "https://ci/2"),
            PRStatusCheckState(name: "deploy", state: ""),
        ])
        XCTAssertNil(doc.slices[1].checks?.checks)
        let encoded = try JSONDecoder().decode(PRStatusDoc.self, from: JSONEncoder().encode(doc))
        XCTAssertEqual(encoded, doc, "round-trips through the read cache")
    }

    /// The PR section lists the reading's checks wherever it read the slice's,
    /// keeping what only the detail knows of each by name; else the detail's.
    func testCheckRowsFollowTheReadingOverAStaleDetail() {
        let detail = [
            PRCheck(name: "CI / lint", state: "SUCCESS", link: "https://pr/lint", rerunnable: true, run: "7"),
            PRCheck(name: "CI / test", state: "IN_PROGRESS", link: "https://pr/test", rerunnable: true, run: "7"),
        ]
        let reading = PRReading(PRStatusDoc(slices: [
            PRStatusSlice(sliceID: "s-1", name: "A", pr: "u", readiness: PRStatusSlice.readyToMerge,
                          checks: PRStatusChecks(verdict: "passing", checks: [
                              PRStatusCheckState(name: "CI / lint", state: "SUCCESS", url: "https://ci/lint"),
                              PRStatusCheckState(name: "CI / test", state: "SUCCESS"),
                              PRStatusCheckState(name: "deploy", state: "SUCCESS", url: "https://ci/deploy"),
                          ])),
            PRStatusSlice(sliceID: "s-2", name: "B", pr: "u", readiness: PRStatusSlice.readyToMerge,
                          checks: PRStatusChecks(verdict: "passing")),
        ]))
        XCTAssertEqual(reading.checkRows(sliceID: "s-1", detail: detail), [
            PRCheck(name: "CI / lint", state: "SUCCESS", link: "https://ci/lint", rerunnable: true, run: "7"),
            PRCheck(name: "CI / test", state: "SUCCESS", link: "https://pr/test", rerunnable: true, run: "7"),
            PRCheck(name: "deploy", state: "SUCCESS", link: "https://ci/deploy"),
        ])
        XCTAssertEqual(reading.checkRows(sliceID: "s-2", detail: detail), detail, "an older nat's reading lists none")
        XCTAssertEqual(reading.checkRows(sliceID: "session", detail: detail), detail, "not in the reading")
        XCTAssertEqual(PRReading.empty.checkRows(sliceID: "s-1", detail: detail), detail, "no reading yet")
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
        let notice = checksNotice(slice: slice(), marks: PRMarks(failingChecks: ["test", "lint"]), hasLiveAgent: false, events: nil)
        XCTAssertEqual(notice, ChecksNotice(checks: ["test", "lint"], action: .sendBack))
        XCTAssertEqual(notice?.text, "Checks failing: test, lint.")
    }

    func testNoticeSaysTheAgentWasToldWhenTheNudgeIsTheLatestEvent() {
        let told: [TaskLogEvent] = [TaskLogEvent(.handedBack), TaskLogEvent(.sentBack, note: "x"), TaskLogEvent(.approved, pr: "u")]
        let notice = checksNotice(slice: slice(), marks: PRMarks(failingChecks: ["test"]), hasLiveAgent: true, events: told)
        XCTAssertEqual(notice?.action, .sentToAgent)
        XCTAssertEqual(notice?.text, "Checks failing: test — sent to the agent to fix.")

        let untold: [TaskLogEvent] = [TaskLogEvent(.sentBack, note: "x"), TaskLogEvent(.handedBack), TaskLogEvent(.approved, pr: "u")]
        XCTAssertEqual(checksNotice(slice: slice(), marks: PRMarks(failingChecks: ["test"]), hasLiveAgent: true, events: untold)?.action, ChecksNotice.Action.none)
        XCTAssertEqual(checksNotice(slice: slice(), marks: PRMarks(failingChecks: ["test"]), hasLiveAgent: true, events: nil)?.action, ChecksNotice.Action.none)
    }

    func testNoticeIsDrawnWhereTheSidebarMarksTheFailure() {
        XCTAssertNil(checksNotice(slice: slice(), marks: .none, hasLiveAgent: false, events: nil), "green, pending or unread")
        XCTAssertNil(checksNotice(slice: slice(status: "Done"), marks: PRMarks(failingChecks: ["test"]), hasLiveAgent: false, events: nil))
        XCTAssertNil(checksNotice(slice: slice(pr: "", handedBack: true, branch: "b"), marks: PRMarks(failingChecks: ["test"]), hasLiveAgent: false, events: nil))

        // Resumed on the nudge: the failure as read, then held off the task
        // log while the fix's checks run, until its hand-back.
        let nudged: [TaskLogEvent] = [TaskLogEvent(.handedBack), TaskLogEvent(.resumed), TaskLogEvent(.sentBack, note: "x")]
        let resumed = checksNotice(
            slice: slice(resumed: true), marks: PRMarks(failingChecks: ["test"]), hasLiveAgent: true, events: nudged)
        XCTAssertEqual(resumed, ChecksNotice(checks: ["test"], action: .sentToAgent))
        let fixing = Slice(
            id: "s-1", name: "", status: "In progress", milestoneID: "M1", assignee: "", pr: "https://pr/1", url: "",
            blocked: false, handedBack: false, resumed: true, takenBack: true, fixingChecks: ["test"])
        XCTAssertEqual(
            checksNotice(slice: fixing, marks: PRMarks(checksRunning: true), hasLiveAgent: true, events: nudged),
            ChecksNotice(checks: ["test"], action: .sentToAgent), "its fix's checks running")
        XCTAssertNil(
            checksNotice(slice: slice(resumed: true), marks: PRMarks(checksRunning: true), hasLiveAgent: true, events: nil),
            "running, with no failure on its log since the hand-back")
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
