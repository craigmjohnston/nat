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

    func testChoicesAreKeptPerSliceAndBatch() {
        let store = FollowUpStore()
        store.setChoice(.queue, sliceID: "a", batch: 1, position: 1)
        store.setChoice(.drop, sliceID: "a", batch: 2, position: 1)
        store.setChoice(.fold, sliceID: "b", batch: 1, position: 1)
        XCTAssertEqual(store.choice(sliceID: "a", batch: 1, position: 1), .queue)
        XCTAssertEqual(store.choice(sliceID: "a", batch: 2, position: 1), .drop)
        XCTAssertEqual(store.choice(sliceID: "b", batch: 1, position: 1), .fold)
        XCTAssertEqual(store.choices(sliceID: "a", batch: 2), [1: .drop])
        store.setChoice(nil, sliceID: "a", batch: 1, position: 1)
        XCTAssertNil(store.choice(sliceID: "a", batch: 1, position: 1))
        XCTAssertEqual(store.choice(sliceID: "a", batch: 2, position: 1), .drop)
    }

    func testApplySendsEachIndexUnderItsChoiceAndClearsThem() async {
        let client = FixtureNatClient(details: Fixtures.followUpsSliceDetails)
        let store = FollowUpStore()
        store.setChoice(.queue, sliceID: sliceID, batch: 1, position: 1)
        store.setChoice(.fold, sliceID: sliceID, batch: 1, position: 2)
        store.setChoice(.drop, sliceID: sliceID, batch: 1, position: 3)

        let result = await store.apply(projectID: "p", sliceID: sliceID, batch: 1, followUps: followUps, client: client)

        XCTAssertEqual(client.writes, ["slice-triage \(sliceID) queue=[1] fold=[2] drop=[3]"])
        XCTAssertEqual(result?.queued.map(\.title), [followUps[0].title])
        XCTAssertEqual(result?.folded, [followUps[1].title])
        XCTAssertEqual(result?.dropped, [followUps[2].title])
        XCTAssertTrue(store.choices(sliceID: sliceID, batch: 1).isEmpty)
        XCTAssertFalse(store.isApplying(sliceID: sliceID, batch: 1))
    }

    /// A second batch's choices are by place in it, and go to nat as the
    /// indexes its follow-ups carry, which count on from the first batch's;
    /// applying it leaves the first batch's choices alone.
    func testApplyingOneBatchSendsItsOwnIndexesAndLeavesTheOther() async {
        let detail = Fixtures.twoBatchesSliceDetail
        let client = FixtureNatClient(details: [sliceID: detail])
        let store = FollowUpStore()
        store.setChoice(.queue, sliceID: sliceID, batch: 1, position: 1)
        store.setChoice(.drop, sliceID: sliceID, batch: 2, position: 1)
        store.setChoice(.queue, sliceID: sliceID, batch: 2, position: 2)

        let second = pendingFollowUps(batch: 2, in: detail.followUps)
        let result = await store.apply(projectID: "p", sliceID: sliceID, batch: 2, followUps: second, client: client)

        XCTAssertEqual(client.writes, ["slice-triage \(sliceID) queue=[5] fold=[] drop=[4]"])
        XCTAssertEqual(result?.dropped, [second[0].title])
        XCTAssertTrue(store.choices(sliceID: sliceID, batch: 2).isEmpty)
        XCTAssertEqual(store.choices(sliceID: sliceID, batch: 1), [1: .queue])
    }

    func testARefusalKeepsTheChoicesAndSaysWhyForItsBatchAlone() async {
        let client = FixtureNatClient(behaviour: .refusing("no live agent"))
        let store = FollowUpStore()
        store.setChoice(.fold, sliceID: sliceID, batch: 1, position: 1)

        let result = await store.apply(projectID: "p", sliceID: sliceID, batch: 1, followUps: followUps, client: client)

        XCTAssertNil(result)
        XCTAssertEqual(store.choice(sliceID: sliceID, batch: 1, position: 1), .fold)
        XCTAssertNotNil(store.error(sliceID: sliceID, batch: 1))
        XCTAssertNil(store.error(sliceID: sliceID, batch: 2))
        // A new choice clears the error.
        store.setChoice(.queue, sliceID: sliceID, batch: 1, position: 1)
        XCTAssertNil(store.error(sliceID: sliceID, batch: 1))
    }

    /// Discarding a batch drops its own follow-ups by index, never `--drop-all`,
    /// which would take every other batch with it.
    func testDiscardDropsTheBatchAlone() async {
        let detail = Fixtures.twoBatchesSliceDetail
        let client = FixtureNatClient(details: [sliceID: detail])
        let store = FollowUpStore()
        let result = await store.discard(
            projectID: "p", sliceID: sliceID, batch: 2,
            followUps: pendingFollowUps(batch: 2, in: detail.followUps), client: client)
        XCTAssertEqual(client.writes, ["slice-triage \(sliceID) queue=[] fold=[] drop=[4, 5]"])
        XCTAssertEqual(result?.dropped.count, 2)
    }

    /// An apply in flight on any batch of the slice holds every other batch's
    /// — their indexes are stale until the slice is read again — but not
    /// another slice's.
    func testAnApplyInFlightHoldsEveryBatchOfTheSlice() async {
        let client = FixtureNatClient(details: Fixtures.followUpsSliceDetails)
        let store = FollowUpStore()
        store.markApplying(sliceID: sliceID, batch: 1)
        XCTAssertTrue(store.isApplying(sliceID: sliceID))
        XCTAssertTrue(store.isApplying(sliceID: sliceID, batch: 1))
        XCTAssertFalse(store.isApplying(sliceID: sliceID, batch: 2))
        XCTAssertFalse(store.isApplying(sliceID: "other"))
        let result = await store.discard(projectID: "p", sliceID: sliceID, batch: 2, followUps: followUps, client: client)
        XCTAssertNil(result)
        XCTAssertTrue(client.writes.isEmpty)
    }

    /// What runs after the triage — the app's re-read — runs while the apply
    /// still holds the slice, so no other card can apply off the old reading.
    func testThenRunsWhileTheApplyStillHoldsTheSlice() async {
        let client = FixtureNatClient(details: Fixtures.followUpsSliceDetails)
        let store = FollowUpStore()
        var heldDuringThen = false
        await store.discard(projectID: "p", sliceID: sliceID, batch: 1, followUps: followUps, client: client) {
            heldDuringThen = store.isApplying(sliceID: self.sliceID)
        }
        XCTAssertTrue(heldDuringThen)
        XCTAssertFalse(store.isApplying(sliceID: sliceID))
    }

    // MARK: - Pairing a proposal with its own items

    func testATriageCardDrawsItsOwnBatchAlone() {
        let pending = Fixtures.twoBatchesSliceDetail.followUps
        XCTAssertEqual(pendingFollowUps(batch: 1, in: pending).map(\.index), [1, 2, 3])
        XCTAssertEqual(pendingFollowUps(batch: 2, in: pending).map(\.index), [4, 5])
        XCTAssertEqual(pendingFollowUps(batch: 3, in: pending), [])
        XCTAssertEqual(pendingFollowUps(batch: nil, in: pending), pending)
    }

    // MARK: - Wire shapes

    func testSliceDetailDecodesFollowUpsAndTheirAbsence() throws {
        let base = #""id":"s","name":"n","url":"u","status":"In progress","milestone":"M","assignee":"a","blocked":false,"handed_back":false,"brief":"b""#
        let none = try JSONDecoder().decode(SliceDetail.self, from: Data("{\(base)}".utf8))
        XCTAssertEqual(none.followUps, [])
        let some = try JSONDecoder().decode(SliceDetail.self, from: Data(
            "{\(base),\"followUps\":[{\"batch\":2,\"index\":3,\"title\":\"T\",\"brief\":\"B\"}]}".utf8))
        XCTAssertEqual(some.followUps, [FollowUp(batch: 2, index: 3, title: "T", brief: "B")])
        // An older nat names no batch: batch 0, as its proposal's event reads.
        let unbatched = try JSONDecoder().decode(SliceDetail.self, from: Data(
            "{\(base),\"followUps\":[{\"index\":3,\"title\":\"T\",\"brief\":\"B\"}]}".utf8))
        XCTAssertEqual(unbatched.followUps.first?.batch, 0)
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
