import XCTest
@testable import NatKit

/// Ports the Go TUI's own test cases for `internal/tui/prview.go`,
/// `prchecks.go`, `prconvo.go` and `prmerge.go` (and
/// `internal/actions/mergerefusal.go`) — the same fixtures and the same
/// expected words, since this file is what proves the port agrees with them.
final class PRPresentationTests: XCTestCase {
    // MARK: - Fixtures (mirrors internal/tui/prchecks_test.go / prview_test.go)

    private let passingChecks = [
        PRCheck(name: "build", state: "SUCCESS", link: ""),
        PRCheck(name: "test (ubuntu-latest)", state: "SUCCESS", link: ""),
        PRCheck(name: "lint", state: "SUCCESS", link: ""),
    ]
    private let failingChecks = [
        PRCheck(name: "build", state: "SUCCESS", link: ""),
        PRCheck(name: "test (ubuntu-latest)", state: "FAILURE", link: ""),
        PRCheck(name: "lint", state: "TIMED_OUT", link: ""),
        PRCheck(name: "coverage", state: "SKIPPED", link: ""),
    ]
    private let pendingChecks = [
        PRCheck(name: "build", state: "SUCCESS", link: ""),
        PRCheck(name: "test (ubuntu-latest)", state: "IN_PROGRESS", link: ""),
        PRCheck(name: "lint", state: "QUEUED", link: ""),
    ]

    private func samplePR(
        state: String = "OPEN", isDraft: Bool = false, checks: [PRCheck] = [],
        reviews: [PRReview] = [], comments: [PRCommentEntry] = [],
        reviewDecision: String = "", mergeable: String = "", mergeStateStatus: String = "",
        baseRefName: String = "main"
    ) -> PRDetail {
        PRDetail(
            number: 12, title: "Open a PR screen over the board",
            body: "## What\n\nA screen over the board that draws a pull request.",
            state: state, isDraft: isDraft, author: "craigmjohnston",
            baseRefName: baseRefName, headRefName: "slice/open-a-pr-screen-over-the-board",
            url: "https://github.test/craig/nat/pull/12",
            checks: checks, reviews: reviews, comments: comments,
            reviewDecision: reviewDecision, mergeable: mergeable, mergeStateStatus: mergeStateStatus
        )
    }

    // MARK: - prStateChip (mirrors TestPRStateChip-equivalent logic in prview.go)

    func testPRStateChipMergedTestedBeforeDraft() {
        let chip = prStateChip(state: PRLifecycleState.merged, isDraft: true)
        XCTAssertEqual(chip.label, "merged")
    }

    func testPRStateChipClosedTestedBeforeDraft() {
        let chip = prStateChip(state: PRLifecycleState.closed, isDraft: true)
        XCTAssertEqual(chip.label, "closed")
    }

    func testPRStateChipDraft() {
        XCTAssertEqual(prStateChip(state: "OPEN", isDraft: true).label, "draft")
    }

    func testPRStateChipUnknownReadsAsOpen() {
        XCTAssertEqual(prStateChip(state: "SOMETHING_ELSE", isDraft: false).label, "open")
        XCTAssertEqual(prStateChip(state: "OPEN", isDraft: false).label, "open")
    }

    // MARK: - checkOutcome (mirrors TestCheckOutcomeOf)

    func testCheckOutcomeOf() {
        let cases: [(String, CheckOutcome)] = [
            ("SUCCESS", .passing), ("FAILURE", .failing), ("ERROR", .failing),
            ("TIMED_OUT", .failing), ("STARTUP_FAILURE", .failing), ("ACTION_REQUIRED", .failing),
            ("SKIPPED", .skipped), ("NEUTRAL", .skipped), ("CANCELLED", .skipped), ("STALE", .skipped),
            ("IN_PROGRESS", .pending), ("QUEUED", .pending), ("PENDING", .pending),
            ("EXPECTED", .pending), ("", .pending), ("REBOOTING", .pending), (" success ", .passing),
        ]
        for (state, want) in cases {
            XCTAssertEqual(checkOutcome(state: state), want, "outcome of \(state)")
        }
    }

    func testCheckStateWord() {
        XCTAssertEqual(checkStateWord("SUCCESS"), "success")
        XCTAssertEqual(checkStateWord("TIMED_OUT"), "timed out")
        XCTAssertEqual(checkStateWord(""), "unknown")
        XCTAssertEqual(checkStateWord("  "), "unknown")
    }

    // MARK: - checkRollup (mirrors TestCheckSummaryAgreesWithTheRows)

