import XCTest
@testable import NatKit

/// The rule that every pane opens the same way, read off the source the way
/// `ViewLayerRulesTests` reads the colour rules.
///
/// The header is a SwiftUI view in the app target, which the test target
/// cannot import — so what is checked here is what can be: the metrics the
/// header is drawn to, which live in the theme, and the fact that no pane
/// assembles a header of its own beside the shared one. Both panes drawing
/// `PaneHeader` is what makes them the same height, the same ground and the
/// same type; a pane that went back to building its own is exactly the
/// regression this catches.
final class PaneHeaderRulesTests: XCTestCase {
    private func source(_ relativePath: String) throws -> String {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()   // Components
            .deletingLastPathComponent()   // NatKitTests
            .deletingLastPathComponent()   // Tests
            .deletingLastPathComponent()   // the package root
        let url = root.appendingPathComponent(relativePath)
        guard FileManager.default.fileExists(atPath: url.path) else {
            throw XCTSkip("no \(relativePath) beside the tests")
        }
        return try String(contentsOf: url, encoding: .utf8)
    }

    private let paneHeader = "Sources/NatApp/Views/PaneHeaderView.swift"
    private let slicePane = "Sources/NatApp/Views/PaneView.swift"
    private let workshopPane = "Sources/NatApp/Views/WorkshopPaneView.swift"
    private let rail = "Sources/NatApp/Views/RailView.swift"

    /// The floor holds the identity block the slice pane's header is measured
    /// by — breadcrumb, gap, title — plus the insets either side of it, so a
    /// header with no breadcrumb on it opens no shorter than one with.
    func testMinHeightHoldsTheFullIdentityBlock() {
        let identity = Typo.caption + PaneHeaderMetrics.identitySpacing + Typo.headline
        let insets = PaneHeaderMetrics.verticalPadding * 2
        XCTAssertGreaterThanOrEqual(
            PaneHeaderMetrics.minHeight, identity + insets,
            "a breadcrumb, a title and the insets must fit inside the header's own floor"
        )
    }

    /// Every one of them is a real number: a zero here would be a header with
    /// no inset or no floor at all, which is the shape the workshop pane's
    /// bare title row had.
    func testMetricsArePositive() {
        XCTAssertGreaterThan(PaneHeaderMetrics.verticalPadding, 0)
        XCTAssertGreaterThan(PaneHeaderMetrics.horizontalPadding, 0)
        XCTAssertGreaterThan(PaneHeaderMetrics.spacing, 0)
        XCTAssertGreaterThan(PaneHeaderMetrics.identitySpacing, 0)
        XCTAssertGreaterThan(PaneHeaderMetrics.minHeight, 0)
    }

    /// Both panes open with the shared header rather than one of their own.
    func testBothPanesDrawTheSharedHeader() throws {
        for path in [slicePane, workshopPane] {
            XCTAssertTrue(
                try source(path).contains("PaneHeader("),
                "\(path): a pane opens with the shared PaneHeader"
            )
        }
    }

    /// The header is drawn to the metrics rather than to numbers typed into
    /// it, which is what keeps the two panes from drifting apart again.
    func testTheHeaderIsDrawnToTheMetrics() throws {
        let header = try source(paneHeader)
        for metric in ["verticalPadding", "horizontalPadding", "spacing", "identitySpacing", "minHeight"] {
            XCTAssertTrue(
                header.contains("PaneHeaderMetrics.\(metric)"),
                "PaneHeader should be drawn to PaneHeaderMetrics.\(metric)"
            )
        }
    }

    /// The one deliberate change of ground: the header sits on the window,
    /// the ground the rail beside it sits on, rather than on the `.band` the
    /// slice pane's header used to paint.
    func testTheHeaderSitsOnTheRailsOwnGround() throws {
        XCTAssertTrue(
            try source(paneHeader).contains(".surface(.window)"),
            "the pane header is painted on the window's ground"
        )
        XCTAssertTrue(
            try source(rail).contains(".surface(.window)"),
            "the rail is painted on the window's ground — the header matches it"
        )
        XCTAssertFalse(
            try source(paneHeader).contains(".surface(.band)"),
            "the band is the ground the shared header deliberately left behind"
        )
        XCTAssertFalse(
            try source(slicePane).contains(".surface(.band)"),
            "the slice pane's header no longer paints its own band"
        )
    }

    /// Nothing of the workshop pane's old title row survives: neither its
    /// hand-set 46pt height nor the bare `Rule()` it hung under itself.
    func testNoBareTitleRowRemainsInTheWorkshopPane() throws {
        let workshop = try source(workshopPane)
        XCTAssertFalse(workshop.contains("frame(height: 46)"), "the 46pt title row is gone")
        XCTAssertFalse(workshop.contains("Rule()"), "the title row's own rule is the header's hairline now")
    }
}
