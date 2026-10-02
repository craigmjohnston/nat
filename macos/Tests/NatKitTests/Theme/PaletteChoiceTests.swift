import XCTest
@testable import NatKit

final class PaletteChoiceTests: XCTestCase {
    func testEachSlotOffersItsOwnPalettes() {
        XCTAssertEqual(PaletteChoice.choices(dark: true), [.iceberg, .slateInk])
        XCTAssertEqual(PaletteChoice.choices(dark: false), [.light, .oneLight, .tokyoDay])
    }

    func testTheDefaultsSitInTheirOwnSlots() {
        XCTAssertTrue(PaletteChoice.defaultDark.palette.isDark)
        XCTAssertFalse(PaletteChoice.defaultLight.palette.isDark)
    }

    func testEachChoiceIsItsPalette() {
        XCTAssertEqual(PaletteChoice.light.palette, .light)
        XCTAssertEqual(PaletteChoice.oneLight.palette, .oneLight)
        XCTAssertEqual(PaletteChoice.tokyoDay.palette, .tokyoDay)
        XCTAssertEqual(PaletteChoice.iceberg.palette, .iceberg)
        XCTAssertEqual(PaletteChoice.slateInk.palette, .slateInk)
    }

    func testTitlesAreDistinctAndNonEmpty() {
        let titles = PaletteChoice.allCases.map(\.title)
        XCTAssertFalse(titles.contains(""))
        XCTAssertEqual(Set(titles).count, titles.count)
        XCTAssertEqual(PaletteChoice.slateInk.id, "slateInk")
    }

    func testTheSlotKeysDiffer() {
        XCTAssertNotEqual(PaletteChoice.darkStorageKey, PaletteChoice.lightStorageKey)
        XCTAssertNotEqual(PaletteChoice.darkStorageKey, Theme.storageKey)
    }

    func testAStoredChoiceRoundTrips() {
        XCTAssertEqual(PaletteChoice(stored: "slateInk", dark: true), .slateInk)
        XCTAssertEqual(PaletteChoice(stored: "light", dark: false), .light)
        XCTAssertEqual(PaletteChoice(stored: "tokyoDay", dark: false), .tokyoDay)
    }

    /// An unwritten key, a palette a later build removed, and a palette of
    /// the other scheme all read back as the slot's default.
    func testAnythingElseIsTheSlotsDefault() {
        XCTAssertEqual(PaletteChoice(stored: nil, dark: true), .iceberg)
        XCTAssertEqual(PaletteChoice(stored: "mocha", dark: true), .iceberg)
        XCTAssertEqual(PaletteChoice(stored: "light", dark: true), .iceberg)
        XCTAssertEqual(PaletteChoice(stored: nil, dark: false), .oneLight)
        XCTAssertEqual(PaletteChoice(stored: "iceberg", dark: false), .oneLight)
    }

    /// A selection puts each palette in its own slot and leaves the other.
    func testSelectingFillsOnlyItsOwnSlot() {
        let selection = PaletteSelection()
        XCTAssertEqual(selection.palette(dark: true), .iceberg)
        XCTAssertEqual(selection.palette(dark: false), .oneLight)
        selection.select(.slateInk)
        XCTAssertEqual(selection.palette(dark: true), .slateInk)
        XCTAssertEqual(selection.palette(dark: false), .oneLight)
        selection.select(.tokyoDay)
        XCTAssertEqual(selection.palette(dark: true), .slateInk)
        XCTAssertEqual(selection.palette(dark: false), .tokyoDay)
    }
}