    func testCheckRollupAllPassing() {
        let rollup = checkRollup(passingChecks)
        XCTAssertEqual(rollup.summary, "3 passing")
        XCTAssertEqual(rollup.outcome, .passing)
    }

    func testCheckRollupAFailureAmongThem() {
        let rollup = checkRollup(failingChecks)
        XCTAssertEqual(rollup.summary, "2 failing · 1 passing · 1 skipped")
        XCTAssertEqual(rollup.outcome, .failing)
    }

    func testCheckRollupStillGoing() {
        let rollup = checkRollup(pendingChecks)
        XCTAssertEqual(rollup.summary, "2 pending · 1 passing")
        XCTAssertEqual(rollup.outcome, .pending)
    }

    func testCheckRollupNothingButSkipped() {
        let rollup = checkRollup([PRCheck(name: "deploy", state: "SKIPPED", link: "")])
        XCTAssertEqual(rollup.summary, "1 skipped")
        XCTAssertEqual(rollup.outcome, .skipped)
    }

    func testCheckRollupOfNoChecksIsPassing() {
        // Unreachable through the checks section (which draws one quiet line
        // instead); asked anyway, it is the outcome nothing is wrong in.
        XCTAssertEqual(checkRollup([]).outcome, .passing)
    }

    func testCheckOutcomeMarksAreDistinctAndOneCellEach() {
        var seen = Set<String>()
        for outcome: CheckOutcome in [.passing, .failing, .pending, .skipped] {
            XCTAssertFalse(outcome.markSymbolName.isEmpty)
            XCTAssertFalse(seen.contains(outcome.markSymbolName), "\(outcome) shares a mark with another outcome")
            seen.insert(outcome.markSymbolName)
        }
    }

    // MARK: - convoTone

    func testConvoToneApprovedChangesRequestedDismissedTakeCheckMarks() {
        XCTAssertEqual(convoTone(reviewState: "APPROVED"), .approved)
        XCTAssertEqual(convoTone(reviewState: "CHANGES_REQUESTED"), .rejected)
        XCTAssertEqual(convoTone(reviewState: "DISMISSED"), .dismissed)
    }

    func testConvoToneEverythingElseIsNeutral() {
        XCTAssertEqual(convoTone(reviewState: "COMMENTED"), .neutral)
        XCTAssertEqual(convoTone(reviewState: "SOMETHING_ELSE"), .neutral)
        XCTAssertEqual(convoTone(reviewState: ""), .neutral)
    }

    func testConvoToneMarksAreDistinct() {
        var seen = Set<String>()
        for tone: ConvoTone in [.neutral, .approved, .rejected, .dismissed] {
            XCTAssertFalse(seen.contains(tone.markSymbolName), "\(tone) shares a mark with another tone")
            seen.insert(tone.markSymbolName)
        }
    }

    // MARK: - reviewEntry (mirrors TestReviewEntry)

    private func at(_ hoursAgo: Int, from now: Date = Date(timeIntervalSince1970: 1_772_625_600)) -> Date {
        now.addingTimeInterval(-Double(hoursAgo) * 3600)
    }

    func testReviewEntryApproved() {
        let entry = reviewEntry(PRReview(author: "r", state: "APPROVED", body: "", submittedAt: at(1)))
        XCTAssertEqual(entry?.verb, "approved")
        XCTAssertEqual(entry?.tone, .approved)
    }

    func testReviewEntryChangesRequested() {
        let entry = reviewEntry(PRReview(author: "r", state: "CHANGES_REQUESTED", body: "", submittedAt: at(1)))
        XCTAssertEqual(entry?.verb, "requested changes")
        XCTAssertEqual(entry?.tone, .rejected)
    }

    func testReviewEntryDismissed() {
        let entry = reviewEntry(PRReview(author: "r", state: "DISMISSED", body: "", submittedAt: at(1)))
        XCTAssertEqual(entry?.verb, "review dismissed")
        XCTAssertEqual(entry?.tone, .dismissed)
    }

    func testReviewEntryCommentedWithWords() {
        let entry = reviewEntry(PRReview(author: "r", state: "COMMENTED", body: "hm", submittedAt: at(1)))
        XCTAssertEqual(entry?.verb, "reviewed")
        XCTAssertEqual(entry?.tone, .neutral)
    }

    func testReviewEntryLowerCaseState() {
        let entry = reviewEntry(PRReview(author: "r", state: " approved ", body: "", submittedAt: at(1)))
        XCTAssertEqual(entry?.verb, "approved")
        XCTAssertEqual(entry?.tone, .approved)
    }

