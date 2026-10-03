import Foundation

/// What the main pane shows for a selected source container: its story (the
/// prose and the comments under it), or one section's own view — a links
/// section's list.
public enum ContainerPaneMode: Equatable, Sendable {
    case story
    case section(String)
}

/// The container navigator's reading of one `container-show`: the first
/// section (the story's facts and its tasks), then one section per remaining
/// section the plugin declared, of a kind this build draws — an unknown kind
/// is skipped, never drawn as something it is not.
public struct ContainerNavigatorModel: Equatable, Sendable {
    public let detail: ContainerDetail
    public let tasks: [Slice]

    /// The first section's id when nothing else names it.
    public static let storyID = "story"

    public init(show: ContainerShow) {
        detail = show.container
        tasks = show.tasks
    }

    /// The first `prose` section: what the first navigator section is titled
    /// by, and what the main pane's story draws.
    public var prose: SourceSection? {
        detail.sections.first { $0.kind == .prose }
    }

    /// The first navigator section's key and title: the prose section's, else
    /// "Story".
    public var storyID: String { prose?.id ?? Self.storyID }
    public var storyTitle: String {
        guard let title = prose?.title, !title.isEmpty else { return "Story" }
        return title
    }

    /// The first `comments` section: the thread the main pane draws under the
    /// story.
    public var comments: SourceSection? {
        detail.sections.first { $0.kind == .comments }
    }

    /// The sections after the first, in the plugin's order: every comments
    /// and links section, and any prose section but the one the first
    /// section stands for. Unknown kinds are skipped.
    public var sections: [SourceSection] {
        detail.sections.filter { section in
            switch section.kind {
            case .prose: return section.id != prose?.id
            case .comments, .links: return true
            case .unknown: return false
            }
        }
    }

    public var tasksDone: Int { tasks.filter { $0.status == "Done" }.count }

    /// The facts grid's last line: "n/m done", or "none yet".
    public var tasksFact: String {
        tasks.isEmpty ? "none yet" : "\(tasksDone)/\(tasks.count) done"
    }

    /// What a section's header says beside its label: a comments section's
    /// count, a links section's count or "none".
    public func meta(for section: SourceSection) -> String? {
        switch section.kind {
        case .comments: return section.comments.isEmpty ? nil : "\(section.comments.count)"
        case .links: return section.links.isEmpty ? "none" : "\(section.links.count)"
        case .prose, .unknown: return nil
        }
    }

    /// The main-pane view a section's header puts up: a comments or prose
    /// section is shown in the story, a links section by its own list.
    public func paneMode(for sectionID: String) -> ContainerPaneMode {
        guard let section = sections.first(where: { $0.id == sectionID }), section.kind == .links else {
            return .story
        }
        return .section(section.id)
    }

    /// The defaults a container opens on: the first section open, the story
    /// up.
    public var defaultFocus: ContainerFocus {
        ContainerFocus(open: [storyID], main: .story)
    }
}

/// What the container navigator has open and what the main pane shows,
/// together — `NavigatorFocus`'s rules over section ids.
public struct ContainerFocus: Equatable, Sendable {
    public var open: Set<String>
    public var main: ContainerPaneMode

    public init(open: Set<String>, main: ContainerPaneMode) {
        self.open = open
        self.main = main
    }

    /// The chevron: fold or unfold, nothing else.
    public func togglingFold(_ section: String) -> ContainerFocus {
        var next = self
        if open.contains(section) { next.open.remove(section) } else { next.open.insert(section) }
        return next
    }

    /// The rest of the header: open the section and put its view up — or,
    /// already open with its view up, fold it.
    public func clickingHead(_ section: String, shows: ContainerPaneMode) -> ContainerFocus {
        guard !(open.contains(section) && main == shows) else { return togglingFold(section) }
        var next = self
        next.open.insert(section)
        next.main = shows
        return next
    }
}
