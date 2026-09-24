import XCTest
@testable import NatKit

final class PaneSkeletonsTests: XCTestCase {
    /// Every width in every pane skeleton is a fraction of the column it is
    /// drawn in: a block wider than its column would run out past the pane's
    /// trailing edge, and one of no width would draw nothing at all.
    private func assertFractions(_ widths: SkeletonLines, _ what: String, line: UInt = #line) {
        XCTAssertFalse(widths.isEmpty, "\(what) draws nothing at all", line: line)
        for width in widths {
            XCTAssertGreaterThan(width, 0, "\(what) has a block of no width", line: line)
            XCTAssertLessThanOrEqual(width, 1, "\(what) has a block wider than its column", line: line)
        }
    }

    /// A rail's sections: a heading that is drawn as itself, so it has to be
    /// the real label, and rows that are placeholders, so they have to be
    /// fractions of the column.
    private func assertRail(_ sections: [SkeletonRailSectionShape], _ what: String, line: UInt = #line) {
        XCTAssertFalse(sections.isEmpty, "\(what) draws nothing at all", line: line)
        for section in sections {
            XCTAssertFalse(section.title.isEmpty, "\(what) has a section with no heading", line: line)
            assertFractions(section.rows, "a section of \(what)", line: line)
        }
    }

    // MARK: - Brief

    func testTheBriefIsDrawnAsParagraphsOfProse() {
        XCTAssertGreaterThan(BriefSkeleton.paragraphs.count, 1,
                             "a brief is prose, and one run of lines does not read as any")
        for paragraph in BriefSkeleton.paragraphs {
            assertFractions(paragraph, "a brief paragraph")
        }
    }

    /// Prose does not end flush: a paragraph whose last line filled the
    /// column would read as a block rather than as text.
    func testEveryBriefParagraphEndsOnAPartLine() {
        for paragraph in BriefSkeleton.paragraphs {
            let last = paragraph.last ?? 1
            XCTAssertLessThan(last, paragraph.max() ?? 1, "a paragraph ending flush does not read as prose")
        }
    }

    /// The properties rail is only drawn once there is a detail to read it
    /// off, so a skeleton with no rail would have the reading column narrow
    /// the moment the brief landed — which is the shift this slice is about.
    func testTheAgentSkeletonIsFractionsOfItsColumn() {
        assertFractions(AgentSkeleton.lines, "the agent skeleton")
        XCTAssertFalse(AgentSkeleton.accessibilityLabel.isEmpty)
    }

    func testTheBriefSkeletonHasARailBesideIt() {
        assertRail(BriefSkeleton.sidebarSections, "the brief's properties rail")
        XCTAssertEqual(
            BriefSkeleton.sidebarSections.map(\.title),
            ["STATUS", "MILESTONE", "BRANCH", "DEPENDS ON"],
            "the four sections `BriefTabView` draws, in its own order"
        )
    }

    func testTheBriefSaysItIsLoadingForAnyoneWhoCannotSeeTheBlocks() {
        XCTAssertFalse(BriefSkeleton.accessibilityLabel.isEmpty)
    }

    // MARK: - Diff

    func testTheDiffIsDrawnAsFileBoxes() {
        XCTAssertFalse(DiffSkeleton.files.isEmpty, "a diff with no boxes is a blank pane")
        for file in DiffSkeleton.files {
            assertFractions([file.pathWidth], "a diff file's path")
            assertFractions(file.rows, "a diff file's rows")
        }
    }

    func testTheDiffSkeletonHasAFileListBesideIt() {
        assertFractions(DiffSkeleton.sidebarRows, "the diff's file list")
    }

    /// The list is of every file the branch touches; the boxes are only the
    /// first few that fit, so a list no longer than them would shrink the
    /// moment the diff landed.
    func testTheFileListIsLongerThanTheBoxesDrawnBesideIt() {
        XCTAssertGreaterThan(DiffSkeleton.sidebarRows.count, DiffSkeleton.files.count)
    }

    func testTheDiffSaysItIsLoadingForAnyoneWhoCannotSeeTheBlocks() {
        XCTAssertFalse(DiffSkeleton.accessibilityLabel.isEmpty)
    }

    func testADiffSkeletonFileCarriesWhatItWasBuiltWith() {
        let file = DiffSkeletonFile(pathWidth: 0.3, rows: [0.5])

        XCTAssertEqual(file.pathWidth, 0.3)
        XCTAssertEqual(file.rows, [0.5])
    }

    // MARK: - PR

    func testThePullRequestIsDrawnAsATitleOverItsBranchLine() {
        assertFractions([PRSkeleton.titleWidth], "the pull request's title")
        assertFractions([PRSkeleton.branchLineWidth], "the pull request's branch line")
        XCTAssertLessThan(PRSkeleton.branchLineWidth, PRSkeleton.titleWidth,
                          "head → base is shorter than a title whatever the branches are called")
    }

    func testThePullRequestIsDrawnWithADescriptionAndAConversation() {
        assertFractions(PRSkeleton.descriptionLines, "the pull request's description")
        XCTAssertFalse(PRSkeleton.conversationEntries.isEmpty)
        for entry in PRSkeleton.conversationEntries {
            assertFractions(entry, "a conversation entry")
        }
    }

    func testThePullRequestSkeletonHasItsRailBesideIt() {
        assertRail(PRSkeleton.sidebarSections, "the pull request's rail")
        XCTAssertEqual(
            PRSkeleton.sidebarSections.map(\.title),
            ["CHECKS", "REVIEW", "CHANGES"],
            "the three sections `PRSidebarView` draws, in its own order"
        )
    }

    func testThePullRequestSaysItIsLoadingForAnyoneWhoCannotSeeTheBlocks() {
        XCTAssertFalse(PRSkeleton.accessibilityLabel.isEmpty)
    }

    // MARK: - All three

    /// Fixed rather than rolled, exactly as `RailSkeleton`'s widths are: a
    /// pane redraws on every hover and every resize, and widths that changed
    /// each time would have the column twitching all through the load.
    func testTheShapesAreTheSameEveryTimeTheyAreRead() {
        XCTAssertEqual(BriefSkeleton.paragraphs, BriefSkeleton.paragraphs)
        XCTAssertEqual(DiffSkeleton.files, DiffSkeleton.files)
        XCTAssertEqual(PRSkeleton.conversationEntries, PRSkeleton.conversationEntries)
    }

    func testEachPaneSaysWhichOfThemIsLoading() {
        let labels = Set([
            BriefSkeleton.accessibilityLabel,
            DiffSkeleton.accessibilityLabel,
            PRSkeleton.accessibilityLabel
        ])

        XCTAssertEqual(labels.count, 3, "three panes reading as one thing tells nobody which is up")
    }
}
