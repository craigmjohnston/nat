import XCTest
@testable import NatKit

/// A `pr-status` reading of each slice at the given readiness word, its
/// checks' verdict the one that word implies — passing for ready to merge,
/// failing for checks failing, pending otherwise — and `conflicting` the
/// slices named.
func prReading(_ readiness: [String: String], conflicting: Set<String> = []) -> PRReading {
    PRReading(PRStatusDoc(slices: readiness.map { id, word in
        let verdict = word == PRStatusSlice.readyToMerge ? PRStatusSlice.checksPassing
            : word == PRStatusSlice.checksFailing ? "failing" : "pending"
        return PRStatusSlice(
            sliceID: id, name: id, pr: "https://pr/\(id)", readiness: word,
            checks: PRStatusChecks(verdict: verdict), conflicting: conflicting.contains(id), base: "main")
    }))
}

final class ProjectAttentionTests: XCTestCase {
    private func slice(
        _ id: String,
        status: String = "In progress",
        pr: String = "",
        handedBack: Bool = false
    ) -> Slice {
        Slice(
            id: id, name: id, status: status, milestoneID: "m-1",
            assignee: "user", pr: pr, url: "", blocked: false, handedBack: handedBack
        )
    }

    // MARK: - The dot's precedence

    func testWorkingAgentsAndNothingWaiting_isWorkingAndPulses() {
        let attention = projectAttention(
            slices: [slice("s-1"), slice("s-2")],
            liveAgents: ["s-1": .working, "s-2": .working]
        )

        XCTAssertEqual(attention.role, .working)
        XCTAssertNil(attention.badge)
        XCTAssertTrue(attention.pulses)
    }

    func testHandedBackSlice_isReviewAndStillEvenWithAgentsWorking() {
        let attention = projectAttention(
            slices: [slice("s-1", handedBack: true), slice("s-2")],
            liveAgents: ["s-2": .working]
        )

        XCTAssertEqual(attention.role, .review)
        XCTAssertEqual(attention.badge, 1)
        XCTAssertFalse(attention.pulses)
    }

    func testReadyToMergePR_isReview() {
        let attention = projectAttention(
            slices: [slice("s-1", pr: "https://pr/1")],
            liveAgents: [:],
            prReading: prReading(["s-1": PRStatusSlice.readyToMerge])
        )

        XCTAssertEqual(attention.role, .review)
        XCTAssertEqual(attention.badge, 1)
    }

    func testPRMerelyAwaitingReview_countsForNothing() {
        let attention = projectAttention(
            slices: [slice("s-1", status: "Done", pr: "https://pr/1")],
            liveAgents: [:],
            prReading: prReading(["s-1": PRStatusSlice.awaitingReview])
        )

        XCTAssertEqual(attention.role, .idle)
        XCTAssertNil(attention.badge)
    }

    func testWaitingAgent_outranksEverything() {
        let attention = projectAttention(
            slices: [slice("s-1", handedBack: true), slice("s-2"), slice("s-3")],
            liveAgents: ["s-2": .waiting, "s-3": .working],
            planningAgent: nil,
            prReading: prReading(["s-1": PRStatusSlice.readyToMerge])
        )

        XCTAssertEqual(attention.role, .waiting)
        XCTAssertFalse(attention.pulses)
        // The handed-back slice and the waiting agent's own — two things to
        // attend to, and the working one is not one of them.
        XCTAssertEqual(attention.badge, 2)
    }

    func testIdleProject_isNeutralAndSilent() {
        let attention = projectAttention(slices: [slice("s-1", status: "Todo")], liveAgents: [:])

        XCTAssertEqual(attention.role, .idle)
        XCTAssertNil(attention.badge)
        XCTAssertFalse(attention.pulses)
        XCTAssertEqual(attention, .none)
    }

    func testEmptyPlan_isNeutral() {
        XCTAssertEqual(projectAttention(slices: [], liveAgents: [:]), .none)
    }

    // MARK: - The planning agent

