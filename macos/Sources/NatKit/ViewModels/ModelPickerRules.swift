import Foundation

/// `ModelPicker`'s own logic — which tag its `Picker` shows selected, and
/// whether the Custom field beneath it is showing — pulled out of the view
/// so it is testable without mounting SwiftUI.
public enum ModelPickerRules {
    /// The tag "Custom…" is offered under. Not a real model value itself —
    /// picking it never gets written to `value` — so it has to be something
    /// no real alias or model ID could ever collide with.
    public static let customTag = "__custom__"

    /// Whether `value` is a full model ID rather than one of the known
    /// aliases: the field's own escape hatch, since there is no API to
    /// enumerate every alias `claude` accepts (see `AgentOptions.fallback`).
    public static func isCustomValue(_ value: String, options: [String]) -> Bool {
        !value.isEmpty && !options.contains(value)
    }

    /// Whether the free-text Custom field should be visible: either the
    /// stored value is one the picker's own options do not carry, or the
    /// user has just picked "Custom…" and not typed anything into it yet —
    /// `value` alone cannot tell that apart from "Default".
    public static func showsCustomField(value: String, options: [String], forcedCustom: Bool) -> Bool {
        forcedCustom || isCustomValue(value, options: options)
    }

    /// The tag the `Picker` itself should show selected.
    public static func selectionTag(value: String, options: [String], forcedCustom: Bool) -> String {
        showsCustomField(value: value, options: options, forcedCustom: forcedCustom) ? customTag : value
    }
}
