import XCTest
import SwiftUI
@testable import NatKit

final class ColorSchemeReportTests: XCTestCase {
    /// `CSI ?997;1n`/`?997;2n`, verified empirically against a real `claude`
    /// process (see `ColorSchemeReport`'s own doc) — written out as escapes
    /// here rather than compared to a constant defined the same way, so a
    /// typo in either digit is the failure rather than agreeing with itself.
    func testDarkIsReportedAs997_1() {
        XCTAssertEqual(ColorSchemeReport.escape(for: .dark), "\u{1b}[?997;1n")
    }

    func testLightIsReportedAs997_2() {
        XCTAssertEqual(ColorSchemeReport.escape(for: .light), "\u{1b}[?997;2n")
    }

    // MARK: - Whether a report is worth sending

    /// No prior reading at all is the terminal host's own first `updateNSView`
    /// — `theme: "auto"` already probed for itself against the palette
    /// `makeNSView` applied before the attach process ever started, so a
    /// report here would tell Claude Code nothing new.
    func testNoPriorReadingNeverReports() {
        XCTAssertFalse(ColorSchemeReport.shouldReport(from: nil, to: .dark, attached: true))
        XCTAssertFalse(ColorSchemeReport.shouldReport(from: nil, to: .light, attached: true))
    }

    /// The same scheme seen again — an unrelated SwiftUI update running
    /// `updateNSView` again — is not a change worth reporting.
    func testAnUnchangedSchemeDoesNotReport() {
        XCTAssertFalse(ColorSchemeReport.shouldReport(from: .dark, to: .dark, attached: true))
        XCTAssertFalse(ColorSchemeReport.shouldReport(from: .light, to: .light, attached: true))
    }

    /// An actual change, with the attach live to receive it, is exactly what
    /// this exists for.
    func testAnActualChangeWhileAttachedReports() {
        XCTAssertTrue(ColorSchemeReport.shouldReport(from: .dark, to: .light, attached: true))
        XCTAssertTrue(ColorSchemeReport.shouldReport(from: .light, to: .dark, attached: true))
    }

    /// A change is only worth reporting where something is actually attached
    /// to receive it — typed at a session still starting, or one already
    /// gone, it has nowhere useful to land.
    func testAChangeWhileNotAttachedDoesNotReport() {
        XCTAssertFalse(ColorSchemeReport.shouldReport(from: .dark, to: .light, attached: false))
    }
}
