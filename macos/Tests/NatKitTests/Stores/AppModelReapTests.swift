import XCTest
@testable import NatKit
import NatFixtures

/// The reaper verifies and kills a session on its slice's own project, never
/// the active one — asked on another, `nat slice-status` cannot find the
/// slice and answers gone — and touches no session no open plan lists.
@MainActor
final class AppModelReapTests: XCTestCase {
    /// The second project's Done slice: a candidate wherever its plan is open.
    private let doneOnB = "f1x75333-0000-4000-8000-000000000004"
    private let projectB = Fixtures.secondProjectID

    private func agent(_ sliceID: String) -> AgentStatus {
        AgentStatus(sliceID: sliceID, session: TmuxSession.name(forSlicePageID: sliceID), activity: .working)
    }

    /// Both projects open, the fixture's active; the second's plan loaded in
    /// the background, then a sweep run.
    private func swept(_ client: FixtureNatClient) async -> AppModel {
        let model = await Fixtures.startedAppModel(client: client, config: Fixtures.twoProjectConfig)
        XCTAssertEqual(model.activeProjectID, Fixtures.projectID)
        let deadline = ContinuousClock.now + .seconds(10)
        while !model.sidebarModel.projects.flatMap(\.milestones).flatMap(\.slices).contains(where: { $0.sliceID == doneOnB }),
              ContinuousClock.now < deadline {
            try? await Task.sleep(for: .milliseconds(2))
        }
        await model.refresh(.replica)
        return model
    }

    private func kills(_ client: FixtureNatClient) -> [String] {
        client.writes.filter { $0.hasPrefix("agent-kill") }
    }

    func testAnotherProjectsFinishedSessionIsVerifiedAndKilledOnThatProject() async {
        let client = FixtureNatClient(agents: [agent(doneOnB)])
        client.setSliceStatus(.found(status: "Done", trashed: false), forSlice: doneOnB)
        let model = await swept(client)

        XCTAssertFalse(client.sliceStatusReads.isEmpty)
        XCTAssertEqual(Set(client.sliceStatusReads), ["slice-status --project \(projectB) \(doneOnB)"])
        XCTAssertFalse(kills(client).isEmpty)
        XCTAssertEqual(Set(kills(client)), ["agent-kill --project \(projectB) \(doneOnB)"])
        model.cleanup()
    }

    func testAnInProgressAnswerOnItsOwnProjectKillsNothing() async {
        let client = FixtureNatClient(agents: [agent(doneOnB)])
        let model = await swept(client)

        XCTAssertEqual(Set(client.sliceStatusReads), ["slice-status --project \(projectB) \(doneOnB)"])
        XCTAssertEqual(kills(client), [])
        model.cleanup()
    }

    func testASessionInNoOpenPlanIsNeitherVerifiedNorKilled() async {
        let unlisted = "f1x7dead-0000-4000-8000-000000000001"
        let client = FixtureNatClient(agents: [agent(unlisted)])
        client.setSliceStatus(.gone, forSlice: unlisted)
        let model = await swept(client)

        XCTAssertEqual(client.sliceStatusReads, [])
        XCTAssertEqual(kills(client), [])
        model.cleanup()
    }
}