    func testWaitingPlanningAgent_isWaitingAndCountsOne() {
        let attention = projectAttention(
            slices: [slice("s-1")],
            liveAgents: ["s-1": .working],
            planningAgent: .waiting
        )

        XCTAssertEqual(attention.role, .waiting)
        XCTAssertEqual(attention.badge, 1)
    }

    func testWorkingPlanningAgentAlone_isWorking() {
        let attention = projectAttention(
            slices: [slice("s-1", status: "Todo")],
            liveAgents: [:],
            planningAgent: .working
        )

        XCTAssertEqual(attention.role, .working)
        XCTAssertNil(attention.badge)
        XCTAssertTrue(attention.pulses)
    }

    // MARK: - The counting rule

    func testAgentsOnAnotherProjectsSlices_areNotRead() {
        let attention = projectAttention(
            slices: [slice("s-1", status: "Todo")],
            liveAgents: ["other-1": .waiting, "other-2": .working]
        )

        XCTAssertEqual(attention, .none)
    }

    func testOneSliceWaitingAndHandedBack_countsOnce() {
        let attention = projectAttention(
            slices: [slice("s-1", handedBack: true)],
            liveAgents: ["s-1": .waiting]
        )

        XCTAssertEqual(attention.badge, 1)
        XCTAssertEqual(attention.role, .waiting)
    }

    func testEverythingAtOnce_countsEachThingOnce() {
        let attention = projectAttention(
            slices: [
                slice("s-1", handedBack: true),
                slice("s-2"),
                slice("s-3", pr: "https://pr/3"),
                slice("s-4")
            ],
            liveAgents: ["s-2": .waiting, "s-4": .working],
            planningAgent: .waiting,
            prReading: prReading(["s-3": PRStatusSlice.readyToMerge])
        )

        // Handed back, a waiting agent, a mergeable pull request and the
        // planning agent — the working slice is not one of them.
        XCTAssertEqual(attention.badge, 4)
        XCTAssertEqual(attention.role, .waiting)
    }

    // MARK: - Only ACTIVE-section work is read

    // A tmux session outlives the slice it was launched on: an idle Claude
    // Code left in the pane of a Done slice whose pull request has merged.
    // The rail's ACTIVE section refuses exactly that, and so must the tab.

    func testWorkingAgentOnADeadSlice_countsForNothing() {
        let attention = projectAttention(
            slices: [slice("s-1", status: "Done", pr: "https://pr/1")],
            liveAgents: ["s-1": .working]
        )

        XCTAssertEqual(attention.role, .idle)
        XCTAssertNil(attention.badge)
        XCTAssertFalse(attention.pulses)
    }

    func testWaitingAgentOnADeadSlice_countsForNothing() {
        let attention = projectAttention(
            slices: [slice("s-1", status: "Done", pr: "https://pr/1")],
            liveAgents: ["s-1": .waiting]
        )

        XCTAssertEqual(attention.role, .idle)
        XCTAssertNil(attention.badge)
    }

    /// A live agent on a Done slice — one marked Done under the old rule,
    /// its pull request not merged yet — is not this count's
    /// either, exactly as it is not `domain.StateOf`'s on the Go side: Notion's
    /// status is read straight, before presence is ever asked about, so a
    /// Done slice contributes nothing here whatever is running on it. The
    /// star drawn on the row itself is what says the session is there; this
    /// count is about what the ACTIVE section holds.
    func testWaitingAgentOnADoneSliceWithAnOpenPR_countsForNothing() {
        let attention = projectAttention(
            slices: [slice("s-1", status: "Done", pr: "https://pr/1")],
            liveAgents: ["s-1": .waiting],
            prReading: prReading(["s-1": PRStatusSlice.awaitingReview])
        )

        XCTAssertEqual(attention.role, .idle)
        XCTAssertNil(attention.badge)
    }

    func testAgentOnAHandedBackSlice_countsExactlyAsBefore() {
        let attention = projectAttention(
            slices: [slice("s-1", handedBack: true)],
            liveAgents: ["s-1": .working]
        )

        XCTAssertEqual(attention.role, .review)
        XCTAssertEqual(attention.badge, 1)
    }