    func testReviewEntryAWordThisBuildDoesNotKnow() {
        let entry = reviewEntry(PRReview(author: "r", state: "SOMETHING_ELSE", body: "", submittedAt: at(1)))
        XCTAssertEqual(entry?.verb, "reviewed (something else)")
        XCTAssertEqual(entry?.tone, .neutral)
    }

    func testReviewEntryCommentedWithNothingToSayIsDropped() {
        XCTAssertNil(reviewEntry(PRReview(author: "r", state: "COMMENTED", body: "", submittedAt: at(1))))
    }

    func testReviewEntryNeverSubmittedIsDropped() {
        XCTAssertNil(reviewEntry(PRReview(author: "r", state: "PENDING", body: "", submittedAt: nil)))
    }

    func testReviewEntrySubmittedWithNoTimeIsDropped() {
        XCTAssertNil(reviewEntry(PRReview(author: "r", state: "APPROVED", body: "", submittedAt: nil)))
    }

    func testReviewEntryIsAlwaysAReview() {
        let entry = reviewEntry(PRReview(author: "r", state: "APPROVED", body: "", submittedAt: at(1)))
        XCTAssertEqual(entry?.isReview, true)
    }

    // MARK: - conversation (mirrors TestConversationOrder / TestConversationStableOnATie)

    func testConversationInterleavesByTimeOldestFirst() {
        let comments = [
            PRCommentEntry(author: "craigmjohnston", body: "Ready for a look.", createdAt: at(9), url: ""),
            PRCommentEntry(author: "craigmjohnston", body: "Pushed a fix for that.", createdAt: at(3), url: ""),
        ]
        let reviews = [
            PRReview(author: "reviewer", state: "CHANGES_REQUESTED", body: "The width is off.", submittedAt: at(6)),
            PRReview(author: "reviewer", state: "APPROVED", body: "", submittedAt: at(1)),
        ]
        let entries = conversation(comments: comments, reviews: reviews)
        XCTAssertEqual(entries.map { "\($0.author) \($0.verb)" }, [
            "craigmjohnston commented",
            "reviewer requested changes",
            "craigmjohnston commented",
            "reviewer approved",
        ])
    }

    func testConversationStableOnATieWithCommentsFirst() {
        let tieTime = at(2)
        let comments = [
            PRCommentEntry(author: "first", body: "a", createdAt: tieTime, url: ""),
            PRCommentEntry(author: "second", body: "b", createdAt: tieTime, url: ""),
        ]
        let reviews = [PRReview(author: "third", state: "APPROVED", body: "", submittedAt: tieTime)]
        for _ in 0..<5 {
            let entries = conversation(comments: comments, reviews: reviews)
            XCTAssertEqual(entries.map(\.author), ["first", "second", "third"])
        }
    }

    func testConversationAuthorlessCommentAndReviewReadAsSomeone() {
        let comments = [PRCommentEntry(author: "", body: "left by nobody", createdAt: at(1), url: "")]
        let reviews = [PRReview(author: "", state: "APPROVED", body: "", submittedAt: at(1))]
        for entry in conversation(comments: comments, reviews: reviews) {
            XCTAssertEqual(entry.author, "someone")
        }
    }

    // MARK: - convoSummary

    func testConvoSummaryBothKinds() {
        let comments = [PRCommentEntry(author: "a", body: "hi", createdAt: at(2), url: "")]
        let reviews = [PRReview(author: "b", state: "APPROVED", body: "", submittedAt: at(1))]
        let entries = conversation(comments: comments, reviews: reviews)
        XCTAssertEqual(convoSummary(entries), "1 comment · 1 review")
    }

    func testConvoSummaryCommentsAlone() {
        let comments = [PRCommentEntry(author: "a", body: "hi", createdAt: at(2), url: "")]
        XCTAssertEqual(convoSummary(conversation(comments: comments, reviews: [])), "1 comment")
    }

    func testConvoSummaryReviewsAlone() {
        let reviews = [PRReview(author: "b", state: "APPROVED", body: "", submittedAt: at(1))]
        XCTAssertEqual(convoSummary(conversation(comments: [], reviews: reviews)), "1 review")
    }

    func testConvoSummaryPluralizesBothKinds() {
        let comments = [
            PRCommentEntry(author: "a", body: "hi", createdAt: at(2), url: ""),
            PRCommentEntry(author: "a", body: "hi again", createdAt: at(1), url: ""),
        ]
        let reviews = [
            PRReview(author: "b", state: "APPROVED", body: "", submittedAt: at(3)),
            PRReview(author: "c", state: "APPROVED", body: "", submittedAt: at(4)),
        ]
        XCTAssertEqual(convoSummary(conversation(comments: comments, reviews: reviews)), "2 comments · 2 reviews")
    }

