import XCTest
@testable import NatKit
@testable import NatFixtures

/// The fixtures are values, so what there is to test is that they are the
/// values they claim to be: a rail with every section filled, a diff whose
/// comments still anchor, pull requests that reach the verdicts they are
/// named for. A fixture that has quietly stopped saying what its name says is
/// worse than no fixture at all — a preview would show the wrong thing and
/// nothing would fail.
final class FixturesTests: XCTestCase {

    // MARK: - The plan

    func testPlanHoldsEveryStatusTheBoardDraws() {
        let statuses = Set(Fixtures.slices.map(\.status))
        XCTAssertEqual(statuses, ["Todo", "In progress", "Done"])
        XCTAssertTrue(Fixtures.slices.contains { $0.blocked })
        XCTAssertTrue(Fixtures.slices.contains { $0.handedBack })
        XCTAssertTrue(Fixtures.slices.contains { !$0.pr.isEmpty })
        XCTAssertEqual(Fixtures.projectInfo.slices.count, Fixtures.slices.count)
        XCTAssertEqual(Fixtures.projectInfo.project.id, Fixtures.projectID)
    }

    func testEveryDependencyNamesASliceThePlanHolds() {
        let ids = Set(Fixtures.slices.map(\.id))
        for slice in Fixtures.slices {
            for dependency in slice.dependsOn ?? [] {
                XCTAssertTrue(ids.contains(dependency), "\(slice.name) waits on an unknown slice")
            }
        }
    }

    func testEverySliceIsFiledUnderAMilestoneThePlanHolds() {
        let milestoneIDs = Set(Fixtures.milestones.map(\.id))
        for slice in Fixtures.slices {
            XCTAssertTrue(milestoneIDs.contains(slice.milestoneID), "\(slice.name) is filed nowhere")
        }
        XCTAssertEqual(Fixtures.milestones.map(\.order), [0, 1, 2])
    }

    func testEmptyPlanIsEmpty() {
        XCTAssertTrue(Fixtures.emptyProjectInfo.slices.isEmpty)
        XCTAssertTrue(Fixtures.emptyProjectInfo.milestones.isEmpty)
    }

    // MARK: - The rail

    func testRailModelFillsEverySection() {
        let rail = Fixtures.railModel

        // The review entries: the handed-back branch with its tally, and the
        // Done slice still waiting on its merge with the reading's own words.
        let review = Dictionary(
            uniqueKeysWithValues: rail.active.filter { $0.tintRole == .needsReview }.map { ($0.sliceID, $0) })
        XCTAssertEqual(review.count, 2)
        XCTAssertEqual(review[Fixtures.mergeBoxSliceID]?.meta, Fixtures.reviewStats[Fixtures.mergeBoxSliceID])
        XCTAssertEqual(review[Fixtures.mergeBoxSliceID]?.detail.last, "4 files")
        XCTAssertEqual(review[Fixtures.approveSliceID]?.meta, "ready to merge")

        // ACTIVE: every reading a row can be in, the review entries included.
        XCTAssertEqual(
            Set(rail.active.map(\.tintRole)), [.needsReview, .working, .waiting, .blocked, .readyToPush])
        let working = rail.active.first { $0.sliceID == Fixtures.diffPaneSliceID }
        XCTAssertEqual(working?.displayState, "Working")
        XCTAssertEqual(working?.meta, "1h 14m")
        XCTAssertEqual(rail.active.first { $0.sliceID == Fixtures.activitySliceID }?.meta, "6m")

        // The folders, and the blocked row inside one of them.
        XCTAssertFalse(rail.todoFolders.isEmpty)
        XCTAssertFalse(rail.doneFolders.isEmpty)
        XCTAssertNotNil(rail.doneSummary)
        let todoRows = rail.todoFolders.flatMap(\.slices)
        XCTAssertTrue(todoRows.contains { $0.glyph == .blocked && $0.isBlocked })
        XCTAssertTrue(rail.doneFolders.contains { $0.isComplete })
    }

