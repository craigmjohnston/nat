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

    func testANoteFromATaskOnThePlanNamesItAsATaskRow() {
        let events = Fixtures.notedTaskLogEvents
        let log = buildThreadEvents(
            slice: slice(status: "In progress"), agent: nil, brief: nil, events: events,
            plan: Fixtures.slices, milestones: Fixtures.milestones)

        XCTAssertEqual(log.map(\.kind), [.launched, .note, .handedBack, .sentBack, .note])
        let shell = Fixtures.slices.first { $0.name == "Bootstrap the SwiftUI shell" }
        XCTAssertEqual(log[1], ThreadEvent(
            .note, who: "Another agent", meta: "left a note", body: events[0].note,
            facts: [ThreadFact("task", "Bootstrap the SwiftUI shell", sliceID: shell?.id)], when: events[0].at))
        XCTAssertEqual(log[1].title, "Another agent left a note")
        XCTAssertEqual(log[4].facts, [ThreadFact("source", "Craig Johnston")], "a person is plain text")
        XCTAssertEqual(log[2].when, events[1].at, "every recorded card carries its time")
        XCTAssertNil(log[0].when, "the launch has no time source")

        let anonymous = buildThreadEvents(
            slice: slice(status: "In progress"), agent: nil, brief: nil, events: [TaskLogEvent(.note, note: "n")])
        XCTAssertEqual(anonymous.last?.title, "Note")
        XCTAssertEqual(anonymous.last?.facts, [])
    }

    /// With no plan to match against, or a source the plan does not hold
    /// once, the provenance nat wrote is the source.
    func testANoteFromATaskNotOnThePlanIsItsSource() {
        let label = "\"Bootstrap the SwiftUI shell\" (M1: Foundations)"
        let event = TaskLogEvent(
            .note, note: "n", by: label,
            fromSlice: NoteSource(name: "Bootstrap the SwiftUI shell", milestone: "M1: Foundations"))
        let log = buildThreadEvents(slice: slice(status: "In progress"), agent: nil, brief: nil, events: [event])
        XCTAssertEqual(log.last?.facts, [ThreadFact("source", label)])
    }

    func testANoteSourceMatchesOneSliceByNameAndMilestone() {
        let milestones = [
            Milestone(id: "m1", name: "M1", order: 0, status: "Todo"),
            Milestone(id: "m2", name: "M2", order: 1, status: "Todo"),
        ]
        func task(_ id: String, _ name: String, _ milestone: String) -> Slice {
            Slice(id: id, name: name, status: "Todo", milestoneID: milestone, assignee: "", pr: "", url: "",
                  blocked: false, handedBack: false)
        }
        let plan = [task("a", "Draw it", "m1"), task("b", "Draw it", "m2"), task("c", "Loose", "gone")]

        XCTAssertEqual(noteSourceSlice(NoteSource(name: "Draw it", milestone: "M2"), plan: plan, milestones: milestones)?.id, "b")
        XCTAssertNil(noteSourceSlice(NoteSource(name: "Draw it"), plan: plan, milestones: milestones),
                     "a name alone matching two is no match")
        XCTAssertEqual(noteSourceSlice(NoteSource(name: "Loose"), plan: plan, milestones: milestones)?.id, "c")
        XCTAssertEqual(noteSourceSlice(NoteSource(name: "Loose", milestone: "gone"), plan: plan, milestones: milestones)?.id, "c",
                       "a milestone the plan does not list reads by its id")
        XCTAssertNil(noteSourceSlice(NoteSource(name: "Draw it", milestone: "M3"), plan: plan, milestones: milestones))
    }

    func testANoteDecodesItsSourceAndTime() throws {
        let json = """
        {"id": "s", "name": "n", "url": "", "status": "Todo", "milestone": "M1", "assignee": "",
         "blocked": false, "handed_back": false, "brief": "",
         "events": [
           {"kind": "note", "note": "a", "by": "\\"Draw it\\" (M2)", "fromSlice": {"name": "Draw it", "milestone": "M2"},
            "at": "2026-10-03T23:14:05+01:00"},
           {"kind": "note", "note": "b", "by": "\\"Loose\\"", "fromSlice": {"name": "Loose"}, "at": "not a time"}
         ]}
        """
        let detail = try JSONDecoder().decode(SliceDetail.self, from: Data(json.utf8))
        XCTAssertEqual(detail.events?[0].fromSlice, NoteSource(name: "Draw it", milestone: "M2"))
        XCTAssertEqual(detail.events?[0].at, Date(timeIntervalSince1970: 1_791_065_645))
        XCTAssertEqual(detail.events?[1].fromSlice, NoteSource(name: "Loose"))
        XCTAssertNil(detail.events?[1].at, "a time that will not parse is no time")
    }

    // MARK: - When

    func testTheTimestampIsTheTimeTodayTheDayThisYearAndTheYearBefore() throws {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = try XCTUnwrap(TimeZone(identifier: "Europe/London"))
        let locale = Locale(identifier: "en_GB")
        let now = try XCTUnwrap(calendar.date(from: DateComponents(year: 2026, month: 10, day: 3, hour: 23, minute: 30)))
        func at(_ year: Int, _ month: Int, _ day: Int, _ hour: Int = 9) throws -> Date {
            try XCTUnwrap(calendar.date(from: DateComponents(year: year, month: month, day: day, hour: hour, minute: 14)))
        }

        XCTAssertEqual(threadTimestamp(try at(2026, 10, 3, 23), now: now, calendar: calendar, locale: locale), "23:14")
        XCTAssertEqual(threadTimestamp(try at(2026, 10, 2), now: now, calendar: calendar, locale: locale), "2 Oct")
        XCTAssertEqual(threadTimestamp(try at(2026, 1, 1), now: now, calendar: calendar, locale: locale), "1 Jan")
        XCTAssertEqual(threadTimestamp(try at(2025, 10, 3), now: now, calendar: calendar, locale: locale), "3 Oct 2025")
        // The US form puts a narrow no-break space before PM; any space will do.
        let us = threadTimestamp(try at(2026, 10, 3, 23), now: now, calendar: calendar, locale: Locale(identifier: "en_US"))
        XCTAssertEqual(
            us.replacingOccurrences(of: "\u{202F}", with: " "), "11:14 PM", "the locale's own clock")
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
        let todo = NavigatorModel(slice: slice(status: "Todo"), agent: nil)
        XCTAssertEqual(todo.threadLabel, "Task")
        let working = NavigatorModel(slice: slice(status: "In progress"), agent: .working)
        XCTAssertEqual(working.threadLabel, "Task log")
        let done = NavigatorModel(slice: slice(status: "Done", branch: "b", pr: prURL), agent: nil)
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
