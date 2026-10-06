import Foundation

/// A project's colour, by the name nat keeps on its config entry
/// (`config.ProjectColors`, in nat's order). A name, never a value: each
/// `Palette` resolves it (`projectTint`), so a project's colour follows light
/// and dark. nat chooses one for every entry; gnat only ever asks it to
/// (`config-set project.<id>.color auto`) or writes the user's pick.
public enum ProjectColor: String, CaseIterable, Codable, Equatable, Sendable {
    case red, orange, yellow, green, teal, blue, purple, pink

    /// The colour a config entry's word names; nil for none, and for a word
    /// this build does not know — a config that will not parse is worse than a
    /// project drawn with the quiet badge.
    public init?(word: String?) {
        guard let word, let color = ProjectColor(rawValue: word) else { return nil }
        self = color
    }
}