    func testEmptyRailDrawsNothing() {
        let rail = Fixtures.emptyRailModel
        XCTAssertTrue(rail.active.isEmpty)
        XCTAssertTrue(rail.todoFolders.isEmpty)
        XCTAssertTrue(rail.doneFolders.isEmpty)
        XCTAssertNil(rail.doneSummary)
    }

    func testUnreadRailKeepsTheMergedSliceOutOfReview() {
        let rail = Fixtures.unreadRailModel
        XCTAssertEqual(
            rail.active.filter { $0.tintRole == .needsReview }.map(\.sliceID), [Fixtures.mergeBoxSliceID])
        // With no live reading, no worked row is working or waiting.
        XCTAssertTrue(rail.active.allSatisfy {
            $0.tintRole == .blocked || $0.tintRole == .readyToPush || $0.tintRole == .needsReview
        })
        XCTAssertTrue(rail.active.allSatisfy { $0.metaRole == .elapsed ? $0.meta == nil : true })
    }

    func testWorkshopEntryIsALivePlanningAgent() {
        XCTAssertEqual(Fixtures.workshopEntry?.kind, .workshop)
        XCTAssertEqual(Fixtures.workshopEntry?.name, "Workshop the plan")
        XCTAssertEqual(Fixtures.workshopEntry?.displayState, "Working")
        XCTAssertEqual(Fixtures.workshopEntry?.tintRole, .working)
        XCTAssertEqual(Fixtures.workshopEntry?.detail, ["Planning agent"])
        XCTAssertEqual(Fixtures.workshopEntry?.meta, "12m")
    }

    /// The acceptance state: one section, the workshop at its head and the
    /// branches awaiting review above the slices being worked.
    func testWorkshopRailLeadsWithTheWorkshopThenTheReviews() {
        let entries = Fixtures.workshopRailModel.active
        XCTAssertEqual(entries.first?.kind, .workshop)
        XCTAssertEqual(entries.first?.id, workshopEntryID)
        let roles = entries.dropFirst().map(\.tintRole)
        XCTAssertEqual(Array(roles.prefix(2)), [.needsReview, .needsReview])
        XCTAssertFalse(roles.dropFirst(2).contains(.needsReview))
    }

    func testAgentReadingsAgreeWithEachOther() {
        XCTAssertEqual(Fixtures.agentStatuses.count, Fixtures.liveAgents.count)
        for status in Fixtures.agentStatuses {
            XCTAssertNotNil(Fixtures.liveAgents[status.sliceID])
            XCTAssertNotNil(Fixtures.agentStarts[status.sliceID])
            XCTAssertTrue(status.session.hasPrefix(TmuxSession.prefix))
        }
        // The readiness reading and the PR-status document say the same thing.
        let open = Fixtures.prStatusDoc.slices.filter { $0.readiness != "unread" }
        XCTAssertEqual(Set(open.map(\.sliceID)), Set(Fixtures.prReadiness.keys))
    }

    // MARK: - Load states

    func testPlanLoadStates() {
        XCTAssertTrue(Fixtures.loadStateLoading.isLoading)
        XCTAssertNil(Fixtures.loadStateIdle.projectInfo)
        XCTAssertEqual(Fixtures.loadStateLoaded.projectInfo, Fixtures.projectInfo)
        XCTAssertEqual(Fixtures.loadStateEmpty.projectInfo, Fixtures.emptyProjectInfo)
        XCTAssertEqual(Fixtures.loadStateFailed.errorMessage, Fixtures.loadErrorMessage)
        XCTAssertNil(Fixtures.loadStateFailed.projectInfo)
        XCTAssertEqual(Fixtures.loadStateStale.projectInfo, Fixtures.projectInfo)
    }

    // MARK: - The diff

