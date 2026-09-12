import XCTest
@testable import NatKit

/// The rail's three sections — ACTIVE, TODO and DONE — each a pinned heading
/// over a scroll of its own. Read off the source the way
/// `ProjectTabStripRulesTests` reads the tab strip: the rail is a SwiftUI
/// view in the app target, which the test target cannot import, so what is
/// checked is that the sections are still built the way the rules say.
final class RailSectionRulesTests: XCTestCase {
    private func packageRoot() -> URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()   // Components
            .deletingLastPathComponent()   // NatKitTests
            .deletingLastPathComponent()   // Tests
            .deletingLastPathComponent()   // the package root
    }

    private func source(_ relative: String = "Sources/NatApp/Views/RailView.swift") throws -> String {
        let url = packageRoot().appendingPathComponent(relative)
        guard FileManager.default.fileExists(atPath: url.path) else {
            throw XCTSkip("no \(relative) beside the tests")
        }
        return try String(contentsOf: url, encoding: .utf8)
    }

    /// The note is one string in one place, so the rail and anything else
    /// that ever says it cannot say two different things.
    func testTheEmptyNoteIsTheSharedOne() throws {
        XCTAssertEqual(EmptyActiveNote.text, "Nothing running")
        XCTAssertTrue(
            try source().contains("Text(EmptyActiveNote.text)"),
            "the rail should draw the shared note rather than a literal of its own"
        )
    }

    /// The heading draws whether or not anything is running — the whole
    /// point of the section being always on the rail.
    func testTheActiveHeadingIsNotConditional() throws {
        let s = try source()
        guard let heading = s.range(of: "sectionHeading(.active)") else {
            return XCTFail("the rail should draw an ACTIVE heading")
        }
        // The line the heading is on, and the one above it: an `if` there is
        // exactly what used to keep the section off an idle rail.
        let upToHeading = s[s.startIndex..<heading.lowerBound]
        let lines = upToHeading.split(separator: "\n", omittingEmptySubsequences: false)
        let preceding = lines.suffix(2).joined(separator: " ")
        XCTAssertFalse(
            preceding.contains("if "),
            "the ACTIVE heading must not be guarded by a condition"
        )
    }

    /// With nothing to list, the section draws the note instead of the rows
    /// — the one branch inside it, rather than around it.
    func testTheEmptyNoteStandsInForTheEntries() throws {
        let s = try source()
        guard let branch = s.range(of: "if railModel.active.isEmpty {"),
              let note = s.range(of: "activeEmptyNote\n"),
              let rows = s.range(of: "ForEach(railModel.active) { entry in") else {
            return XCTFail("the ACTIVE section should branch on having entries")
        }
        XCTAssertTrue(branch.lowerBound < note.lowerBound)
        XCTAssertTrue(note.lowerBound < rows.lowerBound, "the note is the empty half of the branch")
    }

    /// The note sits in the column an entry's name starts in, and it is the
    /// rail's own geometry that puts it there rather than a number typed out
    /// beside it.
    func testTheNoteIsIndentedToTheEntryTextColumn() throws {
        XCTAssertTrue(
            try source().contains(".padding(.leading, RailSlot.leading + RailSlot.slot + RailSlot.spacing)"),
            "the note should be indented off the shared slot geometry"
        )
    }

    /// The empty state reserves a two-line entry's height — the departure
    /// from the mock the design README records — so the rail below it does
    /// not jump as the first entry lands or the last one leaves. The two
    /// hidden lines and the same vertical padding `sessionRow` carries are
    /// what make the two heights one.
    func testTheEmptyNoteReservesATwoLineEntrysHeight() throws {
        let s = try source()
        guard let note = s.range(of: "private var activeEmptyNote: some View {"),
              let end = s.range(of: "// MARK: - Session rows", range: note.upperBound..<s.endIndex) else {
            return XCTFail("the rail should hold an activeEmptyNote")
        }
        let body = String(s[note.upperBound..<end.lowerBound])
        XCTAssertEqual(
            body.components(separatedBy: ".hidden()").count - 1, 2,
            "both of an entry's lines should be reserved"
        )
        XCTAssertTrue(
            body.contains("VStack(alignment: .leading, spacing: 1)"),
            "the reserved lines should be spaced as sessionRow spaces an entry's"
        )
        XCTAssertTrue(body.contains(".padding(.vertical, 8)"), "an entry's own vertical padding")
    }

    /// ACTIVE is the only flight section there is: the planning agent and
    /// the branches awaiting review are entries of it, so neither of the two
    /// headings they used to have may be left anywhere on the rail.
    func testTheOtherFlightHeadingsAreGone() throws {
        let s = try source()
        XCTAssertFalse(s.contains("WORKSHOP"), "the workshop is an ACTIVE entry, not a section")
        XCTAssertFalse(s.contains("NEEDS REVIEW"), "a branch awaiting review is an ACTIVE entry")
    }

    /// Which pane an entry selects is the model's `kind`, so the one row
    /// builder can draw all three without the view sorting them out again.
    func testTheEntrysKindIsWhatSelectsItsPane() throws {
        let s = try source()
        XCTAssertTrue(
            s.contains("case .workshop: appModel.workshopSelected = true"),
            "the workshop entry should still select the workshop pane"
        )
        XCTAssertTrue(
            s.contains("case .slice: appModel.selectedSliceID = entry.sliceID"),
            "a slice entry should still select its slice"
        )
    }

    // MARK: - Three sections, three scrolls

    /// Each section is built by a builder of its own, in the order the rail
    /// reads: what is running, what is queued, what is finished.
    func testTheRailIsThreeSectionsInOneColumn() throws {
        let s = try source()
        guard let column = s.range(of: "private var railColumn: some View {"),
              let active = s.range(of: "activeSection(height:", range: column.upperBound..<s.endIndex),
              let todo = s.range(of: "todoSection(height:", range: active.upperBound..<s.endIndex),
              let done = s.range(of: "doneSection(summary, height:", range: todo.upperBound..<s.endIndex)
        else {
            return XCTFail("the rail should stack its three sections in one column")
        }
        XCTAssertTrue(active.lowerBound < todo.lowerBound && todo.lowerBound < done.lowerBound)
    }

    /// No heading scrolls: in every section the heading is built before the
    /// `ScrollView` rather than inside it, so the title holds still while
    /// its own list moves under it.
    func testEveryHeadingIsOutsideItsSectionsScroll() throws {
        let s = try source()
        for (builder, heading) in [
            ("private func activeSection(height: CGFloat?) -> some View {", "sectionHeading(.active)"),
            ("private func todoSection(height: CGFloat?) -> some View {", "sectionHeading(.todo)"),
            ("private func doneSection(_ summary: DoneSummary, height: CGFloat?) -> some View {",
             "sectionHeading(.done,")
        ] {
            guard let start = s.range(of: builder) else {
                return XCTFail("the rail should build \(heading)'s section")
            }
            guard let title = s.range(of: heading, range: start.upperBound..<s.endIndex),
                  let scroll = s.range(of: "ScrollView {", range: start.upperBound..<s.endIndex) else {
                return XCTFail("\(heading) should sit over a scroll of its own")
            }
            XCTAssertTrue(title.lowerBound < scroll.lowerBound,
                          "\(heading) must be built outside its section's ScrollView")
        }
    }

    /// Three scrolls, one per section, and each is given its share as a
    /// frame and scrolls within it — never the one shared scroll the plan
    /// used to be.
    func testEachSectionScrollsWithinItsOwnShare() throws {
        let s = try source()
        XCTAssertEqual(
            s.components(separatedBy: "ScrollView {").count - 1, 3,
            "one scroll per section and no more"
        )
        for section in RailSection.allCases {
            XCTAssertTrue(
                s.contains(".scrollDisabled(!scrolls(.\(section.rawValue), within: height))"),
                "\(section.title) should scroll only once it has more than its share"
            )
        }
        XCTAssertEqual(
            s.components(separatedBy: ".frame(height: height)").count - 1, 3,
            "every section should be drawn at the height it was given"
        )
    }

    /// How the rail is shared out is `RailSectionLayout`'s answer rather
    /// than a number typed into the view, so the rule and its tests are one
    /// thing. A collapsed section is simply not in the share.
    func testTheSharingIsTheSharedRule() throws {
        let s = try source()
        XCTAssertTrue(s.contains("RailSectionLayout.heights("), "the shares come from the shared rule")
        XCTAssertTrue(s.contains("RailSectionLayout.scrolls("), "and so does whether one scrolls")
        XCTAssertTrue(s.contains("RailSectionLayout.footRoom"), "and the air under the last of them")
        XCTAssertTrue(
            s.contains("drawnSections.filter { !collapsed.contains($0) }"),
            "a collapsed section takes no part in the share"
        )
        XCTAssertFalse(s.contains("RailPinnedBand"), "the band's own rule is gone with the band")
    }

    /// The rule between two sections belongs to the section under it and is
    /// outside its scroll: a separator that moved with what it separates is
    /// not one.
    func testTheSeparatorsDoNotScroll() throws {
        let s = try source()
        for builder in [
            "private func todoSection(height: CGFloat?) -> some View {",
            "private func doneSection(_ summary: DoneSummary, height: CGFloat?) -> some View {"
        ] {
            guard let start = s.range(of: builder),
                  let rule = s.range(of: "sectionRule", range: start.upperBound..<s.endIndex),
                  let scroll = s.range(of: "ScrollView {", range: start.upperBound..<s.endIndex) else {
                return XCTFail("\(builder) should open with the rule above its heading")
            }
            XCTAssertTrue(rule.lowerBound < scroll.lowerBound, "the rule is chrome, not a row")
        }
    }

    /// The load's own states stay with the plan: the skeleton, the retry and
    /// the stale-plan warning are TODO's, since what they stand in for is
    /// the plan.
    func testTheLoadStatesStayWithThePlan() throws {
        let s = try source()
        guard let todo = s.range(of: "private func todoSection(height: CGFloat?) -> some View {"),
              let states = s.range(of: "planLoadStates", range: todo.upperBound..<s.endIndex),
              let done = s.range(of: "private func doneSection(") else {
            return XCTFail("the load states belong to the TODO section")
        }
        XCTAssertTrue(states.lowerBound < done.lowerBound)
        XCTAssertTrue(s.contains("RailSkeletonView()"), "a cold load still draws the plan's shape")
        XCTAssertTrue(s.contains("Button(\"Try Again\")"), "a failed first load still offers the retry")
    }

    // MARK: - The headings are one control

    /// One builder draws all three headings, so none of them can drift from
    /// the others — and every one of them carries its section's icon and the
    /// fold chevron both.
    func testEveryHeadingIsIconAndChevron() throws {
        let s = try source()
        XCTAssertTrue(
            s.contains("private func sectionHeading(_ section: RailSection, trailing: String? = nil)"),
            "the three headings should be one builder"
        )
        XCTAssertTrue(s.contains("Image(systemName: section.icon)"), "every heading wears its own icon")
        XCTAssertTrue(
            s.contains(#"Image(systemName: open ? "chevron.down" : "chevron.right")"#),
            "and every heading wears a fold chevron"
        )
        XCTAssertFalse(s.contains("doneHeadingRow"), "DONE is drawn by the shared builder now")
    }

    /// Clicking a heading folds its section, and the fold is the view's own
    /// state — never anything written to the plan.
    func testAHeadingFoldsItsSection() throws {
        let s = try source()
        XCTAssertTrue(s.contains("@State private var collapsed: Set<RailSection>"),
                      "the fold is the view's own state")
        XCTAssertTrue(s.contains(".onTapGesture { toggle(section) }"),
                      "the whole heading row is the fold's target")
        XCTAssertTrue(s.contains("withAnimation(Motion.stateChange)"),
                      "a fold moves the way every other state change does")
    }

    /// The one seam a story needs: which sections a rail opens with. The app
    /// takes the default — DONE away, the rest open — so nothing about the
    /// running rail is decided by the gallery.
    func testTheFoldSeamIsTheStorysAlone() throws {
        let s = try source()
        XCTAssertTrue(
            s.contains("init(appModel: AppModel, collapsedSections: Set<RailSection> = [.done])"),
            "a story should be able to seed the fold it is a story about"
        )
        let stories = try source("Sources/NatApp/Gallery/AppStories.swift")
        XCTAssertTrue(stories.contains("collapsedSections: [.active, .todo]"),
                      "the gallery should cover a folded section")
        XCTAssertTrue(stories.contains("collapsedSections: []"),
                      "and all three of them open")
    }

    // MARK: - Elasticity

    /// Every scroll in the app is inelastic: the rubber band says a list has
    /// more to show when it has not, which on a rail of three scrolling
    /// sections is three lies at once. The helper is applied to each
    /// scroll's content, since that is what is inside the `NSScrollView`.
    func testEveryScrollIsInelastic() throws {
        let views = packageRoot().appendingPathComponent("Sources/NatApp/Views")
        let names = try FileManager.default.contentsOfDirectory(atPath: views.path)
            .filter { $0.hasSuffix(".swift") }
            .sorted()
        XCTAssertFalse(names.isEmpty, "there should be views to read")
        var scrolls = 0
        for name in names {
            let text = try String(contentsOf: views.appendingPathComponent(name), encoding: .utf8)
            let opens = text.components(separatedBy: "ScrollView {").count - 1
            guard opens > 0 else { continue }
            scrolls += opens
            XCTAssertEqual(
                text.components(separatedBy: ".inelastic()").count - 1, opens,
                "\(name) should take the elasticity off every scroll it builds"
            )
        }
        XCTAssertGreaterThan(scrolls, 1, "the app should still be building scrolls")
    }
}
