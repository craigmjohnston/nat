import XCTest
@testable import NatKit
@testable import NatFixtures

/// The Task log: `slice-show`'s `events` decoded, and the cards built from
/// them in the order they happened.
final class TaskLogTests: XCTestCase {
    private func slice(status: String, branch: String? = nil, pr: String = "") -> Slice {
        Slice(
            id: "s", name: "Slice", status: status, milestoneID: "M1", assignee: "", pr: pr, url: "",
            branch: branch, blocked: false, handedBack: false)
    }

    private let prURL = "https://github.com/o/r/pull/40"

    // MARK: - Decoding

    func testTheEventsDecodeInOrderAndAnUnknownKindIsLeftOut() throws {
        let json = """
        {"id": "s", "name": "n", "url": "", "status": "Done", "milestone": "M1", "assignee": "",
         "blocked": false, "handed_back": false, "brief": "",
         "events": [
           {"kind": "handed_back", "note": "first"},
           {"kind": "something_newer", "note": "?"},
           {"kind": "follow_ups", "followUps": [
             {"index": 1, "title": "Queue me", "brief": "b", "decision": "queued", "link": "https://x"},
             {"index": 2, "title": "Undecided", "decision": ""}
           ]},
           {"kind": "released", "by": "Craig"},
           {"kind": "approved", "pr": "https://github.com/o/r/pull/40"},
           {"kind": "merged"}
         ]}
        """
        let detail = try JSONDecoder().decode(SliceDetail.self, from: Data(json.utf8))

        XCTAssertEqual(detail.events?.map(\.kind), [.handedBack, .followUps, .released, .approved, .merged])
        XCTAssertEqual(detail.events?[0].note, "first")
        XCTAssertEqual(detail.events?[1].followUps, [
            TaskFollowUp(index: 1, title: "Queue me", brief: "b", decision: .queued, link: "https://x"),
            TaskFollowUp(index: 2, title: "Undecided"),
        ])
        XCTAssertEqual(detail.events?[2].by, "Craig")
        XCTAssertEqual(detail.events?[3].pr, prURL)
    }

    func testANoteDecodesWithWhoItCameFrom() throws {
        let json = """
        {"id": "s", "name": "n", "url": "", "status": "Todo", "milestone": "M1", "assignee": "",
         "blocked": false, "handed_back": false, "brief": "",
         "events": [{"kind": "note", "note": "The seam moved.", "by": "\\"Draw it\\" (M2)"}]}
        """
        let detail = try JSONDecoder().decode(SliceDetail.self, from: Data(json.utf8))
        XCTAssertEqual(detail.events, [TaskLogEvent(.note, note: "The seam moved.", by: "\"Draw it\" (M2)")])
    }

    func testAReadingWithNoEventsHasNone() throws {
        let json = """
        {"id": "s", "name": "n", "url": "", "status": "Todo", "milestone": "M1", "assignee": "",
         "blocked": false, "handed_back": false, "brief": ""}
        """
        let detail = try JSONDecoder().decode(SliceDetail.self, from: Data(json.utf8))
        XCTAssertNil(detail.events, "a nat too old to report them is told apart from a slice with no history")
    }

    // MARK: - The cards

    func testTheAcceptanceHistoryReadsInOrder() {
        let log = buildThreadEvents(
            slice: slice(status: "Done", branch: "b", pr: prURL), agent: nil, brief: nil,
            events: Fixtures.taskLogEvents)

        XCTAssertEqual(log.map(\.kind), [
            .launched, .handedBack, .sentBack, .handedBack, .followUps, .sentBack, .handedBack, .approved, .merged,
        ])
        XCTAssertEqual(log.filter { $0.kind == .handedBack }.count, 3)
        XCTAssertEqual(log.filter { $0.kind == .sentBack }.count, 2)
        let followUps = log.first { $0.kind == .followUps }
        XCTAssertEqual(followUps?.meta, "proposed 2 follow-ups")
        XCTAssertEqual(followUps?.facts, [
            ThreadFact("queued", "Restore the last selected project on launch"),
            ThreadFact("dropped", "Drop the unused toolbar style"),
        ])
        XCTAssertEqual(followUps?.awaitsTriage, false)
        XCTAssertEqual(log.first { $0.kind == .approved }?.facts, [ThreadFact("pr", "#101"), ThreadFact("into", "main")])
    }

    func testANoteIsDrawnAsItsOwnCardLabelledWithWhoItCameFrom() {
        let log = buildThreadEvents(
            slice: slice(status: "In progress"), agent: nil, brief: nil, events: Fixtures.notedTaskLogEvents)

        XCTAssertEqual(log.map(\.kind), [.launched, .note, .handedBack, .sentBack, .note])
        XCTAssertEqual(log[1], ThreadEvent(
            .note, who: "\"Bootstrap the SwiftUI shell\" (M1: Foundations)", meta: "left a note",
            body: Fixtures.notedTaskLogEvents[0].note))
        XCTAssertEqual(log[4].who, "Craig Johnston")
        let anonymous = buildThreadEvents(
            slice: slice(status: "In progress"), agent: nil, brief: nil, events: [TaskLogEvent(.note, note: "n")])
        XCTAssertEqual(anonymous.last?.title, "Note")
        XCTAssertEqual(log[4].title, "Craig Johnston left a note")
    }

