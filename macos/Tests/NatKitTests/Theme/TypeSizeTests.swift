import XCTest
@testable import NatKit

final class TypeSizeTests: XCTestCase {
    /// Every test here moves the process-wide sizes; each puts the defaults
    /// back, so no other test draws at a size it did not ask for.
    override func tearDown() {
        TypeSizeSelection.shared.select(.default)
        super.tearDown()
    }

    // MARK: - TypeSize

    func testDefaultsAreFourteenAndFourteen() {
        XCTAssertEqual(TypeSize.default, TypeSize(ui: 14, mono: 14))
        XCTAssertEqual(TypeSizeSelection.shared.current, .default)
    }

    func testTheStorageKeysDiffer() {
        XCTAssertNotEqual(TypeSize.uiStorageKey, TypeSize.monoStorageKey)
    }

    func testOutOfRangeValuesClampToTheNearestEnd() {
        XCTAssertEqual(TypeSize(ui: 2, mono: 2), TypeSize(ui: TypeSize.uiRange.lowerBound, mono: TypeSize.monoRange.lowerBound))
        XCTAssertEqual(TypeSize(ui: 99, mono: 99), TypeSize(ui: TypeSize.uiRange.upperBound, mono: TypeSize.monoRange.upperBound))
        XCTAssertEqual(TypeSize(ui: 16, mono: 18).ui, 16)
        XCTAssertEqual(TypeSize(ui: 16, mono: 18).mono, 18)
    }

    func testIdentityNamesBothSizes() {
        XCTAssertEqual(TypeSize(ui: 16, mono: 12).identity, "16/12")
        XCTAssertNotEqual(TypeSize(ui: 16, mono: 12).identity, TypeSize(ui: 12, mono: 16).identity)
    }

    // MARK: - The ramp

    /// At the defaults the app draws exactly as it did before there was a
    /// setting — only the terminal and the diff move, a point up.
    func testDefaultsDrawTheRampAsItWas() {
        XCTAssertEqual(Typo.headline, 15)
        XCTAssertEqual(Typo.body, 14)
        XCTAssertEqual(Typo.subhead, 12)
        XCTAssertEqual(Typo.caption, 11)
        XCTAssertEqual(Typo.code, 13)
        XCTAssertEqual(Typo.codeView, 14)
        XCTAssertEqual(Typo.codeView(12), 13)
    }

    /// The UI size is the body's own size, and the rest of the ramp keeps
    /// its proportions, rounded to whole points.
    func testUISizeScalesTheProportionalRamp() {
        TypeSizeSelection.shared.select(TypeSize(ui: 18, mono: 14))
        XCTAssertEqual(Typo.body, 18)
        XCTAssertEqual(Typo.headline, 19)   // 15 × 18/14 = 19.29
        XCTAssertEqual(Typo.subhead, 15)    // 12 × 18/14 = 15.43
        XCTAssertEqual(Typo.caption, 14)    // 11 × 18/14 = 14.14
    }

    /// Each field moves only its own half: code ignores the UI size, and
    /// the proportional ramp ignores the code size.
    func testTheTwoSizesAreIndependent() {
        TypeSizeSelection.shared.select(TypeSize(ui: 18, mono: 14))
        XCTAssertEqual(Typo.codeView, 14)
        XCTAssertEqual(Typo.code, 13)

        TypeSizeSelection.shared.select(TypeSize(ui: 14, mono: 20))
        XCTAssertEqual(Typo.codeView, 20)
        XCTAssertEqual(Typo.codeView(12), 18) // 12 × 20/13 = 18.46
        XCTAssertEqual(Typo.body, 14)
        XCTAssertEqual(Typo.headline, 15)
        XCTAssertEqual(Typo.code, 13)
    }

    func testTerminalFollowsTheCodeSize() {
        TypeSizeSelection.shared.select(TypeSize(ui: 14, mono: 17))
        XCTAssertEqual(TerminalType.font.pointSize, 17)
    }

    // MARK: - The diff's geometry

    /// At the size the defaults were drawn for, nothing moves.
    func testDiffMetricsAtThirteenAreTheDefaults() {
        XCTAssertEqual(DiffMetrics(codeSize: 13), DiffMetrics())
    }

    /// A line, a row and a gutter digit grow with the face; the header band
    /// and the padding are the window's and do not.
    func testDiffMetricsGrowWithTheCodeSize() {
        let metrics = DiffMetrics(codeSize: 26)
        XCTAssertEqual(metrics.lineHeight, 38)
        XCTAssertEqual(metrics.rowMinHeight, 40)
        XCTAssertEqual(metrics.digitWidth, 17.8, accuracy: 0.0001)
        XCTAssertEqual(metrics.headerHeight, DiffMetrics().headerHeight)
        XCTAssertEqual(metrics.rowPadding, DiffMetrics().rowPadding)
    }

    /// The default code size's line holds the face it draws: JetBrains Mono
    /// at 14 is taller than the 19 a 13-point line was given.
    func testDefaultDiffLineHoldsItsFace() {
        let metrics = DiffMetrics(codeSize: Typo.codeView)
        let font = Typo.monoNSFont(size: Typo.codeView)
        XCTAssertEqual(metrics.lineHeight, 20)
        XCTAssertGreaterThanOrEqual(metrics.lineHeight, ceil(font.ascender - font.descender))
    }
}