    func testDiffLineRunsMeasureTheirOwnText() {
        for line in Fixtures.mergeBoxLines + Fixtures.mergeBoxViewLines + Fixtures.mergeRefusalLines {
            let content = line.mark == nil ? line.text : String(line.text.dropFirst())
            XCTAssertEqual(
                line.runs.reduce(0) { $0 + $1.length },
                content.utf8.count,
                "runs do not measure: \(line.text)"
            )
        }
    }

    func testDiffModelHasEveryRowKindAViewerDraws() {
        let diff = Fixtures.diffModel
        XCTAssertEqual(diff.base, Fixtures.diffBase)
        XCTAssertEqual(diff.branch, Fixtures.diffBranch)
        XCTAssertEqual(diff.files.count, 4)

        let kinds = Set(diff.files.flatMap { $0.rows.map(\.kind) })
        XCTAssertEqual(kinds, [.context, .added, .removed, .hunkBreak, .described])

        // The described file kept git's own words and nothing else.
        let described = diff.files.first { $0.described }
        XCTAssertNotNil(described)
        XCTAssertTrue(described?.rows.allSatisfy { $0.kind == .described } ?? false)

        // The headers git wrote about a file are drawn as no row at all.
        XCTAssertFalse(diff.files.contains { $0.rows.contains { $0.text.hasPrefix("diff --git ") } })
        XCTAssertFalse(diff.files.contains { $0.rows.contains { $0.text.hasPrefix("index ") } })

        // Two languages, and every syntax-highlighted row keeps its runs.
        XCTAssertEqual(Set(Fixtures.sliceDiff.files.map(\.language)), ["Swift", "Go", ""])
        XCTAssertGreaterThan(diff.numberWidth, 0)
    }

    func testSmallAndEmptyDiffs() {
        XCTAssertEqual(Fixtures.smallDiffModel.files.count, 1)
        XCTAssertEqual(Fixtures.smallDiffModel.files[0].path, "internal/actions/mergerefusal.go")
        XCTAssertTrue(Fixtures.emptyDiffModel.files.isEmpty)
        XCTAssertEqual(Fixtures.emptyDiffModel.numberWidth, 1)
    }

    func testPendingCommentsStillAnchorOntoTheDiff() {
        let diff = Fixtures.diffModel
        let comments = Fixtures.pendingComments
        XCTAssertEqual(comments.count, 2)
        XCTAssertEqual(comments.map(\.anchorRowIDs.count), [1, 3])

        let rowIDs = Set(diff.files.flatMap { $0.rows.map(\.id) })
        for comment in comments {
            XCTAssertFalse(comment.anchorRowIDs.isEmpty)
            for id in comment.anchorRowIDs {
                XCTAssertTrue(rowIDs.contains(id), "comment anchored to a row the diff no longer has")
            }
        }

        // A comment that resolves is named by its lines; the "N lines"
        // wording is the fallback for one that does not, so its absence is
        // what says every anchor found its row.
        let prompt = commentsPrompt(comments, diff: diff)
        XCTAssertTrue(prompt.contains("line "))
        XCTAssertFalse(prompt.contains(" lines\n"))
        XCTAssertTrue(prompt.contains("> +"))
    }

    func testCommitsAreTheBranchesOwn() {
        XCTAssertEqual(Fixtures.commitsDoc.branch, Fixtures.diffBranch)
        XCTAssertEqual(Fixtures.commitsDoc.commits, Fixtures.commits)
        XCTAssertEqual(Fixtures.commits.count, 3)
        XCTAssertEqual(Set(Fixtures.commits.map(\.shortSHA.count)), [8])
    }

    func testDiffLoadStates() {
        XCTAssertTrue(Fixtures.diffStateLoading.isLoading)
        XCTAssertNil(Fixtures.diffStateIdle.diff)
        XCTAssertEqual(Fixtures.diffStateLoaded.diff, Fixtures.diffModel)
        XCTAssertEqual(Fixtures.diffStateEmpty.diff?.files.count, 0)
        XCTAssertNil(Fixtures.diffStateFailed.diff)
        XCTAssertEqual(Fixtures.diffStateFailed.errorMessage, Fixtures.diffErrorMessage)
        XCTAssertEqual(Fixtures.diffStateStale.diff, Fixtures.diffModel)
    }

