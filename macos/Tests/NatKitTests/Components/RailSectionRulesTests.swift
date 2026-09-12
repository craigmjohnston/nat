import XCTest
@testable import NatKit

/// The rail's ACTIVE section is always there. Read off the source the way
/// `ProjectTabStripRulesTests` reads the tab strip: the rail is a SwiftUI
/// view in the app target, which the test target cannot import, so what is
/// checked is that the section and the note under it are still built the way
/// the rule says.
final class RailSectionRulesTests: XCTestCase {
    private func source() throws -> String {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()   // Components
            .deletingLastPathComponent()   // NatKitTests
            .deletingLastPathComponent()   // Tests
            .deletingLastPathComponent()   // the package root
        let url = root.appendingPathComponent("Sources/NatApp/Views/RailView.swift")
        guard FileManager.default.fileExists(atPath: url.path) else {
            throw XCTSkip("no RailView.swift beside the tests")
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
        guard let heading = s.range(of: #"sectionHeading("ACTIVE", icon: "bolt")"#) else {
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
              let end = s.range(of: "private func doneHeadingRow(", range: note.upperBound..<s.endIndex) else {
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

    /// The divider under the flight sections is unconditional now that the
    /// section above it always is: the comment that explained the condition
    /// is gone with it, so neither can be read as still true.
    func testTheFlightDividerAlwaysDraws() throws {
        let s = try source()
        XCTAssertFalse(
            s.contains("if workshopEntry != nil || !railModel.needsReview.isEmpty || !railModel.active.isEmpty"),
            "the flight-sections divider should no longer be conditional"
        )
        XCTAssertFalse(
            s.contains("an empty board opening with a bare line"),
            "the comment about a bare line no longer applies and should not be left behind"
        )
    }
}
