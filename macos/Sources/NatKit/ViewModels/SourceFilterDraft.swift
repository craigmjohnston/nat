import Foundation

/// The filter editor's choices, from the moment it opens on a `filter`
/// action until Apply sends them: one list of option ids per field, opened on
/// what the plugin says is saved (`SourceFilterField.value`). Kept by field
/// id, so a re-read of the tree under an open editor — a loading field
/// filling in — keeps what the user has picked so far. The answer goes back as
/// the action's input: a JSON object of field id to the ids chosen, every
/// field named, an empty list being "Any".
public struct SourceFilterDraft: Equatable, Sendable {
    public private(set) var selection: [String: [String]]

    public init(action: SourceAction) {
        selection = Dictionary(action.fields.map { ($0.id, $0.value) }, uniquingKeysWith: { first, _ in first })
    }

    /// The ids chosen in a field, in the order they were chosen.
    public func selected(_ field: SourceFilterField) -> [String] {
        selection[field.id] ?? field.value
    }

    /// A single-choice field's one choice, nil for "Any".
    public func choice(_ field: SourceFilterField) -> String? {
        selected(field).first
    }

    /// Picks a single-choice field's one option, or "Any" with nil.
    public mutating func choose(_ optionID: String?, in field: SourceFilterField) {
        selection[field.id] = optionID.map { [$0] } ?? []
    }

    /// Adds a multi-choice field's option, or takes it back off.
    public mutating func toggle(_ optionID: String, in field: SourceFilterField) {
        var ids = selected(field)
        if let at = ids.firstIndex(of: optionID) {
            ids.remove(at: at)
        } else {
            ids.append(optionID)
        }
        selection[field.id] = ids
    }

    /// Whether a multi-choice field has an option chosen.
    public func isChosen(_ optionID: String, in field: SourceFilterField) -> Bool {
        selected(field).contains(optionID)
    }

    /// What the field shows: the chosen options by name, else "Any".
    public func summary(_ field: SourceFilterField) -> String {
        let ids = selected(field)
        guard !ids.isEmpty else { return Self.anyLabel(field) }
        return ids.map { id in field.options.first { $0.id == id }?.label ?? id }.joined(separator: ", ")
    }

    /// "Any" as a field offers it: where a wider filter sets the field, what
    /// "Any" falls through to is named, so an override reads as one.
    public static func anyLabel(_ field: SourceFilterField) -> String {
        guard let inherited = field.inherited, !inherited.isEmpty else { return "Any" }
        return "Any (section\u{2019}s: \(inherited))"
    }

    /// Whether anything differs from what was saved when it opened.
    public func differs(from action: SourceAction) -> Bool {
        action.fields.contains { selected($0) != $0.value }
    }

    /// The action's input: `{"<field id>": ["<option id>", …]}`, every one of
    /// the action's fields named, keys sorted so it reads the same each time.
    public func input(for action: SourceAction) -> String {
        let doc = Dictionary(action.fields.map { ($0.id, selected($0)) }, uniquingKeysWith: { first, _ in first })
        let encoder = JSONEncoder()
        encoder.outputFormatting = .sortedKeys
        // A dictionary of strings to string lists: it encodes.
        let data = (try? encoder.encode(doc)) ?? Data("{}".utf8)
        return String(decoding: data, as: UTF8.self)
    }

    /// Whether any field is still loading its options — what the editor
    /// reads the tree again once for.
    public static func isLoading(_ action: SourceAction) -> Bool {
        action.fields.contains(where: \.loading)
    }
}