    func testOnlyDeadSliceSessions_readAsIdle() {
        let attention = projectAttention(
            slices: [
                slice("s-1", status: "Done", pr: "https://pr/1"),
                slice("s-2", status: "Todo")
            ],
            liveAgents: ["s-1": .working, "s-2": .working]
        )

        XCTAssertEqual(attention, .none)
    }

    // MARK: - The shared ACTIVE membership rule

    func testInFlightSliceIDs_isTheUnionOfTheTwoHalves() {
        let slices = [
            slice("active"),
            slice("handed-back", handedBack: true),
            slice("open-pr", pr: "https://pr/1"),
            slice("done-with-open-pr", status: "Done", pr: "https://pr/2"),
            slice("todo", status: "Todo"),
            slice("approved-no-reading", pr: "https://pr/3")
        ]

        // "done-with-open-pr" is left out: Notion's status is read straight, and
        // a Done slice is never a review entry until the un-done rule writes it
        // back to In progress. An approved slice is in flight (its `pr` stage)
        // whether or not gh has been read for it.
        XCTAssertEqual(
            inFlightSliceIDs(slices: slices),
            ["active", "handed-back", "open-pr", "approved-no-reading"]
        )
    }

    // MARK: - One pulse rule across the tab and the rail

    func testOnlyWorkingPulses_onTheTab() {
        XCTAssertTrue(ProjectAttention(count: 0, role: .working).pulses)
        for role: ProjectAttentionRole in [.waiting, .review, .idle] {
            XCTAssertFalse(ProjectAttention(count: 1, role: role).pulses, "\(role)")
        }
    }

    func testOnlyWorkingPulses_onARailRow() {
        XCTAssertTrue(ActiveTintRole.working.pulses)
        let still: [ActiveTintRole] = [.waiting, .blocked, .readyToPush, .needsReview, .launching, .new]
        for role in still {
            XCTAssertFalse(role.pulses, "\(role)")
        }
    }

    func testOnlyWorkingPulses_onTheWorkshopRow() {
        XCTAssertEqual(buildWorkshopEntry(activity: .working, isLaunching: false)?.tintRole.pulses, true)
        XCTAssertEqual(buildWorkshopEntry(activity: .waiting, isLaunching: false)?.tintRole.pulses, false)
        XCTAssertEqual(buildWorkshopEntry(activity: nil, isLaunching: true)?.tintRole.pulses, false)
        XCTAssertEqual(
            buildWorkshopEntry(activity: nil, isLaunching: false, isSelected: true)?.tintRole.pulses,
            false
        )
    }

    // MARK: - AgentActivity from the live map's own words

    func testUnknownActivityReadsAsWorking() {
        XCTAssertEqual(AgentActivity(.working), .working)
        XCTAssertEqual(AgentActivity(.waiting), .waiting)
        XCTAssertEqual(AgentActivity(.unknown), .working)
    }

    // MARK: - Ad hoc sessions

    private func session(_ id: String, tag: String, prs: [SessionPR] = []) -> Session {
        Session(id: id, tag: tag, live: false, startedAt: Date(), dir: "/tmp", branch: "session/\(id)", prs: prs)
    }

    func testWaitingSessionAgent_isWaitingAndCounted() {
        let attention = projectAttention(
            slices: [],
            liveAgents: ["session:p:1": .waiting],
            sessions: [session("1", tag: "session:p:1")]
        )

        XCTAssertEqual(attention.role, .waiting)
        XCTAssertEqual(attention.badge, 1)
    }

    func testWorkingSessionAgent_isWorkingWithNoSliceWork() {
        let attention = projectAttention(
            slices: [],
            liveAgents: ["session:p:1": .working],
            sessions: [session("1", tag: "session:p:1")]
        )

        XCTAssertEqual(attention.role, .working)
        XCTAssertTrue(attention.pulses)
        XCTAssertNil(attention.badge, "working alone counts nothing that needs the user")
    }

