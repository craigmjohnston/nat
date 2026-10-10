import Foundation

/// What the navigator's titlebar names the selection by: the sidebar's
/// Active row for the same thing — the project's tag, the state dot, the
/// title. A source container has no state of its own: it is named by its
/// source's icon in the dot's place (`icon`); a workshop by its wand
/// (`symbol`), taking the state's ink. A source project takes no tag: its
/// tasks and containers are named by their card's badge (`cardBadge`),
/// led by the source's icon (`cardIcon`), where the card has one.
public struct TitlebarIdentity: Equatable, Sendable {
    public let tag: String
    public let state: SliceDisplayState
    public let live: Bool
    public let title: String
    /// Drawn in place of the state dot — a source container's.
    public let icon: SourceIcon?
    /// An SF Symbol drawn in place of the state dot, in the state's ink — a
    /// workshop's wand (`SidebarActiveRow.symbol`).
    public let symbol: String?
    /// A source task's or container's card's badge, drawn in the project
    /// badge's place.
    public let cardBadge: SourceBadge?
    /// The source's icon, leading `cardBadge` — or alone, where the card has
    /// none; nil outside a source project.
    public let cardIcon: SourceIcon?
    /// The scratch project's work: named by Scratch's mark — its icon and
    /// the word Scratch — in the project badge's place.
    public let isScratch: Bool

    public init(
        tag: String, state: SliceDisplayState, live: Bool, title: String, icon: SourceIcon? = nil,
        symbol: String? = nil, cardBadge: SourceBadge? = nil, cardIcon: SourceIcon? = nil, isScratch: Bool = false
    ) {
        self.tag = tag
        self.state = state
        self.live = live
        self.title = title
        self.icon = icon
        self.symbol = symbol
        self.cardBadge = cardBadge
        self.cardIcon = cardIcon
        self.isScratch = isScratch
    }

    /// A source container's identity: its card's badge, if it has one, its
    /// source's icon in the dot's place, its title.
    public static func container(title: String, icon: SourceIcon, badge: SourceBadge? = nil) -> TitlebarIdentity {
        TitlebarIdentity(tag: "", state: .todo, live: false, title: title, icon: icon, cardBadge: badge, cardIcon: icon)
    }

    /// The identity as the titlebar's breadcrumb draws it, as its last crumb:
    /// without the project's tag (or a source's card mark) where a crumb
    /// before it already names the project, whole otherwise.
    public func lastCrumb(afterProjectCrumb: Bool) -> TitlebarIdentity {
        guard afterProjectCrumb else { return self }
        return TitlebarIdentity(tag: "", state: state, live: live, title: title, icon: icon, symbol: symbol)
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
    /// A source project's container, named by its source.
    case container(id: String, title: String, icon: SourceIcon)
}

/// The titlebar's identity for `selection` in `projectID`. Where the
/// selection has an Active row, tag, state and liveness are read off that row,
/// so the two can never disagree. Otherwise the tag is the project's own from
/// `tags` and nothing is live: a slice keeps the state it was given, a
/// workshop nothing runs for is todo, and a session out of Active has ended.
/// A container is never in Active: it is its card's badge, its source's icon
/// and its title. In a source project (`plan` carrying `source`) there is
/// no tag: a task is named by its card's badge instead, read off `plan`.
/// In the scratch project (`scratchProjectID`) it is named by Scratch's mark.
public func titlebarIdentity(
    for selection: TitlebarSelection, projectID: String, active: [SidebarActiveRow], tags: [String: String],
    plan: ProjectInfo? = nil, scratchProjectID: String? = nil
) -> TitlebarIdentity {
    let kind: SidebarActiveKind
    let targetID: String
    let title: String
    let fallback: SliceDisplayState
    var symbol: String?
    switch selection {
    case let .slice(id, name, state):
        (kind, targetID, title, fallback) = (.slice, id, name, state)
    case .workshop:
        (kind, targetID, title, fallback) = (.workshop, projectID, workshopRowTitle, .todo)
        symbol = workshopSymbol
    case let .session(id, sessionTitle):
        (kind, targetID, title, fallback) = (.session, id, sessionTitle, .done)
    case let .container(id, containerTitle, icon):
        return .container(title: containerTitle, icon: icon, badge: plan?.source?.badge(ofContainer: id))
    }
    if let row = active.first(where: { $0.kind == kind && $0.targetID == targetID && $0.projectID == projectID }) {
        return TitlebarIdentity(
            tag: row.projectTag, state: row.state, live: row.live, title: title, symbol: symbol,
            cardBadge: row.card?.badge, cardIcon: row.card?.icon ?? plan?.source?.icon, isScratch: row.isScratch)
    }
    guard let plan, let info = plan.source else {
        return TitlebarIdentity(
            tag: tags[projectID] ?? "", state: fallback, live: false, title: title, symbol: symbol,
            isScratch: projectID == scratchProjectID)
    }
    let card = kind == .slice
        ? plan.slices.first { $0.id == targetID }.flatMap { activeCard($0.milestoneID, projectID: projectID, plan: plan) }
        : nil
    return TitlebarIdentity(
        tag: "", state: fallback, live: false, title: title, symbol: symbol, cardBadge: card?.badge,
        cardIcon: info.icon)
}
