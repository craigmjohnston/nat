import XCTest
@testable import NatKit

final class ModelPickerRulesTests: XCTestCase {
    private let options = ["sonnet", "opus", "haiku"]

    // MARK: - isCustomValue

    func testIsCustomValue_emptyIsNotCustom() {
        XCTAssertFalse(ModelPickerRules.isCustomValue("", options: options))
    }

    func testIsCustomValue_knownAliasIsNotCustom() {
        XCTAssertFalse(ModelPickerRules.isCustomValue("sonnet", options: options))
    }

    func testIsCustomValue_fullModelIDIsCustom() {
        XCTAssertTrue(ModelPickerRules.isCustomValue("claude-sonnet-5", options: options))
    }

    // MARK: - showsCustomField

    func testShowsCustomField_defaultValueNotForced() {
        XCTAssertFalse(ModelPickerRules.showsCustomField(value: "", options: options, forcedCustom: false))
    }

    func testShowsCustomField_knownAliasNotForced() {
        XCTAssertFalse(ModelPickerRules.showsCustomField(value: "sonnet", options: options, forcedCustom: false))
    }

    /// A configured value the picker's own options do not carry selects
    /// Custom on its own, without anything having forced it — the round-trip
    /// a full model ID needs.
    func testShowsCustomField_unrecognisedValueShowsEvenUnforced() {
        XCTAssertTrue(
            ModelPickerRules.showsCustomField(value: "claude-sonnet-5", options: options, forcedCustom: false))
    }

    /// "Custom…" picked with nothing typed yet: `value` is still empty, so
    /// only `forcedCustom` says the field belongs on screen.
    func testShowsCustomField_forcedWithEmptyValueShows() {
        XCTAssertTrue(ModelPickerRules.showsCustomField(value: "", options: options, forcedCustom: true))
    }

    // MARK: - selectionTag

    func testSelectionTag_defaultValue() {
        XCTAssertEqual(ModelPickerRules.selectionTag(value: "", options: options, forcedCustom: false), "")
    }

    func testSelectionTag_knownAlias() {
        XCTAssertEqual(
            ModelPickerRules.selectionTag(value: "sonnet", options: options, forcedCustom: false), "sonnet")
    }

    func testSelectionTag_unrecognisedValueSelectsCustomTag() {
        XCTAssertEqual(
            ModelPickerRules.selectionTag(value: "claude-sonnet-5", options: options, forcedCustom: false),
            ModelPickerRules.customTag
        )
    }

    func testSelectionTag_forcedCustomWithEmptyValueSelectsCustomTag() {
        XCTAssertEqual(
            ModelPickerRules.selectionTag(value: "", options: options, forcedCustom: true),
            ModelPickerRules.customTag
        )
    }
}
