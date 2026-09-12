import Foundation

/// What a link clicked in the agent terminal resolves to, if anything.
///
/// The terminal's emulator finds two kinds of link: an explicit one, from an
/// OSC 8 hyperlink the agent emitted, and an implicit one, a URL the
/// emulator's own detector picked out of plain output. Either arrives as a
/// bare string, and this is the one place that says what opening it means —
/// so the view bridge is left with `NSWorkspace.shared.open` and no
/// judgement of its own.
public enum TerminalLink {
    /// The schemes a click is allowed to open.
    ///
    /// An allowlist rather than "whatever has a scheme": the text in an
    /// agent's pane is written by a model and by whatever it ran, and a
    /// click is one gesture away from handing an arbitrary scheme to
    /// whichever app on the Mac registered it. These four are what a URL an
    /// agent prints actually is — a page, a mail address, or a file.
    public static let openableSchemes: Set<String> = ["http", "https", "mailto", "file"]

    /// The URL `link` should open, or nil for one that names nothing this
    /// app will open.
    ///
    /// A string with a scheme is taken as a URL and refused unless the
    /// scheme is one of `openableSchemes`. Anything else is read as a
    /// filesystem path — which is what an OSC 8 payload naming a file
    /// without the `file:` scheme is — and has to be absolute and actually
    /// there: a relative path would be opened against whatever directory
    /// the app happens to be running in, which is nowhere the agent meant.
    public static func destination(
        _ link: String,
        fileExists: (String) -> Bool = { FileManager.default.fileExists(atPath: $0) }
    ) -> URL? {
        let trimmed = link.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            return nil
        }

        if let url = URL(string: trimmed), let scheme = url.scheme {
            return openableSchemes.contains(scheme.lowercased()) ? url : nil
        }

        let path = (trimmed as NSString).expandingTildeInPath
        guard path.hasPrefix("/"), fileExists(path) else {
            return nil
        }
        return URL(fileURLWithPath: path)
    }
}
