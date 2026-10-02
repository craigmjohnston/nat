import XCTest
@testable import NatKit
import NatFixtures

@MainActor
final class FollowUpStoreTests: XCTestCase {
    private let followUps = Fixtures.proposedFollowUps
    private let sliceID = Fixtures.activitySliceID

    // MARK: - Apply rule

    func testApplyNeedsEveryRowDecided() {
        XCTAssertFalse(FollowUpStore.canApply(followUps: followUps, choices: [:], hasLiveAgent: true))
        XCTAssertFalse(FollowUpStore.canApply(followUps: followUps, choices: [1: .queue, 2: .drop], hasLiveAgent: true))
        XCTAssertTrue(FollowUpStore.canApply(followUps: followUps, choices: [1: .queue, 2: .fold, 3: .drop], hasLiveAgent: true))
    }

    func testFoldingInNeedsALiveAgent() {
        XCTAssertFalse(FollowUpStore.canApply(followUps: followUps, choices: [1: .queue, 2: .fold, 3: .drop], hasLiveAgent: false))
        XCTAssertTrue(FollowUpStore.canApply(followUps: followUps, choices: [1: .queue, 2: .drop, 3: .drop], hasLiveAgent: false))
    }

    func testNothingToApplyWithNoFollowUps() {
        XCTAssertFalse(FollowUpStore.canApply(followUps: [], choices: [:], hasLiveAgent: true))
    }

    // MARK: - Foot text

    func testSummarySaysWhatApplyWillDo() {
        XCTAssertEqual(
            FollowUpStore.summary(followUps: followUps, choices: [1: .queue, 2: .fold, 3: .drop], milestone: "M53"),
            "Apply queues 1 task under M53, folds 1 into this task and drops 1.")
        XCTAssertEqual(
            FollowUpStore.summary(followUps: followUps, choices: [1: .queue, 2: .queue, 3: .queue], milestone: ""),
            "Apply queues 3 tasks.")
        XCTAssertEqual(
            FollowUpStore.summary(followUps: followUps, choices: [1: .drop, 2: .fold, 3: .drop], milestone: "M"),
            "Apply folds 1 into this task and drops 2.")
        XCTAssertEqual(
            FollowUpStore.summary(followUps: followUps, choices: [1: .queue], milestone: "M"),
            "Decide every follow-up to apply, or discard them all.")
    }

    func testApplyingSummary() {
        XCTAssertEqual(FollowUpStore.applyingSummary(choices: [1: .queue, 2: .fold, 3: .drop]), "Queued 1 · sending 1 to the agent…")
        XCTAssertEqual(FollowUpStore.applyingSummary(choices: [1: .queue]), "Queued 1…")
        XCTAssertEqual(FollowUpStore.applyingSummary(choices: [1: .fold]), "sending 1 to the agent…")
        XCTAssertEqual(FollowUpStore.applyingSummary(choices: [1: .drop]), "Recording…")
    }

    // MARK: - Choices and apply

    func testChoicesAreKeptPerSlice() {
        let store = FollowUpStore()
        store.setChoice(.queue, sliceID: "a", index: 1)
        store.setChoice(.drop, sliceID: "b", index: 1)
        XCTAssertEqual(store.choice(sliceID: "a", index: 1), .queue)
        XCTAssertEqual(store.choice(sliceID: "b", index: 1), .drop)
        store.setChoice(nil, sliceID: "a", index: 1)
        XCTAssertNil(store.choice(sliceID: "a", index: 1))
    }

    func testApplySendsEachIndexUnderItsChoiceAndClearsThem() async {
        let client = FixtureNatClient(details: Fixtures.followUpsSliceDetails)
        let store = FollowUpStore()
        store.setChoice(.queue, sliceID: sliceID, index: 1)
        store.setChoice(.fold, sliceID: sliceID, index: 2)
        store.setChoice(.drop, sliceID: sliceID, index: 3)

        let result = await store.apply(projectID: "p", sliceID: sliceID, followUps: followUps, client: client)

        XCTAssertEqual(client.writes, ["slice-triage \(sliceID) queue=[1] fold=[2] drop=[3]"])
        XCTAssertEqual(result?.queued.map(\.title), [followUps[0].title])
        XCTAssertEqual(result?.folded, [followUps[1].title])
        XCTAssertEqual(result?.dropped, [followUps[2].title])
        XCTAssertTrue(store.choices(sliceID: sliceID).isEmpty)
        XCTAssertFalse(store.isApplying(sliceID: sliceID))
    }

