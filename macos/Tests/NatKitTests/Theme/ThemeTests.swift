import XCTest
import AppKit
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

    /// `light`/`dark` answer for themselves, whatever appearance is handed
    /// in — the whole point of choosing one outright rather than `system`.
    @MainActor
    func testCLIValueForLightAndDarkIgnoresTheAppearance() {
        XCTAssertEqual(Theme.light.cliValue(appearance: NSAppearance(named: .darkAqua)), "light")
        XCTAssertEqual(Theme.dark.cliValue(appearance: NSAppearance(named: .aqua)), "dark")
        XCTAssertEqual(Theme.light.cliValue(appearance: nil), "light")
        XCTAssertEqual(Theme.dark.cliValue(appearance: nil), "dark")
    }

    /// `system` resolves through whichever `NSAppearance` it is handed —
    /// same rule `DesignTokens.palette(for:)` resolves every dynamic colour
    /// with, so a headless `nat --theme` call never disagrees with the
    /// window on screen.
    @MainActor
    func testCLIValueForSystemResolvesTheGivenAppearance() {
        XCTAssertEqual(Theme.system.cliValue(appearance: NSAppearance(named: .darkAqua)), "dark")
        XCTAssertEqual(Theme.system.cliValue(appearance: NSAppearance(named: .aqua)), "light")
    }

    /// No appearance to read at all is nil, not a guess — every launch
    /// treats a nil theme exactly as it always did: no override.
    @MainActor
    func testCLIValueForSystemWithNoAppearanceIsNil() {
        XCTAssertNil(Theme.system.cliValue(appearance: nil))
    }
}