    func testSessionWithOpenPR_isReviewAndCountedOnce() {
        let attention = projectAttention(
            slices: [],
            liveAgents: [:],
            sessions: [session("1", tag: "session:p:1", prs: [
                SessionPR(number: 1, title: "x", url: "https://x", state: "OPEN"),
                SessionPR(number: 2, title: "y", url: "https://y", state: "OPEN"),
            ])]
        )

        XCTAssertEqual(attention.role, .review)
        XCTAssertEqual(attention.badge, 1, "one session with two open PRs still counts once")
    }

    func testDoneSession_countsNothing() {
        let attention = projectAttention(
            slices: [],
            liveAgents: [:],
            sessions: [session("1", tag: "session:p:1", prs: [
                SessionPR(number: 1, title: "x", url: "https://x", state: "MERGED"),
            ])]
        )

        XCTAssertEqual(attention.role, .idle)
        XCTAssertNil(attention.badge)
    }

    // MARK: - Attention items

    private func items(
        _ slices: [Slice], agents: [String: AgentActivity] = [:], planning: AgentActivity? = nil,
        reading: PRReading = .empty, sessions: [Session] = []
    ) -> [AttentionItem] {
        attentionItems(
            projectID: "p", slices: slices, liveAgents: agents, planningAgent: planning,
            prReading: reading, sessions: sessions)
    }

    private func kinds(_ items: [AttentionItem]) -> [AttentionKind] { items.map(\.kind) }

    func testEveryKind_inOrderOfUrgency_withPlanOrderWithinAKind() {
        let reading = prReading(
            [
                "ready": PRStatusSlice.awaitingReview, "red": PRStatusSlice.checksFailing,
                "clash": PRStatusSlice.awaitingReview, "red-too": PRStatusSlice.checksFailing
            ],
            conflicting: ["clash"])
        // The ready one's checks read passing although nobody approved it:
        // the sidebar's tick is the gate, not GitHub's review.
        let passing = PRReading(PRStatusDoc(
            slices: reading.doc.slices.map {
                $0.sliceID == "ready"
                    ? PRStatusSlice(sliceID: "ready", name: "ready", pr: $0.pr, readiness: $0.readiness,
                                    checks: PRStatusChecks(verdict: PRStatusSlice.checksPassing))
                    : $0
            }))
        let result = items(
            [
                slice("ready", pr: "https://pr/ready"),
                slice("red", pr: "https://pr/red"),
                slice("handed", handedBack: true),
                slice("clash", pr: "https://pr/clash"),
                slice("asking"),
                slice("red-too", pr: "https://pr/red-too")
            ],
            agents: ["asking": .waiting],
            planning: .waiting,
            reading: passing)

        XCTAssertEqual(kinds(result), [.waiting, .waiting, .review, .checksFailed, .checksFailed, .conflict, .readyToMerge])
        XCTAssertEqual(result.map(\.subject), [
            .slice("asking"), .workshop, .slice("handed"), .slice("red"), .slice("red-too"), .slice("clash"),
            .slice("ready")
        ])
        XCTAssertEqual(result.map(\.name).first, "asking")
        XCTAssertEqual(result[1].name, "Workshop")
        XCTAssertTrue(result.allSatisfy { $0.projectID == "p" })
    }

    func testHandedBackSliceWhoseAgentWaits_isOneWaitingItem() {
        let result = items([slice("s-1", handedBack: true)], agents: ["s-1": .waiting])

        XCTAssertEqual(kinds(result), [.waiting])
    }

    func testFailingChecksWithALiveAgent_neverCount() {
        let red = prReading(["s-1": PRStatusSlice.checksFailing])
        // Before the nudge lands: still at the PR stage, its agent working.
        XCTAssertEqual(items([slice("s-1", pr: "https://pr/1")], agents: ["s-1": .working], reading: red), [])
        // After: resumed, working again.
        let resumed = Slice(
            id: "s-1", name: "s-1", status: "In progress", milestoneID: "m-1", assignee: "user",
            pr: "https://pr/1", url: "", blocked: false, handedBack: false, resumed: true)
        XCTAssertEqual(items([resumed], agents: ["s-1": .working], reading: red), [])
        XCTAssertEqual(items([resumed], reading: red), [])
        // Its agent waiting is the agent's question, not the checks.
        XCTAssertEqual(kinds(items([slice("s-1", pr: "https://pr/1")], agents: ["s-1": .waiting], reading: red)), [.waiting])
        // With no agent at all it is the user's.
        XCTAssertEqual(kinds(items([slice("s-1", pr: "https://pr/1")], reading: red)), [.checksFailed])
    }

