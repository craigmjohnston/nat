import XCTest

/// The shape of the project tab strip, read off the source the way
/// `PaneHeaderRulesTests` reads the pane headers: the strip is a SwiftUI view
/// in the app target, which the test target cannot import, so what is checked
/// here is what can be — that the modifiers carrying each rule are still on
/// the elements they are rules about.
final class ProjectTabStripRulesTests: XCTestCase {
    private func source() throws -> String {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()   // Components
            .deletingLastPathComponent()   // NatKitTests
            .deletingLastPathComponent()   // Tests
            .deletingLastPathComponent()   // the package root
        let url = root.appendingPathComponent("Sources/NatApp/Views/ProjectTabsView.swift")
        guard FileManager.default.fileExists(atPath: url.path) else {
            throw XCTSkip("no ProjectTabsView.swift beside the tests")
        }
        return try String(contentsOf: url, encoding: .utf8)
    }

    /// The close button sits at the tab's trailing edge, whatever the tab's
    /// width: a Spacer between the label block and the ✕ is what puts it
    /// there, and what keeps it there as the tab stretches to its 220pt
    /// maximum. Without one the ✕ sat hard against the label, leaving the
    /// whole right of the tab — where a click aiming at it lands — activating
    /// the tab instead.
    func testCloseButtonIsPushedToTheTrailingEdge() throws {
        let s = try source()
        guard let spacer = s.range(of: "Spacer(minLength: 0)"),
              let xmark = s.range(of: #"Image(systemName: "xmark")"#) else {
            return XCTFail("the tab should hold a Spacer and an xmark")
        }
        XCTAssertTrue(
            spacer.lowerBound < xmark.lowerBound,
            "the Spacer must precede the ✕, or the button is not at the tab's trailing edge"
        )
    }

    /// What the ✕ is drawn at and what a click can reach are one answer.
    /// Drawn at `opacity(0)` alone it stayed a live hit target sitting exactly
    /// where a click meaning to select the tab lands.
    func testTheHiddenCloseButtonIsNotAHitTarget() throws {
        let s = try source()
        XCTAssertTrue(
            s.contains(".opacity(showClose ? 1 : 0)"),
            "the ✕ should fade on the shared rule rather than an expression of its own"
        )
        XCTAssertTrue(
            s.contains(".allowsHitTesting(showClose)"),
            "the ✕ must be unreachable by a click while it is faded out"
        )
    }

    /// Both of the button's rules are NatKit's, not expressions inlined here.
    func testTheCloseButtonAsksTheSharedRules() throws {
        let s = try source()
        XCTAssertTrue(s.contains("ProjectTabRules.showsClose(tabCount:"))
        XCTAssertTrue(s.contains("ProjectTabRules.closeIsVisible("))
    }

    /// A tab is a flat cell filling the band, hard against its neighbours:
    /// no rounded silhouette, no flare, and no bottom padding lifting it off
    /// the band's foot.
    func testATabIsAFlatFullHeightCell() throws {
        let s = try source()
        XCTAssertFalse(
            s.contains("BrowserTabShape"),
            "the curved browser silhouette is gone — a tab is a plain rectangle"
        )
        XCTAssertTrue(
            s.contains(".frame(height: 40)"),
            "a tab should fill the 40pt band rather than being seated on its foot"
        )
    }

    /// Nothing is inserted between two tabs but the hairline where they meet,
    /// and it runs the band's full height like the cells either side of it.
    func testTabsAbutWithAFullHeightRule() throws {
        let s = try source()
        XCTAssertTrue(
            s.contains("HStack(alignment: .bottom, spacing: 0)"),
            "the strip should lay its tabs out with no spacing between them"
        )
        XCTAssertTrue(
            s.contains(".frame(width: 1, height: 40)"),
            "the rule between two tabs should run the band's full height"
        )
    }
}
