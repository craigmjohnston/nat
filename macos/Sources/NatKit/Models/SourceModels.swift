import Foundation

// The task-source shapes, mirrored field for field from
// `docs/design/task-sources/README.md` — the plugin protocol's shared types
// ("Wire contract") as `nat` passes them through, and the `--json` envelopes
// of the commands that carry them ("The `nat` contract"). Every decode is
// lenient: a missing optional is nil or empty, and an `input` or `kind` this
// build does not know is kept as its raw word, so one odd field from a plugin
// never fails a whole read.

extension KeyedDecodingContainer {
    /// A string that reads as empty where it is absent, null or not a string.
    fileprivate func lenientString(_ key: Key) -> String {
        ((try? decodeIfPresent(String.self, forKey: key)) ?? nil) ?? ""
    }

    /// A string that reads as nil where it is absent, null, empty or not a
    /// string — the protocol's own "optional", where `""` means none too.
    fileprivate func optionalString(_ key: Key) -> String? {
        let value = lenientString(key)
        return value.isEmpty ? nil : value
    }

    fileprivate func list<T: Decodable>(_ type: T.Type, _ key: Key) throws -> [T] {
        try decodeIfPresent([T].self, forKey: key) ?? []
    }
}

/// How an action asks for its input: `none` runs on click, `text` asks for a
/// line of text, `choice` offers its `options`.
public enum SourceActionInput: Equatable, Sendable, Codable {
    case none, text, choice
    /// An input word this build does not know, kept as given.
    case unknown(String)

    public init(word: String) {
        switch word {
        case "", "none": self = .none
        case "text": self = .text
        case "choice": self = .choice
        default: self = .unknown(word)
        }
    }

    public var word: String {
        switch self {
        case .none: "none"
        case .text: "text"
        case .choice: "choice"
        case .unknown(let word): word
        }
    }

    public init(from decoder: Decoder) throws {
        self.init(word: (try? decoder.singleValueContainer().decode(String.self)) ?? "")
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        try c.encode(word)
    }
}

/// One of a plugin's own actions — a menu item, a section's composer.
public struct SourceAction: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let label: String
    public let input: SourceActionInput
    public let options: [String]
    public let destructive: Bool

    enum CodingKeys: String, CodingKey { case id, label, input, options, destructive }

    public init(id: String, label: String, input: SourceActionInput = .none, options: [String] = [], destructive: Bool = false) {
        self.id = id
        self.label = label
        self.input = input
        self.options = options
        self.destructive = destructive
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.lenientString(.id)
        label = c.lenientString(.label)
        input = SourceActionInput(word: c.lenientString(.input))
        options = (try? c.list(String.self, .options)) ?? []
        destructive = ((try? c.decodeIfPresent(Bool.self, forKey: .destructive)) ?? nil) ?? false
    }
}

/// A short coloured tag drawn after a container's title.
public struct SourceBadge: Codable, Equatable, Sendable {
    public let text: String
    /// `#rrggbb`; one that will not parse is drawn in secondary ink.
    public let color: String
    public let title: String?

    enum CodingKeys: String, CodingKey { case text, color, title }

    public init(text: String, color: String, title: String? = nil) {
        self.text = text
        self.color = color
        self.title = title
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        text = c.lenientString(.text)
        color = c.lenientString(.color)
        title = c.optionalString(.title)
    }
}

/// One container row of the sidebar tree — a card, a story.
public struct SourceContainer: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let title: String
    public let externalURL: String?
    public let badges: [SourceBadge]
    public let meta: String?
    public let menu: [SourceAction]

    enum CodingKeys: String, CodingKey {
        case id, title
        case externalURL = "external_url"
        case badges, meta, menu
    }

    public init(
        id: String, title: String, externalURL: String? = nil, badges: [SourceBadge] = [],
        meta: String? = nil, menu: [SourceAction] = []
    ) {
        self.id = id
        self.title = title
        self.externalURL = externalURL
        self.badges = badges
        self.meta = meta
        self.menu = menu
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.lenientString(.id)
        title = c.lenientString(.title)
        externalURL = c.optionalString(.externalURL)
        badges = try c.list(SourceBadge.self, .badges)
        meta = c.optionalString(.meta)
        menu = try c.list(SourceAction.self, .menu)
    }
}