    func testConflictWithALiveAgent_neverCounts() {
        let clash = prReading(["s-1": PRStatusSlice.awaitingReview], conflicting: ["s-1"])

        XCTAssertEqual(items([slice("s-1", pr: "https://pr/1")], agents: ["s-1": .working], reading: clash), [])
        XCTAssertEqual(kinds(items([slice("s-1", pr: "https://pr/1")], reading: clash)), [.conflict])
    }

    func testReadyToMerge_isTheSidebarsPassingGate() {
        let green = prReading(["s-1": PRStatusSlice.readyToMerge])
        let pr = slice("s-1", pr: "https://pr/1")

        XCTAssertEqual(kinds(items([pr], reading: green)), [.readyToMerge])
        XCTAssertEqual(items([pr], agents: ["s-1": .working], reading: green), [], "an agent working on it")
        XCTAssertEqual(
            kinds(items([pr], reading: prReading(["s-1": PRStatusSlice.readyToMerge], conflicting: ["s-1"]))),
            [.conflict], "a conflicting pull request is a conflict, never ready")
        XCTAssertEqual(items([pr], reading: prReading(["s-1": PRStatusSlice.awaitingReview])), [], "checks pending")
        XCTAssertEqual(items([pr]), [], "nothing read")
    }

    func testDoneSlice_countsForNothing() {
        let done = slice("s-1", status: "Done", pr: "https://pr/1")
        let red = prReading(["s-1": PRStatusSlice.checksFailing], conflicting: ["s-1"])

        XCTAssertEqual(items([done], agents: ["s-1": .waiting], reading: red), [])
    }

    func testSessions_waitingAndReview() {
        let waiting = Session(id: "1", tag: "session:p:1", live: true, startedAt: Date(), dir: "/tmp/one", branch: "")
        let open = Session(
            id: "2", tag: "session:p:2", live: false, startedAt: Date(), dir: "/tmp", branch: "session/2",
            prs: [SessionPR(number: 1, title: "x", url: "https://x", state: "OPEN")])
        let working = Session(id: "3", tag: "session:p:3", live: true, startedAt: Date(), dir: "/tmp", branch: "b")
        let result = items(
            [], agents: ["session:p:1": .waiting, "session:p:3": .working], sessions: [open, waiting, working])

        XCTAssertEqual(kinds(result), [.waiting, .review])
        XCTAssertEqual(result.map(\.subject), [.session("1"), .session("2")])
        XCTAssertEqual(result.map(\.name), ["Ad hoc session one", "Ad hoc session session/2"])
    }

    func testThePillCountsTheItems() {
        let slices = [slice("s-1", handedBack: true), slice("s-2", pr: "https://pr/2"), slice("s-3", pr: "https://pr/3")]
        let red = prReading(["s-2": PRStatusSlice.checksFailing, "s-3": PRStatusSlice.checksFailing])
        let agents: [String: AgentActivity] = ["s-1": .waiting, "s-2": .working]

        let attention = projectAttention(slices: slices, liveAgents: agents, planningAgent: .waiting, prReading: red)
        XCTAssertEqual(
            attention.count,
            items(slices, agents: agents, planning: .waiting, reading: red).count)
        // The handed-back one (waiting), the planning agent and s-3's red
        // checks — s-2's are its live agent's.
        XCTAssertEqual(attention.count, 3)
    }

