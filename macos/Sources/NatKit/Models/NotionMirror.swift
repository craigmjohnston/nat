import Foundation

/// Somewhere in the Notion workspace a project page can be put: what
/// `nat notion-search --json` lists. `id` is what `nat project-mirror --parent`
/// takes — a page's own ID, or a database's data source ID.
public struct NotionPlace: Equatable, Sendable, Decodable, Identifiable {
    public enum Kind: String, Sendable, Decodable {
        case page
        case database
    }

    public let id: String
    public let kind: Kind
    public let title: String

    public init(id: String, kind: Kind, title: String) {
        self.id = id
        self.kind = kind
        self.title = title
    }
}

/// `nat notion-search --json`'s answer, wrapped the way every listing is.
struct NotionSearchDoc: Decodable {
    let places: [NotionPlace]
}

/// What `nat project-mirror --json` reports: the project as Notion now knows
/// it — the page ID every later `--project` takes — the local project's ID it
/// replaces, and how much of the plan went in.
public struct ProjectMirrored: Equatable, Sendable, Decodable {
    public let project: ProjectEntry
    public let replaced: String
    public let milestones: Int
    public let slices: Int

    public init(project: ProjectEntry, replaced: String, milestones: Int, slices: Int) {
        self.project = project
        self.replaced = replaced
        self.milestones = milestones
        self.slices = slices
    }
}

/// The words of the mirror nudge and its picker, written once — the mock is
/// `NFNudgeCard` and `NFNotionPicker` in
/// `docs/design/nat-new-project/ui-npflow.jsx`.
public enum MirrorText {
    public static let cardTitle = "Mirror this plan to Notion?"
    public static let cardBody =
        "The plan stays local either way — a Notion page keeps it in sync, so anyone on the project can read and edit it."
    public static let choosePage = "Choose page\u{2026}"
    public static let dismiss = "Dismiss"

    public static let pickerTitle = "Create Notion project page"
    public static let pickerSubtitle =
        "Choose where the project page goes. The plan mirrors there as it changes."
    public static let searchPrompt = "Search your workspace"
    public static let databaseChip = "database"
    public static let create = "Create page"
    public static let nothingFound = "Nothing in the workspace matches."
}