/// A group of the sidebar tree: either child groups (one level) or
/// containers. A `lazy` group lists its containers only once expanded.
public struct SourceGroup: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let label: String
    /// The plugin's own number, drawn as-is; never computed by nat.
    public let count: Int?
    public let lazy: Bool
    public let menu: [SourceAction]
    public let children: [SourceGroup]
    public let containers: [SourceContainer]

    enum CodingKeys: String, CodingKey { case id, label, count, lazy, menu, children, containers }

    /// nat's own group of containers with tasks that appear nowhere else in
    /// the tree.
    public static let unlistedID = "_unlisted"

    public init(
        id: String, label: String, count: Int? = nil, lazy: Bool = false, menu: [SourceAction] = [],
        children: [SourceGroup] = [], containers: [SourceContainer] = []
    ) {
        self.id = id
        self.label = label
        self.count = count
        self.lazy = lazy
        self.menu = menu
        self.children = children
        self.containers = containers
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.lenientString(.id)
        label = c.lenientString(.label)
        count = (try? c.decodeIfPresent(Int.self, forKey: .count)) ?? nil
        lazy = ((try? c.decodeIfPresent(Bool.self, forKey: .lazy)) ?? nil) ?? false
        menu = try c.list(SourceAction.self, .menu)
        children = try c.list(SourceGroup.self, .children)
        containers = try c.list(SourceContainer.self, .containers)
    }
}

/// `info --json`'s `source` block: who the plugin is, its header menu, and
/// the tree its sidebar section draws. `error` carries a failed plugin read,
/// when `groups` is `_unlisted` alone.
public struct SourceInfo: Codable, Equatable, Sendable {
    public let name: String
    public let title: String
    public let tag: String
    public let iconSymbol: String
    public let iconSVG: String?
    public let containerNoun: String
    public let taskNoun: String
    public let menu: [SourceAction]
    public let groups: [SourceGroup]
    public let error: String?

    enum CodingKeys: String, CodingKey {
        case name, title, tag
        case iconSymbol = "icon_symbol"
        case iconSVG = "icon_svg"
        case containerNoun = "container_noun"
        case taskNoun = "task_noun"
        case menu, groups, error
    }

    public init(
        name: String, title: String, tag: String, iconSymbol: String, iconSVG: String? = nil,
        containerNoun: String, taskNoun: String, menu: [SourceAction] = [], groups: [SourceGroup] = [],
        error: String? = nil
    ) {
        self.name = name
        self.title = title
        self.tag = tag
        self.iconSymbol = iconSymbol
        self.iconSVG = iconSVG
        self.containerNoun = containerNoun
        self.taskNoun = taskNoun
        self.menu = menu
        self.groups = groups
        self.error = error
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = c.lenientString(.name)
        title = c.lenientString(.title)
        tag = c.lenientString(.tag)
        iconSymbol = c.lenientString(.iconSymbol)
        iconSVG = c.optionalString(.iconSVG)
        containerNoun = c.lenientString(.containerNoun)
        taskNoun = c.lenientString(.taskNoun)
        menu = try c.list(SourceAction.self, .menu)
        groups = try c.list(SourceGroup.self, .groups)
        error = c.optionalString(.error)
    }

    /// Every container in the tree, depth-first in drawing order, each once:
    /// a container listed under several groups is one container, and its
    /// first appearance is the one kept.
    public var allContainers: [SourceContainer] {
        var seen = Set<String>()
        var result: [SourceContainer] = []
        func walk(_ groups: [SourceGroup]) {
            for group in groups {
                for container in group.containers where seen.insert(container.id).inserted {
                    result.append(container)
                }
                walk(group.children)
            }
        }
        walk(groups)
        return result
    }

    /// The group with `id`, at any depth.
    public func group(withID id: String) -> SourceGroup? {
        func find(_ groups: [SourceGroup]) -> SourceGroup? {
            for group in groups {
                if group.id == id { return group }
                if let found = find(group.children) { return found }
            }
            return nil
        }
        return find(groups)
    }
}

/// One label/value line of a container's facts.
public struct SourceFact: Codable, Equatable, Sendable {
    public let label: String
    public let value: String
    /// `#rrggbb`, tinting the value's leading dot.
    public let color: String?

    enum CodingKeys: String, CodingKey { case label, value, color }

    public init(label: String, value: String, color: String? = nil) {
        self.label = label
        self.value = value
        self.color = color
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        label = c.lenientString(.label)
        value = c.lenientString(.value)
        color = c.optionalString(.color)
    }
}