    /// A working agent on a red pull request used to count on the pill; it
    /// is the agent's now, and the dot says working.
    func testRedPullRequestWithAWorkingAgent_readsWorking() {
        let attention = projectAttention(
            slices: [slice("s-1", pr: "https://pr/1")], liveAgents: ["s-1": .working],
            prReading: prReading(["s-1": PRStatusSlice.checksFailing]))

        XCTAssertEqual(attention, ProjectAttention(count: 0, role: .working))
    }

    // MARK: - Arrivals

    private func item(_ id: String, _ kind: AttentionKind = .review, project: String = "p") -> AttentionItem {
        AttentionItem(kind: kind, subject: .slice(id), name: id, projectID: project)
    }

    func testArrivals_areTheNewItems() {
        XCTAssertEqual(AttentionChange.arrivals(from: [item("a")], to: [item("a"), item("b")]), [item("b")])
        XCTAssertEqual(AttentionChange.arrivals(from: [], to: [item("a")]), [item("a")])
    }

    func testDepartures_arriveNothing() {
        XCTAssertEqual(AttentionChange.arrivals(from: [item("a"), item("b")], to: [item("a")]), [])
        XCTAssertEqual(AttentionChange.arrivals(from: [item("a")], to: []), [])
    }

    func testASwap_arrivesTheNewcomer() {
        XCTAssertEqual(AttentionChange.arrivals(from: [item("a")], to: [item("b")]), [item("b")])
    }

    func testAReorderOrARename_arrivesNothing() {
        XCTAssertEqual(AttentionChange.arrivals(from: [item("a"), item("b")], to: [item("b"), item("a")]), [])
        let renamed = AttentionItem(kind: .review, subject: .slice("a"), name: "A, renamed", projectID: "p")
        XCTAssertEqual(AttentionChange.arrivals(from: [item("a")], to: [renamed]), [])
    }

    func testANewKindOrProject_isAnArrival() {
        XCTAssertEqual(AttentionChange.arrivals(from: [item("a")], to: [item("a", .readyToMerge)]), [item("a", .readyToMerge)])
        let workshop = AttentionItem(kind: .waiting, subject: .workshop, name: "Workshop", projectID: "q")
        let elsewhere = AttentionItem(kind: .waiting, subject: .workshop, name: "Workshop", projectID: "r")
        XCTAssertEqual(AttentionChange.arrivals(from: [workshop], to: [workshop, elsewhere]), [elsewhere])
    }

    // MARK: - The dock menu

    func testDockMenuSections_groupByKindWithTaggedRows() {
        let sections = dockMenuSections(
            [item("one", .waiting, project: "p"), item("two", .waiting, project: "q"), item("three", .readyToMerge)],
            tags: ["p": "NAT", "q": ""])

        XCTAssertEqual(sections.map(\.heading), ["Waiting for input", "Ready to merge"])
        XCTAssertEqual(sections.map { $0.rows.map(\.title) }, [["NAT · one", "two"], ["NAT · three"]])
        XCTAssertEqual(sections[0].rows[0].tag, "NAT")
        XCTAssertEqual(sections[0].rows[1].item, item("two", .waiting, project: "q"))
        XCTAssertEqual(dockMenuSections([], tags: [:]), [])
    }

    func testALongName_isCutWithAnEllipsis() {
        let long = String(repeating: "word ", count: 20)
        let row = dockMenuSections([item(long, .review)], tags: ["p": "NAT"])[0].rows[0]

        XCTAssertEqual(row.name.count, dockMenuNameLimit)
        XCTAssertEqual(row.name, String(long.prefix(dockMenuNameLimit - 1)) + "…")
        XCTAssertEqual(row.title, "NAT · " + row.name)
        XCTAssertEqual(truncated(String(repeating: "x", count: dockMenuNameLimit), to: dockMenuNameLimit).count, dockMenuNameLimit)
        XCTAssertEqual(truncated("ab cd", to: 4), "ab…", "no space left before the ellipsis")
    }

    func testEveryKindHasItsHeading() {
        XCTAssertEqual(
            AttentionKind.allCases.map(\.heading),
            ["Waiting for input", "Handed back for review", "Checks failed", "Conflicts", "Ready to merge"])
    }
}