    // MARK: - The pull requests

    func testGreenPullRequestReachesTheYes() {
        XCTAssertNil(mergeRefusal(Fixtures.prGreen))
        guard case .verdicts(let heading, let verdicts) = mergeBoxState(for: Fixtures.prGreen) else {
            return XCTFail("a green pull request is still being weighed up")
        }
        XCTAssertEqual(heading.words, "ready to merge")
        XCTAssertTrue(verdicts.allSatisfy { $0.outcome == .passing || $0.outcome == .skipped })
        XCTAssertEqual(approvedBy(reviews: Fixtures.prGreen.reviews), "craigmjohnston")
        XCTAssertFalse(conversation(comments: Fixtures.prGreen.comments, reviews: Fixtures.prGreen.reviews).isEmpty)
    }

    func testFailingChecksRefuseWithTheReviewFirst() {
        XCTAssertEqual(mergeRefusal(Fixtures.prFailingChecks), "review: changes requested")
        let rollup = checkRollup(Fixtures.prFailingChecks.checks)
        XCTAssertEqual(rollup.outcome, .failing)
        XCTAssertEqual(rollup.summary, "1 failing · 1 pending · 1 passing · 1 skipped")
    }

    func testConflictingRefusesOnTheBranch() {
        XCTAssertEqual(mergeRefusal(Fixtures.prConflicting), "mergeable: conflicting with main")
    }

    func testDraftAndMergedAreTheEndsOfTheRange() {
        XCTAssertEqual(prStateChip(state: Fixtures.prDraft.state, isDraft: true, on: .window).label, "draft")
        XCTAssertTrue(Fixtures.prDraft.checks.isEmpty)
        XCTAssertTrue(Fixtures.prDraft.comments.isEmpty)

        guard case .ended(let words, _) = mergeBoxState(for: Fixtures.prMerged) else {
            return XCTFail("a merged pull request is not still being weighed up")
        }
        XCTAssertEqual(words, "merged into main")
    }

    func testPRLoadStates() {
        XCTAssertTrue(Fixtures.prStateLoading.isLoading)
        XCTAssertNil(Fixtures.prStateIdle.pr)
        XCTAssertEqual(Fixtures.prStateLoaded.pr, Fixtures.prGreen)
        XCTAssertNil(Fixtures.prStateFailed.pr)
        XCTAssertEqual(Fixtures.prStateFailed.errorMessage, Fixtures.prErrorMessage)
        XCTAssertEqual(Fixtures.prStateStale.pr, Fixtures.prGreen)
    }

    // MARK: - Slice details and config

    func testSliceDetails() {
        XCTAssertEqual(Fixtures.sliceDetails.count, 3)
        XCTAssertTrue(Fixtures.sliceDetail.handedBack)
        XCTAssertEqual(Fixtures.sliceDetail.branch, Fixtures.diffBranch)
        XCTAssertTrue(Fixtures.blockedSliceDetail.blocked)
        XCTAssertEqual(Fixtures.blockedSliceDetail.dependsOn, [Fixtures.commentsSliceID])
        XCTAssertTrue(Fixtures.brieflessSliceDetail.brief.isEmpty)
        // Every detail is of a slice the plan actually holds.
        let ids = Set(Fixtures.slices.map(\.id))
        for (key, detail) in Fixtures.sliceDetails {
            XCTAssertEqual(key, detail.id)
            XCTAssertTrue(ids.contains(detail.id))
        }
    }