/// One comment of a `comments` section; `when` is free text.
public struct SourceComment: Codable, Equatable, Sendable {
    public let by: String
    public let when: String
    public let text: String

    enum CodingKeys: String, CodingKey { case by, when, text }

    public init(by: String, when: String, text: String) {
        self.by = by
        self.when = when
        self.text = text
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        by = c.lenientString(.by)
        when = c.lenientString(.when)
        text = c.lenientString(.text)
    }
}

/// One link of a `links` section; `state` is free text drawn as a pill.
public struct SourceLink: Codable, Equatable, Sendable {
    public let label: String
    public let text: String
    public let state: String?
    public let url: String?

    enum CodingKeys: String, CodingKey { case label, text, state, url }

    public init(label: String, text: String, state: String? = nil, url: String? = nil) {
        self.label = label
        self.text = text
        self.state = state
        self.url = url
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        label = c.lenientString(.label)
        text = c.lenientString(.text)
        state = c.optionalString(.state)
        url = c.optionalString(.url)
    }
}

/// What a container section draws; an unknown kind is kept and skipped.
public enum SourceSectionKind: Equatable, Sendable, Codable {
    case prose, comments, links
    case unknown(String)

    public init(word: String) {
        switch word {
        case "prose": self = .prose
        case "comments": self = .comments
        case "links": self = .links
        default: self = .unknown(word)
        }
    }

    public var word: String {
        switch self {
        case .prose: "prose"
        case .comments: "comments"
        case .links: "links"
        case .unknown(let word): word
        }
    }

    public init(from decoder: Decoder) throws {
        self.init(word: (try? decoder.singleValueContainer().decode(String.self)) ?? "")
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        try c.encode(word)
    }
}

/// One section of a container's detail.
public struct SourceSection: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let title: String
    public let kind: SourceSectionKind
    /// Markdown, for a `prose` section.
    public let body: String?
    public let comments: [SourceComment]
    public let links: [SourceLink]
    /// A `text` action drawn as a compose box under a `comments` section.
    public let composer: SourceAction?

    enum CodingKeys: String, CodingKey { case id, title, kind, body, comments, links, composer }

    public init(
        id: String, title: String, kind: SourceSectionKind, body: String? = nil,
        comments: [SourceComment] = [], links: [SourceLink] = [], composer: SourceAction? = nil
    ) {
        self.id = id
        self.title = title
        self.kind = kind
        self.body = body
        self.comments = comments
        self.links = links
        self.composer = composer
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.lenientString(.id)
        title = c.lenientString(.title)
        kind = SourceSectionKind(word: c.lenientString(.kind))
        body = c.optionalString(.body)
        comments = try c.list(SourceComment.self, .comments)
        links = try c.list(SourceLink.self, .links)
        composer = try c.decodeIfPresent(SourceAction.self, forKey: .composer)
    }
}

/// The plugin's `container` response: one container's detail.
public struct ContainerDetail: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let title: String
    public let externalURL: String?
    public let facts: [SourceFact]
    public let sections: [SourceSection]
    public let menu: [SourceAction]
    /// Drawn in the PR section of every task under this container.
    public let taskNote: String?

    enum CodingKeys: String, CodingKey {
        case id, title
        case externalURL = "external_url"
        case facts, sections, menu
        case taskNote = "task_note"
    }

    public init(
        id: String, title: String, externalURL: String? = nil, facts: [SourceFact] = [],
        sections: [SourceSection] = [], menu: [SourceAction] = [], taskNote: String? = nil
    ) {
        self.id = id
        self.title = title
        self.externalURL = externalURL
        self.facts = facts
        self.sections = sections
        self.menu = menu
        self.taskNote = taskNote
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.lenientString(.id)
        title = c.lenientString(.title)
        externalURL = c.optionalString(.externalURL)
        facts = try c.list(SourceFact.self, .facts)
        sections = try c.list(SourceSection.self, .sections)
        menu = try c.list(SourceAction.self, .menu)
        taskNote = c.optionalString(.taskNote)
    }
}

/// `nat container-show --json`: the plugin's container detail as-is, and
/// the plan's tasks under it, each in `info`'s slice shape.
public struct ContainerShow: Codable, Equatable, Sendable {
    public let container: ContainerDetail
    public let tasks: [Slice]

    enum CodingKeys: String, CodingKey { case container, tasks }

