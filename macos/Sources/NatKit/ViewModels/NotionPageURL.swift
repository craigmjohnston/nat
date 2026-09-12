import Foundation

/// Where a Notion page lives on the web, built from the page ID alone.
///
/// A slice carries its own `url` — `nat info` reads it off the page — but a
/// project carries only its ID, and the context menus offer "Open in Notion"
/// on both. Notion resolves `notion.so/<the id with its dashes stripped>` to
/// the page whatever its title is, so the ID is the whole of what a link
/// needs; building one here rather than teaching `nat info` a second field
/// keeps the CLI's reading of a project as it was.
public enum NotionPageURL {
    /// The page's URL, or nil for anything that is not a page ID: an empty
    /// string, or one holding characters a Notion ID never does. A nil is
    /// what a menu drops the item on rather than offering a link that opens
    /// a search page.
    public static func forPage(_ pageID: String) -> URL? {
        let compact = pageID
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: "-", with: "")
        guard !compact.isEmpty else { return nil }
        guard compact.allSatisfy({ $0.isHexDigit }) else { return nil }
        return URL(string: "https://www.notion.so/\(compact)")
    }
}
