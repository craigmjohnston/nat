import Foundation

/// What the navigator's titlebar names the selection by: the sidebar's
/// Active row for the same thing — the project's tag, the state dot, the
/// title.
public struct TitlebarIdentity: Equatable, Sendable {
    public let tag: String
    public let state: SliceDisplayState
    public let live: Bool
    public let title: String

    public init(tag: String, state: SliceDisplayState, live: Bool, title: String) {
        self.tag = tag
        self.state = state
        self.live = live
        self.title = title
    }
}

/// The selection the navigator's titlebar names, with what it knows of it
/// beyond the sidebar.
public enum TitlebarSelection: Equatable, Sendable {
    /// A slice, with the state the navigator reads for it.
    case slice(id: String, name: String, state: SliceDisplayState)
    /// The project's workshop.
    case workshop
    /// An ad hoc session, with the title the titlebar gives it.
    case session(id: String, title: String)
}

/// The titlebar's identity for `selection` in `projectID`. Where the
/// selection has an Active row, tag, state and liveness are read off that row,
/// so the two can never disagree. Otherwise the tag is the project's own from
/// `tags` and nothing is live: a slice keeps the state it was given, a
/// workshop nothing runs for is todo, and a session out of Active has ended.
public func titlebarIdentity(
    for selection: TitlebarSelection, projectID: String, active: [SidebarActiveRow], tags: [String: String]
) -> TitlebarIdentity {
    let kind: SidebarActiveKind
    let targetID: String
    let title: String
    let fallback: SliceDisplayState
    switch selection {
    case let .slice(id, name, state):
        (kind, targetID, title, fallback) = (.slice, id, name, state)
    case .workshop:
        (kind, targetID, title, fallback) = (.workshop, projectID, workshopRowTitle, .todo)
    case let .session(id, sessionTitle):
        (kind, targetID, title, fallback) = (.session, id, sessionTitle, .done)
    }
    if let row = active.first(where: { $0.kind == kind && $0.targetID == targetID && $0.projectID == projectID }) {
        return TitlebarIdentity(tag: row.projectTag, state: row.state, live: row.live, title: title)
    }
    return TitlebarIdentity(tag: tags[projectID] ?? "", state: fallback, live: false, title: title)
}
