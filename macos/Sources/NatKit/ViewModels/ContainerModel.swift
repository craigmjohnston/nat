import Foundation

/// What the main pane shows for a selected source container: its story (the
/// prose and the comments under it), or one section's own view — a links
/// section's list.
public enum ContainerPaneMode: Equatable, Sendable {
    case story
    case section(String)
}

/// The container navigator's reading of one `container show`: the first
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

    /// The navigator's sections after the first, in the plugin's order:
    /// every links section, and any prose section but the one the first
    /// section stands for. Comments are the main pane's alone, under the
    /// description; unknown kinds are skipped.
    public var sections: [SourceSection] {
        detail.sections.filter { section in
            switch section.kind {
            case .prose: return section.id != prose?.id
            case .links: return true
            case .comments, .unknown: return false
            }
        }
    }

    /// The comments as the main pane threads them: each comment that answers
    /// none, oldest first, with its replies under it in the order given.
    /// Threads are one level deep — nat refuses a reply to a reply — and a
    /// reply whose parent is not in the section stands as a comment of its
    /// own rather than being lost.
    public var threads: [CommentThread] {
        guard let comments = comments?.comments else { return [] }
        let ids = Set(comments.map(\.id))
        func answers(_ comment: SourceComment) -> String? {
            guard let parent = comment.parent, !parent.isEmpty, parent != comment.id, ids.contains(parent) else {
                return nil
            }
            return parent
        }
        var threads: [CommentThread] = []
        var index: [String: Int] = [:]
        for comment in comments where answers(comment) == nil {
            index[comment.id] = threads.count
            threads.append(CommentThread(comment: comment))
        }
        for comment in comments {
            guard let parent = answers(comment) else { continue }
            if let at = index[parent] {
                threads[at].replies.append(comment)
            } else {
                // Its parent is itself a reply: nat refuses that, but a reading
                // that slipped past still shows the comment.
                threads.append(CommentThread(comment: comment))
            }
        }
        return threads
    }

    /// The card's own actions — Launch agent, Workshop, New task — as the
    /// navigator's action row and the sidebar row's menu offer them.
    public var actions: CardActions {
        CardActions(title: detail.title, story: prose?.body ?? "", taskCount: tasks.count)
    }

    public var tasksDone: Int { tasks.filter { $0.status == "Done" }.count }

    /// The facts grid's last line: "n/m done", or "none yet".
    public var tasksFact: String {
        tasks.isEmpty ? "none yet" : "\(tasksDone)/\(tasks.count) done"
    }

    /// What a section's header says beside its label: a links section's
    /// count or "none" (a comments section's count, should one be asked).
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

/// One comment of a container's thread with the replies under it.
public struct CommentThread: Equatable, Sendable, Identifiable {
    public let comment: SourceComment
    public var replies: [SourceComment]

    public init(comment: SourceComment, replies: [SourceComment] = []) {
        self.comment = comment
        self.replies = replies
    }

    public var id: String { comment.id }

    /// The comment a reply written under this thread answers: its first
    /// comment, whichever of its comments the Reply was under — a thread is
    /// one level deep.
    public var replyTarget: String { comment.id }
}

/// What a source card's own actions do. **Launch agent** files one task under
/// the card — titled after it, its brief the card's story — and launches it,
/// offered only while the card has no task at all (a card with tasks is
/// worked through them). **Workshop** opens the project's workshop with the
/// card as its request and starts it. **New task** opens the New task sheet
/// with the card preset.
public struct CardActions: Equatable, Sendable {
    public let title: String
    public let story: String
    public let taskCount: Int

    public init(title: String, story: String, taskCount: Int) {
        self.title = title
        self.story = story
        self.taskCount = taskCount
    }

    /// Whether Launch agent is offered: only on a card with no task yet.
    public var canLaunchAgent: Bool { taskCount == 0 }

    /// Why Launch agent is greyed, where it is.
    public var launchRefusal: String? {
        canLaunchAgent ? nil : "This card already has tasks — launch one of them instead"
    }

    /// The new task's title: the card's, cut at a word to nat's title limit
    /// where it is longer, with an ellipsis. nat counts runes, so this counts
    /// Unicode scalars, never characters.
    public var taskTitle: String {
        let trimmed = title.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.unicodeScalars.count > maxTaskTitle else { return trimmed }
        var room = String.UnicodeScalarView(trimmed.unicodeScalars.prefix(maxTaskTitle - 1))
        if let space = room.lastIndex(of: " ") { room = String.UnicodeScalarView(room[..<space]) }
        return String(room).trimmingCharacters(in: .whitespaces) + "\u{2026}"
    }

    /// The new task's brief: the card's story, trimmed — nil where it has none.
    public var taskBrief: String? {
        let brief = story.trimmingCharacters(in: .whitespacesAndNewlines)
        return brief.isEmpty ? nil : brief
    }

    /// What Workshop starts the planning agent on: the card's title, then its
    /// story.
    public var workshopRequest: String {
        [title.trimmingCharacters(in: .whitespacesAndNewlines), taskBrief]
            .compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: "\n\n")
    }

    /// nat's `domain.MaxSliceTitleLen`: the longest title `slice add` takes.
    public static let maxTaskTitle = 64
    private var maxTaskTitle: Int { Self.maxTaskTitle }
}

/// Why a card's detail could not be read for one of its actions.
public struct CardReadError: Error, Equatable, Sendable {
    public let message: String

    public init(message: String) {
        self.message = message
    }
}