    func testConvoSummaryOfNothingIsEmpty() {
        XCTAssertEqual(convoSummary([]), "")
    }

    // MARK: - Merge verdicts (mirrors TestMergeReviewVerdict / TestMergeChecksVerdict / TestMergeMergeableVerdict)

    func testReviewVerdictCases() {
        let cases: [(String, String, CheckOutcome)] = [
            ("APPROVED", "approved", .passing),
            ("CHANGES_REQUESTED", "changes requested", .failing),
            ("REVIEW_REQUIRED", "review required", .pending),
            ("", "no review required", .skipped),
            ("   ", "no review required", .skipped),
            (" approved ", "approved", .passing),
            ("SECOND_OPINION", "second opinion", .pending),
        ]
        for (decision, word, outcome) in cases {
            let verdict = reviewVerdict(reviewDecision: decision)
            XCTAssertEqual(verdict.label, "review")
            XCTAssertEqual(verdict.word, word, "decision \(decision)")
            XCTAssertEqual(verdict.outcome, outcome, "decision \(decision)")
        }
    }

    func testChecksVerdictCases() {
        XCTAssertEqual(checksVerdict(checks: passingChecks), MergeVerdict(label: "checks", word: "3 passing", outcome: .passing))
        XCTAssertEqual(
            checksVerdict(checks: failingChecks),
            MergeVerdict(label: "checks", word: "2 failing · 1 passing · 1 skipped", outcome: .failing))
        XCTAssertEqual(
            checksVerdict(checks: pendingChecks),
            MergeVerdict(label: "checks", word: "2 pending · 1 passing", outcome: .pending))
        XCTAssertEqual(checksVerdict(checks: []), MergeVerdict(label: "checks", word: "no checks", outcome: .skipped))
    }

    func testMergeableVerdictCases() {
        let cases: [(String, String, String, CheckOutcome)] = [
            ("MERGEABLE", "CLEAN", "no conflicts with main", .passing),
            ("MERGEABLE", "BLOCKED", "no conflicts with main", .passing),
            ("CONFLICTING", "DIRTY", "conflicting with main", .failing),
            ("CONFLICTING", "", "conflicting with main", .failing),
            ("UNKNOWN", "DIRTY", "conflicting with main", .failing),
            ("MERGEABLE", "BEHIND", "behind main", .pending),
            ("UNKNOWN", "UNKNOWN", "mergeability unknown", .pending),
            ("", "", "mergeability unknown", .pending),
            ("PERHAPS", "SOMEHOW", "mergeability unknown", .pending),
        ]
        for (mergeable, state, word, outcome) in cases {
            let verdict = mergeableVerdict(mergeable: mergeable, mergeStateStatus: state, baseRefName: "main")
            XCTAssertEqual(verdict.label, "mergeable")
            XCTAssertEqual(verdict.word, word, "mergeable=\(mergeable) state=\(state)")
            XCTAssertEqual(verdict.outcome, outcome, "mergeable=\(mergeable) state=\(state)")
        }
        XCTAssertEqual(
            mergeableVerdict(mergeable: "MERGEABLE", mergeStateStatus: "", baseRefName: "").word,
            "no conflicts with its base")
    }

    // MARK: - mergeRollup / mergeHeading (mirrors TestMergeRollupAndSummary)

    func testMergeRollupAndHeading() {
        let clean = samplePR(reviewDecision: "APPROVED", mergeable: "MERGEABLE", mergeStateStatus: "CLEAN", baseRefName: "main")
        XCTAssertEqual(mergeRollup(mergeVerdicts(clean)), .passing)
        XCTAssertEqual(mergeHeading(mergeRollup(mergeVerdicts(clean))).words, "ready to merge")

        let conflicting = samplePR(
            reviewDecision: "APPROVED", mergeable: "CONFLICTING", mergeStateStatus: "DIRTY", baseRefName: "main")
        XCTAssertEqual(mergeRollup(mergeVerdicts(conflicting)), .failing)
        XCTAssertEqual(mergeHeading(mergeRollup(mergeVerdicts(conflicting))).words, "cannot merge")

        let nothingSettled = samplePR()
        XCTAssertEqual(mergeRollup(mergeVerdicts(nothingSettled)), .pending)
        XCTAssertEqual(mergeHeading(mergeRollup(mergeVerdicts(nothingSettled))).words, "not ready to merge")
    }

