import XCTest
import SwiftUI
@testable import NatKit

final class ThemeTests: XCTestCase {
    /// Three states, and the one that needs no explaining is the default:
    /// anything that is not one of them — a key never written, or one left
    /// by a build that named its options differently — reads as `system`.
    func testStoredValueRoundTripsAndFallsBackToSystem() {
        for theme in Theme.allCases {
            XCTAssertEqual(Theme(stored: theme.rawValue), theme)
            XCTAssertEqual(theme.id, theme.rawValue)
            XCTAssertFalse(theme.title.isEmpty)
        }
        XCTAssertEqual(Theme(stored: nil), .system)
        XCTAssertEqual(Theme(stored: ""), .system)
        XCTAssertEqual(Theme(stored: "sepia"), .system)
        XCTAssertEqual(Theme.allCases, [.system, .dark, .light])
    }

    /// `system` pins nothing, which is the whole of what it means: an
    /// unpinned window follows the Mac's appearance and goes on following it
    /// as that changes. The other two say which.
    func testColorSchemePinsOnlyTheChosenThemes() {
        XCTAssertNil(Theme.system.colorScheme)
        XCTAssertEqual(Theme.dark.colorScheme, .dark)
        XCTAssertEqual(Theme.light.colorScheme, .light)
    }

    /// Each option is named differently, since the switcher is three labels
    /// side by side.
    func testTitlesAreDistinct() {
        XCTAssertEqual(Set(Theme.allCases.map(\.title)).count, Theme.allCases.count)
    }
}
