import XCTest
@testable import NatKit

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
            slices: [slice("s-1", status: "Done", pr: "https://pr/1")],
            liveAgents: [:],
            prReadiness: ["s-1": PRStatusSlice.readyToMerge]
        )

        XCTAssertEqual(attention.role, .review)
        XCTAssertEqual(attention.badge, 1)
    }

    func testPRMerelyAwaitingReview_countsForNothing() {
        let attention = projectAttention(
            slices: [slice("s-1", status: "Done", pr: "https://pr/1")],
            liveAgents: [:],
            prReadiness: ["s-1": PRStatusSlice.awaitingReview]
        )

        XCTAssertEqual(attention.role, .idle)
        XCTAssertNil(attention.badge)
    }

    func testWaitingAgent_outranksEverything() {
        let attention = projectAttention(
            slices: [slice("s-1", handedBack: true), slice("s-2"), slice("s-3")],
            liveAgents: ["s-2": .waiting, "s-3": .working],
            planningAgent: nil,
            prReadiness: ["s-1": PRStatusSlice.readyToMerge]
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
                slice("s-3", status: "Done", pr: "https://pr/3"),
                slice("s-4")
            ],
            liveAgents: ["s-2": .waiting, "s-4": .working],
            planningAgent: .waiting,
            prReadiness: ["s-3": PRStatusSlice.readyToMerge]
        )

        // Handed back, a waiting agent, a mergeable pull request and the
        // planning agent — the working slice is not one of them.
        XCTAssertEqual(attention.badge, 4)
        XCTAssertEqual(attention.role, .waiting)
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
}
