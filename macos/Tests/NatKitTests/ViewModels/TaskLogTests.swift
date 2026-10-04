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
             {"index": 1, "title": "Queue me", "brief": "b", "decision": "queued", "link": "https://x",
              "decidedAt": "2026-10-03T23:14:05+01:00"},
             {"index": 2, "title": "Undecided", "decision": "", "decidedAt": "not a time"}
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
            TaskFollowUp(
                index: 1, title: "Queue me", brief: "b", decision: .queued, link: "https://x",
                decidedAt: Date(timeIntervalSince1970: 1_791_065_645)),
            TaskFollowUp(index: 2, title: "Undecided"),
        ], "a decision time that will not parse is no time")
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
            events: Fixtures.taskLogEvents, plan: Fixtures.slices, milestones: Fixtures.milestones)

        XCTAssertEqual(log.map(\.kind), [
            .launched, .handedBack, .sentBack, .handedBack, .followUps, .followUp, .followUp, .followUp,
            .sentBack, .handedBack, .approved, .merged,
        ])
        XCTAssertEqual(log.filter { $0.kind == .handedBack }.count, 3)
        XCTAssertEqual(log.filter { $0.kind == .sentBack }.count, 2)
        let followUps = log.first { $0.kind == .followUps }
        XCTAssertEqual(followUps?.meta, "proposed 3 follow-ups")
        XCTAssertEqual(followUps?.facts, [], "the proposal is the count line alone")
        XCTAssertEqual(followUps?.awaitsTriage, false)
        XCTAssertEqual(followUps?.isLive, false, "a settled proposal is no longer live")
        XCTAssertFalse(log.contains(where: \.isLive), "nothing in a merged slice's log is live")
        XCTAssertEqual(log.first { $0.kind == .approved }?.facts, [ThreadFact("pr", "#101"), ThreadFact("into", "main")])
    }

    /// Each decided follow-up is its own card after its proposal, headed by
    /// the decision as one sentence, its title then its brief as the body,
    /// the queued one's slice as a task row, and the time it was decided —
    /// its proposal's where that was not recorded.
    func testEachDecidedFollowUpIsItsOwnCard() throws {
        let event = try XCTUnwrap(Fixtures.taskLogEvents.first { $0.kind == .followUps })
        let log = buildThreadEvents(
            slice: slice(status: "Done", branch: "b", pr: prURL), agent: nil, brief: nil,
            events: Fixtures.taskLogEvents, plan: Fixtures.slices, milestones: Fixtures.milestones)
        let decided = log.filter { $0.kind == .followUp }

        XCTAssertEqual(decided.map(\.title), [
            "Queued proposed follow-up", "Folded in proposed follow-up", "Dismissed proposed follow-up",
        ])
        XCTAssertEqual(decided.map(\.body), event.followUps.map { "\($0.title)\n\n\($0.brief)" })
        XCTAssertEqual(decided[0].facts, [ThreadFact("task", "Cache the plan on disk", sliceID: Fixtures.cacheSliceID)])
        XCTAssertEqual(decided[1].facts, [])
        XCTAssertEqual(decided[2].facts, [])
        XCTAssertEqual(decided.map(\.when), [event.followUps[0].decidedAt, event.followUps[1].decidedAt, event.at])
        XCTAssertNotNil(event.followUps[0].decidedAt)
        XCTAssertNotEqual(event.followUps[0].decidedAt, event.at, "the decision's own time, not the proposal's")
        XCTAssertFalse(decided.contains(where: \.isLive))
    }

    /// A queued follow-up's link names its slice by URL or by ID; one the
    /// plan does not hold draws no task row, and a pending one no card.
    func testAQueuedFollowUpsSliceIsFoundByLink() {
        func task(_ id: String, url: String) -> Slice {
            Slice(id: id, name: id, status: "Todo", milestoneID: "m", assignee: "", pr: "", url: url,
                  blocked: false, handedBack: false)
        }
        let plan = [task("1ef38308-f654-81bf-a16e-c48ada02cca5", url: ""), task("b", url: "https://x/b")]

        XCTAssertEqual(followUpSlice(link: "https://x/b", plan: plan)?.id, "b")
        XCTAssertEqual(
            followUpSlice(link: "https://www.notion.so/Some-title-1ef38308f65481bfa16ec48ada02cca5", plan: plan)?.name,
            "1ef38308-f654-81bf-a16e-c48ada02cca5")
        XCTAssertEqual(followUpSlice(link: "1EF38308F65481BFA16EC48ADA02CCA5", plan: plan)?.url, "")
        XCTAssertNil(followUpSlice(link: "https://x/gone", plan: plan))
        XCTAssertNil(followUpSlice(link: "", plan: plan))
        XCTAssertNil(followUpSlice(link: nil, plan: plan))

        let log = buildThreadEvents(
            slice: slice(status: "In progress"), agent: nil, brief: nil,
            events: [TaskLogEvent(.followUps, followUps: [
                TaskFollowUp(index: 1, title: "Gone", decision: .queued, link: "https://x/gone"),
                TaskFollowUp(index: 2, title: "Undecided"),
            ])],
            plan: plan)
        XCTAssertEqual(log.map(\.kind), [.launched, .followUps, .followUp])
        XCTAssertEqual(log[2].facts, [], "a slice the plan does not hold is no row")
        XCTAssertEqual(log[2].body, "Gone", "an empty brief leaves the title alone")
        XCTAssertTrue(log[1].isLive, "a proposal awaiting a decision is live")
    }

    /// The live agent's card is live whether working or waiting.
    func testTheLiveAgentsCardIsLive() {
        for activity in [AgentActivityState.working, .waiting] {
            let log = buildThreadEvents(
                slice: slice(status: "In progress"), agent: AgentStatus(sliceID: "s", session: "nat-s", activity: activity),
                brief: nil, events: [TaskLogEvent(.handedBack, note: "n")])
            XCTAssertEqual(log.map(\.isLive), [false, false, true])
        }
    }

    func testTheWidestFactKeyIsTheLongest() {
        XCTAssertEqual(widestThreadFactKey, "depends on")
        XCTAssertTrue(threadFactKeys.allSatisfy { $0.count <= widestThreadFactKey.count })
    }

    func testANoteFromATaskOnThePlanNamesItAsATaskRow() {
        let events = Fixtures.notedTaskLogEvents
        let log = buildThreadEvents(
            slice: slice(status: "In progress"), agent: nil, brief: nil, events: events,
            plan: Fixtures.slices, milestones: Fixtures.milestones)

        XCTAssertEqual(log.map(\.kind), [.note, .launched, .handedBack, .sentBack, .note])
        let shell = Fixtures.slices.first { $0.name == "Bootstrap the SwiftUI shell" }
        XCTAssertEqual(log[0], ThreadEvent(
            .note, who: "Another agent", meta: "left a note", body: events[0].note,
            facts: [ThreadFact("task", "Bootstrap the SwiftUI shell", sliceID: shell?.id)], when: events[0].at))
        XCTAssertEqual(log[0].title, "Another agent left a note")
        XCTAssertEqual(log[4].facts, [ThreadFact("source", "Craig Johnston")], "a person is plain text")
        XCTAssertEqual(log[2].when, events[1].at, "every recorded card carries its time")
        XCTAssertNil(log[1].when, "the launch has no time source")

        let anonymous = buildThreadEvents(
            slice: slice(status: "In progress"), agent: nil, brief: nil, events: [TaskLogEvent(.note, note: "n")])
        XCTAssertEqual(anonymous.first?.title, "Note")
        XCTAssertEqual(anonymous.first?.facts, [])
    }

    /// With no plan to match against, or a source the plan does not hold
    /// once, the provenance nat wrote is the source.
    func testANoteFromATaskNotOnThePlanIsItsSource() {
        let label = "\"Bootstrap the SwiftUI shell\" (M1: Foundations)"
        let event = TaskLogEvent(
            .note, note: "n", by: label,
            fromSlice: NoteSource(name: "Bootstrap the SwiftUI shell", milestone: "M1: Foundations"))
        let log = buildThreadEvents(slice: slice(status: "In progress"), agent: nil, brief: nil, events: [event])
        XCTAssertEqual(log.first?.facts, [ThreadFact("source", label)])
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

    /// Notes left on a slice never launched are its log, with no launch
    /// claimed before them.
    func testNotesAloneOnASliceNeverLaunchedAreItsLog() {
        let note = TaskLogEvent(.note, note: "n", by: "Craig", at: Date(timeIntervalSince1970: 1_791_065_645))
        let log = buildThreadEvents(slice: slice(status: "Todo"), agent: nil, brief: nil, events: [note])
        XCTAssertEqual(log.map(\.kind), [.note])
        XCTAssertEqual(log[0].body, "n")
        XCTAssertEqual(log[0].facts, [ThreadFact("source", "Craig")])
        XCTAssertEqual(log[0].when, note.at)
    }

    /// The notes ahead of every other recorded event sit before Launched; a
    /// note after one stays in its place.
    func testLeadingNotesSitBeforeTheLaunch() {
        let log = buildThreadEvents(
            slice: slice(status: "In progress"), agent: nil, brief: nil,
            events: [TaskLogEvent(.note, note: "a"), TaskLogEvent(.handedBack, note: "h"), TaskLogEvent(.note, note: "b")])
        XCTAssertEqual(log.map(\.kind), [.note, .launched, .handedBack, .note])
        XCTAssertEqual(log.map(\.body), ["a", nil, "h", "b"])
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
            .launched, .released, .released, .relaunched, .blocked, .closed, .followUps, .followUp, .followUps,
            .approved,
        ])
        XCTAssertEqual(log[1].title, "Craig released to Todo")
        XCTAssertEqual(log[2].title, "Released to Todo")
        XCTAssertEqual(log[3].title, "Relaunched on the work so far")
        XCTAssertEqual(log[4].title, "Agent blocked")
        XCTAssertEqual(log[4].tone, .hot)
        XCTAssertEqual(log[4].body, "No token.")
        XCTAssertEqual(log[5].body, "Wrote it up.")
        XCTAssertEqual(log[6].meta, "proposed 1 follow-up")
        XCTAssertEqual(log[7].title, "Folded in proposed follow-up")
        XCTAssertEqual(log[7].body, "Later")
        XCTAssertTrue(log[8].awaitsTriage, "a proposal still undecided is the triage card")
        XCTAssertEqual(log[8].tone, .hot)
        XCTAssertEqual(log[9].facts, [])
    }

    func testAReleasedSliceKeepsItsHistory() {
        let log = buildThreadEvents(
            slice: slice(status: "Todo"), agent: nil, brief: nil, events: [TaskLogEvent(.released, by: "Craig")])
        XCTAssertEqual(log.map(\.kind), [.launched, .released])
        XCTAssertTrue(buildThreadEvents(slice: slice(status: "Todo"), agent: nil, brief: nil, events: []).isEmpty)
    }

    func testTheDecisionHeadings() {
        XCTAssertEqual(followUpDecisionHeading(.queued), "Queued")
        XCTAssertEqual(followUpDecisionHeading(.folded), "Folded in")
        XCTAssertEqual(followUpDecisionHeading(.dropped), "Dismissed")
    }

    // MARK: - The label

    func testTheSectionIsTheTaskWhateverItsState() {
        let todo = NavigatorModel(slice: slice(status: "Todo"), agent: nil)
        XCTAssertEqual(todo.threadLabel, "Task")
        let working = NavigatorModel(slice: slice(status: "In progress"), agent: .working)
        XCTAssertEqual(working.threadLabel, "Task")
        let done = NavigatorModel(slice: slice(status: "Done", branch: "b", pr: prURL), agent: nil)
        XCTAssertEqual(done.threadLabel, "Task")
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