    func testSliceDetailLoadStates() {
        XCTAssertTrue(Fixtures.sliceDetailStateLoading.isLoading)
        XCTAssertNil(Fixtures.sliceDetailStateIdle.detail)
        XCTAssertEqual(Fixtures.sliceDetailStateLoaded.detail, Fixtures.sliceDetail)
        XCTAssertNil(Fixtures.sliceDetailStateFailed.detail)
        XCTAssertEqual(Fixtures.sliceDetailStateFailed.errorMessage, Fixtures.sliceDetailErrorMessage)
        XCTAssertEqual(Fixtures.sliceDetailStateStale.detail, Fixtures.sliceDetail)
    }

    func testConfigNamesTheFixtureProject() {
        XCTAssertEqual(Array(Fixtures.config.projects.keys), [Fixtures.projectID])
        XCTAssertEqual(Fixtures.config.assigneeUserName, "Craig Johnston")
        XCTAssertTrue(Fixtures.emptyConfig.projects.isEmpty)
        XCTAssertEqual(Array(Fixtures.configDoc.projects.keys), [Fixtures.projectID])
        XCTAssertFalse(Fixtures.paths.config.isEmpty)
        XCTAssertFalse(Fixtures.paths.logDir.isEmpty)
    }

    // MARK: - The shell's own states

    func testSliceLooksOneUpByID() {
        XCTAssertEqual(Fixtures.slice(Fixtures.mergeBoxSliceID).id, Fixtures.mergeBoxSliceID)
        XCTAssertTrue(Fixtures.slice(Fixtures.mergeBoxSliceID).handedBack)
    }

    /// The planning agent sits where `AppModel.planningAgentKey` looks for
    /// one, and beside the slice agents rather than instead of them.
    func testThePlanningAgentIsKeyedByItsProjectsPlanTag() {
        let planner = Fixtures.planningAgentStatus
        XCTAssertEqual(planner.sliceID, TmuxSession.planTag(projectID: Fixtures.projectID))
        XCTAssertEqual(planner.session, TmuxSession.planSessionName(projectID: Fixtures.projectID))
        XCTAssertEqual(planner.activity, .working)
        XCTAssertEqual(Fixtures.agentStatusesWithPlanner, Fixtures.agentStatuses + [planner])
    }

    func testTheToolChecklistIsPinnedRatherThanRead() {
        for binary in ["nat", "tmux", "gh", "ntn"] {
            XCTAssertTrue(Fixtures.toolStatus(binary, in: Fixtures.toolsFound).isFound, binary)
        }
        XCTAssertFalse(Fixtures.toolStatus("nat", in: Fixtures.toolsWithoutNat).isFound)
        XCTAssertFalse(Fixtures.toolStatus("gh", in: Fixtures.toolsWithoutNat).isFound)
        XCTAssertTrue(Fixtures.toolStatus("tmux", in: Fixtures.toolsWithoutNat).isFound)
        // A binary no map names is missing rather than a crash.
        XCTAssertFalse(Fixtures.toolStatus("rg", in: Fixtures.toolsFound).isFound)
    }

    /// The review left on a diff is the store's own state, so what is canned
    /// is the seeding of it — and it lands as the comment box would leave it.
    @MainActor
    func testSeedingPendingCommentsLeavesThemOnTheStore() async {
        let store = DiffStore(client: FixtureNatClient())
        await store.fetch(projectID: Fixtures.projectID, sliceRef: Fixtures.mergeBoxSliceID)
        Fixtures.seedPendingComments(into: store)

        XCTAssertEqual(store.pendingCommentCount, Fixtures.pendingComments.count)
        for canned in Fixtures.pendingComments {
            let left = store.comment(path: canned.path, anchorRowIDs: canned.anchorRowIDs)
            XCTAssertEqual(left?.text, canned.text)
        }
    }

    func testMinutesAgoCountsBackFromThePinnedNow() {
        XCTAssertEqual(Fixtures.minutesAgo(90), Fixtures.now.addingTimeInterval(-5400))
    }
}