    /// An action reads as one line with who did it; a meta that is not an
    /// action (a comment's time) stays apart from who.
    func testAnActionsCardReadsAsOneLine() {
        let log = buildThreadEvents(
            slice: slice(status: "In progress"), agent: nil, brief: nil,
            events: [TaskLogEvent(.handedBack, note: "n")])
        XCTAssertEqual(log.last?.title, "Agent handed back")
        XCTAssertEqual(ThreadEvent(.launched, who: "Launched").title, "Launched")
        XCTAssertEqual(ThreadEvent(.agent, who: "Craig", meta: "2h ago", metaIsAction: false).title, "Craig")
    }

    /// Notes left on a slice never launched are read in its brief; they open
    /// no log of a launch that never happened.
    func testNotesAloneOnASliceNeverLaunchedOpenNoLog() {
        let log = buildThreadEvents(
            slice: slice(status: "Todo"), agent: nil, brief: nil,
            events: [TaskLogEvent(.note, note: "n", by: "Craig")])
        XCTAssertEqual(log, [])
    }

    func testTheLiveAgentSitsAfterWhatThePageRecordsAndBeforeTheApprove() {
        let agent = AgentStatus(sliceID: "s", session: "nat-s", activity: .waiting)
        let log = buildThreadEvents(
            slice: slice(status: "In progress", pr: "not a pull url"), agent: agent, brief: nil,
            events: [TaskLogEvent(.handedBack, note: ""), TaskLogEvent(.approved, pr: "not a pull url")])

        XCTAssertEqual(log.map(\.kind), [.launched, .handedBack, .agent, .approved])
        XCTAssertNil(log[1].body, "an empty note draws no body")
        XCTAssertEqual(log[3].facts, [ThreadFact("pr", "not a pull url")])
    }

    func testEveryKindHasACard() {
        let log = buildThreadEvents(
            slice: slice(status: "In progress"), agent: nil, brief: nil,
            events: [
                TaskLogEvent(.released, by: "Craig"),
                TaskLogEvent(.released, by: ""),
                TaskLogEvent(.relaunched),
                TaskLogEvent(.blocked, note: "No token."),
                TaskLogEvent(.summary, note: "Wrote it up."),
                TaskLogEvent(.followUps, followUps: [TaskFollowUp(index: 1, title: "Later", decision: .folded)]),
                TaskLogEvent(.followUps, followUps: [TaskFollowUp(index: 1, title: "Pending")]),
                TaskLogEvent(.approved),
            ])

        XCTAssertEqual(log.map(\.kind), [
            .launched, .released, .released, .relaunched, .blocked, .closed, .followUps, .followUps, .approved,
        ])
        XCTAssertEqual(log[1].title, "Craig released to Todo")
        XCTAssertEqual(log[2].title, "Released to Todo")
        XCTAssertEqual(log[3].title, "Relaunched on the work so far")
        XCTAssertEqual(log[4].title, "Agent blocked")
        XCTAssertEqual(log[4].tone, .hot)
        XCTAssertEqual(log[4].body, "No token.")
        XCTAssertEqual(log[5].body, "Wrote it up.")
        XCTAssertEqual(log[6].facts, [ThreadFact("folded in", "Later")])
        XCTAssertEqual(log[6].meta, "proposed 1 follow-up")
        XCTAssertTrue(log[7].awaitsTriage, "a proposal still undecided is the triage card")
        XCTAssertEqual(log[7].tone, .hot)
        XCTAssertEqual(log[8].facts, [])
    }

    func testAReleasedSliceKeepsItsHistory() {
        let log = buildThreadEvents(
            slice: slice(status: "Todo"), agent: nil, brief: nil, events: [TaskLogEvent(.released, by: "Craig")])
        XCTAssertEqual(log.map(\.kind), [.launched, .released])
        XCTAssertTrue(buildThreadEvents(slice: slice(status: "Todo"), agent: nil, brief: nil, events: []).isEmpty)
    }

    func testTheDecisionWords() {
        XCTAssertEqual(followUpDecisionWord(.queued), "queued")
        XCTAssertEqual(followUpDecisionWord(.folded), "folded in")
        XCTAssertEqual(followUpDecisionWord(.dropped), "dropped")
    }

    // MARK: - The label

    func testTheSectionIsTheTaskUntilItIsUnderWay() {
        let todo = NavigatorModel(slice: slice(status: "Todo"), agent: nil, fixLaunched: false)
        XCTAssertEqual(todo.threadLabel, "Task")
        let working = NavigatorModel(slice: slice(status: "In progress"), agent: .working, fixLaunched: false)
        XCTAssertEqual(working.threadLabel, "Task log")
        let done = NavigatorModel(slice: slice(status: "Done", branch: "b", pr: prURL), agent: nil, fixLaunched: false)
        XCTAssertEqual(done.threadLabel, "Task log")
    }

    // MARK: - What a send-back files

    func testAVisualSendBackRecordsEachCommentAndWhereItSits() {
        let comments = [
            PendingVisualComment(
                index: 1, name: "Settings, dark", uri: "/tmp/dark.png",
                point: nil, imageSize: CGSize(width: 1440, height: 900), text: "Too much contrast."),
            PendingVisualComment(
                index: 1, name: "Settings, dark", uri: "/tmp/dark.png",
                point: CGPoint(x: 412.4, y: 88), imageSize: CGSize(width: 1440, height: 900), text: "Clipped."),
        ]
        XCTAssertEqual(visualCommentsRecord(comments), """
        Settings, dark, whole image: Too much contrast.

        Settings, dark, at (412, 88): Clipped.
        """)
    }
}