    public init(container: ContainerDetail, tasks: [Slice] = []) {
        self.container = container
        self.tasks = tasks
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        container = try c.decode(ContainerDetail.self, forKey: .container)
        tasks = try c.list(Slice.self, .tasks)
    }
}

/// A plugin's `describe` response, as `source-list` carries it.
public struct SourceDescribe: Codable, Equatable, Sendable {
    public let `protocol`: Int
    public let name: String
    public let title: String
    public let tag: String
    public let iconSymbol: String
    public let iconSVG: String?
    public let containerNoun: String
    public let taskNoun: String
    public let menu: [SourceAction]

    enum CodingKeys: String, CodingKey {
        case `protocol`, name, title, tag
        case iconSymbol = "icon_symbol"
        case iconSVG = "icon_svg"
        case containerNoun = "container_noun"
        case taskNoun = "task_noun"
        case menu
    }

    public init(
        protocol: Int = 1, name: String, title: String, tag: String, iconSymbol: String,
        iconSVG: String? = nil, containerNoun: String, taskNoun: String, menu: [SourceAction] = []
    ) {
        self.protocol = `protocol`
        self.name = name
        self.title = title
        self.tag = tag
        self.iconSymbol = iconSymbol
        self.iconSVG = iconSVG
        self.containerNoun = containerNoun
        self.taskNoun = taskNoun
        self.menu = menu
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        `protocol` = ((try? c.decodeIfPresent(Int.self, forKey: .protocol)) ?? nil) ?? 0
        name = c.lenientString(.name)
        title = c.lenientString(.title)
        tag = c.lenientString(.tag)
        iconSymbol = c.lenientString(.iconSymbol)
        iconSVG = c.optionalString(.iconSVG)
        containerNoun = c.lenientString(.containerNoun)
        taskNoun = c.lenientString(.taskNoun)
        menu = try c.list(SourceAction.self, .menu)
    }
}

/// One row of `nat source-list --json`: a discovered plugin, `describe`d —
/// or, where it would not describe, its `error`.
public struct SourcePlugin: Codable, Equatable, Sendable, Identifiable {
    public let name: String
    public let path: String
    public let describe: SourceDescribe?
    public let error: String?

    public var id: String { name }

    /// The executable a plugin of this name is discovered as.
    public var executableName: String { "nat-source-\(name)" }

    /// What a list of plugins calls this one: its own title once described,
    /// else the name it was discovered by.
    public var displayTitle: String {
        guard let title = describe?.title, !title.isEmpty else { return name }
        return title
    }

    /// The SF Symbol to draw it with: its own once described, else a
    /// generic plugin glyph.
    public var iconSymbol: String {
        guard let symbol = describe?.iconSymbol, !symbol.isEmpty else { return "puzzlepiece.extension" }
        return symbol
    }

    enum CodingKeys: String, CodingKey { case name, path, describe, error }

    public init(name: String, path: String, describe: SourceDescribe? = nil, error: String? = nil) {
        self.name = name
        self.path = path
        self.describe = describe
        self.error = error
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = c.lenientString(.name)
        path = c.lenientString(.path)
        describe = try c.decodeIfPresent(SourceDescribe.self, forKey: .describe)
        error = c.optionalString(.error)
    }
}

/// `slice-show --json`'s `container`, for a task in a source project. On a
/// failed plugin read only `id` and the cached `title` are filled.
public struct SliceContainer: Codable, Equatable, Sendable {
    public let id: String
    public let title: String
    public let externalURL: String?
    public let taskNote: String?
    public let facts: [SourceFact]

    enum CodingKeys: String, CodingKey {
        case id, title
        case externalURL = "external_url"
        case taskNote = "task_note"
        case facts
    }

    public init(id: String, title: String, externalURL: String? = nil, taskNote: String? = nil, facts: [SourceFact] = []) {
        self.id = id
        self.title = title
        self.externalURL = externalURL
        self.taskNote = taskNote
        self.facts = facts
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = c.lenientString(.id)
        title = c.lenientString(.title)
        externalURL = c.optionalString(.externalURL)
        taskNote = c.optionalString(.taskNote)
        facts = try c.list(SourceFact.self, .facts)
    }
}

/// `nat source-action --json`: what the plugin said, if anything.
public struct SourceActionResult: Codable, Equatable, Sendable {
    public let message: String?

    enum CodingKeys: String, CodingKey { case message }

    public init(message: String? = nil) {
        self.message = message
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        message = c.optionalString(.message)
    }
}