    func testMergeRollupOfSkippedVerdictsIsSkipped() {
        // Every verdict never asked at all — no review required and nothing
        // to run — is one nothing stands in the way of.
        let skipped = [MergeVerdict(label: "x", word: "", outcome: .skipped)]
        XCTAssertEqual(mergeRollup(skipped), .skipped)
        XCTAssertEqual(mergeHeading(.skipped).words, "ready to merge")
    }

    func testMergeRollupOfNoVerdictsIsPassing() {
        XCTAssertEqual(mergeRollup([]), .passing)
    }

    // MARK: - mergeRefusal (mirrors internal/actions/mergerefusal.go's own tests)

    func testMergeRefusalNamesTheFirstFailingVerdict() {
        let changesRequested = samplePR(
            reviewDecision: "CHANGES_REQUESTED", mergeable: "MERGEABLE", mergeStateStatus: "CLEAN", baseRefName: "main")
        XCTAssertEqual(mergeRefusal(changesRequested), "review: changes requested")

        let failingChecksPR = samplePR(
            checks: failingChecks, reviewDecision: "APPROVED", mergeable: "MERGEABLE", mergeStateStatus: "CLEAN")
        XCTAssertEqual(mergeRefusal(failingChecksPR), "checks: 2 failing · 1 passing · 1 skipped")

        let conflicting = samplePR(
            reviewDecision: "APPROVED", mergeable: "CONFLICTING", mergeStateStatus: "DIRTY", baseRefName: "main")
        XCTAssertEqual(mergeRefusal(conflicting), "mergeable: conflicting with main")
    }

    func testMergeRefusalIsNilWhenNothingFails() {
        let clean = samplePR(reviewDecision: "APPROVED", mergeable: "MERGEABLE", mergeStateStatus: "CLEAN")
        XCTAssertNil(mergeRefusal(clean))

        // Still to come, not a refusal.
        XCTAssertNil(mergeRefusal(samplePR()))
    }

    // MARK: - mergeBoxState (mirrors TestMergeSectionEndings)

    func testMergeBoxStateMergedEnding() {
        let merged = samplePR(state: PRLifecycleState.merged, baseRefName: "main")
        guard case .ended(let words, _) = mergeBoxState(for: merged) else {
            return XCTFail("expected an ending")
        }
        XCTAssertEqual(words, "merged into main")
    }

    func testMergeBoxStateMergedWithNoBaseNamesItInWords() {
        let merged = samplePR(state: PRLifecycleState.merged, baseRefName: "")
        guard case .ended(let words, _) = mergeBoxState(for: merged) else {
            return XCTFail("expected an ending")
        }
        XCTAssertEqual(words, "merged into its base")
    }

    func testMergeBoxStateClosedEnding() {
        let closed = samplePR(state: PRLifecycleState.closed)
        guard case .ended(let words, _) = mergeBoxState(for: closed) else {
            return XCTFail("expected an ending")
        }
        XCTAssertEqual(words, "closed without merging")
    }

    func testMergeBoxStateOpenStillWeighsVerdicts() {
        let open = samplePR(reviewDecision: "APPROVED", mergeable: "MERGEABLE", mergeStateStatus: "CLEAN")
        guard case .verdicts(let heading, let verdicts) = mergeBoxState(for: open) else {
            return XCTFail("expected verdicts")
        }
        XCTAssertEqual(heading.words, "ready to merge")
        XCTAssertEqual(verdicts.count, 3)
    }

    // MARK: - ago (mirrors internal/tui/refresh.go's ago)

    func testAgo() {
        XCTAssertEqual(ago(0), "just now")
        XCTAssertEqual(ago(59), "just now")
        XCTAssertEqual(ago(-5), "just now", "a clock that has gone backwards reads as just now")
        XCTAssertEqual(ago(60), "1m ago")
        XCTAssertEqual(ago(90), "1m ago")
        XCTAssertEqual(ago(3599), "59m ago")
        XCTAssertEqual(ago(3600), "1h ago")
        XCTAssertEqual(ago(6 * 3600), "6h ago")
        XCTAssertEqual(ago(86399), "23h ago")
        XCTAssertEqual(ago(86400), "1d ago")
        XCTAssertEqual(ago(3 * 86400), "3d ago")
    }

    // MARK: - convoAuthor

    func testConvoAuthorBlankReadsAsSomeone() {
        XCTAssertEqual(convoAuthor(""), "someone")
        XCTAssertEqual(convoAuthor("   "), "someone")
        XCTAssertEqual(convoAuthor("craig"), "craig")
    }
}