    func testARefusalKeepsTheChoicesAndSaysWhy() async {
        let client = FixtureNatClient(behaviour: .refusing("no live agent"))
        let store = FollowUpStore()
        store.setChoice(.fold, sliceID: sliceID, index: 1)

        let result = await store.apply(projectID: "p", sliceID: sliceID, followUps: followUps, client: client)

        XCTAssertNil(result)
        XCTAssertEqual(store.choice(sliceID: sliceID, index: 1), .fold)
        XCTAssertNotNil(store.error(sliceID: sliceID))
        // A new choice clears the error.
        store.setChoice(.queue, sliceID: sliceID, index: 1)
        XCTAssertNil(store.error(sliceID: sliceID))
    }

    func testDiscardAllDropsEverything() async {
        let client = FixtureNatClient(details: Fixtures.followUpsSliceDetails)
        let store = FollowUpStore()
        let result = await store.discardAll(projectID: "p", sliceID: sliceID, client: client)
        XCTAssertEqual(client.writes, ["slice-triage \(sliceID) --drop-all"])
        XCTAssertEqual(result?.dropped.count, 3)
    }

    func testAnApplyInFlightIsNotStartedTwice() async {
        let client = FixtureNatClient(details: Fixtures.followUpsSliceDetails)
        let store = FollowUpStore()
        store.markApplying(sliceID: sliceID)
        let result = await store.discardAll(projectID: "p", sliceID: sliceID, client: client)
        XCTAssertNil(result)
        XCTAssertTrue(client.writes.isEmpty)
    }

    // MARK: - Wire shapes

    func testSliceDetailDecodesFollowUpsAndTheirAbsence() throws {
        let base = #""id":"s","name":"n","url":"u","status":"In progress","milestone":"M","assignee":"a","blocked":false,"handed_back":false,"brief":"b""#
        let none = try JSONDecoder().decode(SliceDetail.self, from: Data("{\(base)}".utf8))
        XCTAssertEqual(none.followUps, [])
        let some = try JSONDecoder().decode(SliceDetail.self, from: Data(
            "{\(base),\"followUps\":[{\"index\":3,\"title\":\"T\",\"brief\":\"B\"}]}".utf8))
        XCTAssertEqual(some.followUps, [FollowUp(index: 3, title: "T", brief: "B")])
    }

    func testTriageArgumentsRepeatEachFlag() {
        XCTAssertEqual(
            NatClient.triageArguments(projectID: "p", sliceRef: "s", queue: [1, 4], fold: [2], drop: [3]),
            ["slice-triage", "--project", "p", "--json", "--queue", "1", "--queue", "4", "--fold", "2", "--drop", "3", "s"])
    }

    func testTriageResultDecodes() throws {
        let json = #"{"queued":[{"title":"A","id":"i","url":"u"}],"folded":["B"],"dropped":[]}"#
        let result = try JSONDecoder().decode(TriageResult.self, from: Data(json.utf8))
        XCTAssertEqual(result, TriageResult(queued: [.init(title: "A", id: "i", url: "u")], folded: ["B"], dropped: []))
    }

    // MARK: - Rail

    func testTheRailRowCountsPendingFollowUps() {
        let model = buildRailModel(
            from: Fixtures.projectInfo, liveAgents: Fixtures.liveAgents,
            followUpCounts: [Fixtures.activitySliceID: 3])
        let entry = model.active.first { $0.sliceID == Fixtures.activitySliceID }
        XCTAssertEqual(entry?.meta, "3 follow-ups")
        XCTAssertEqual(entry?.metaRole, .followUps)

        let one = buildRailModel(
            from: Fixtures.projectInfo, liveAgents: Fixtures.liveAgents,
            followUpCounts: [Fixtures.activitySliceID: 1])
        XCTAssertEqual(one.active.first { $0.sliceID == Fixtures.activitySliceID }?.meta, "1 follow-up")
    }
}
