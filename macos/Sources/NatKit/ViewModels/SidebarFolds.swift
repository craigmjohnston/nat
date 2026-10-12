import Foundation
import Observation

/// What the sidebar has selected, as far as its folds care: the active
/// project and, in it, the selected slice or ad hoc session.
public struct SidebarSelection: Equatable, Sendable {
    public let projectID: String
    public let sliceID: String?
    public let sessionID: String?

    public init(projectID: String, sliceID: String? = nil, sessionID: String? = nil) {
        self.projectID = projectID
        self.sliceID = sliceID
        self.sessionID = sessionID
    }
}

/// The sidebar's folds, by key: `active`, `work`, `scratch`, `s:<project>`
/// (a source fold), `p:<project>`, `m:<project>/<milestone>`, `d:<project>`
/// (the Done folder), `m:<project>/~sessions` and `g:<project>/<group>`.
/// Where the user has neither folded nor opened one, and nothing has
/// revealed it, it takes its static default (`isOpen(_:byDefault:)`) — never
/// one read off the selection, so selecting elsewhere moves nothing.
///
/// The selection is *revealed* instead, as VS Code's explorer and Zed's
/// project panel reveal the active file: each time it changes, every fold
/// over its row is opened (`follow`), and those opens are the user's as much
/// as a click's — kept after the selection moves on. Reveal never folds
/// anything, and runs once per selection: a fold the user then makes over
/// it stands until the selection changes again. Every fold is kept in
/// `FoldMemory`, so the tree outlasts a relaunch.
@MainActor
@Observable
public final class SidebarFolds {
    /// Fold key → folded.
    public private(set) var folded: [String: Bool]

    @ObservationIgnored private let memory: FoldMemory
    /// A story's own folds (`init(memory:seed:)`), laid back over the first
    /// reveal — what the user folded after the starting selection was shown.
    @ObservationIgnored private var seed: [String: Bool]
    /// The selection `follow` was last given, and whether it has been
    /// revealed yet.
    @ObservationIgnored private var following: SidebarSelection?
    @ObservationIgnored private var revealed = false

    /// - Parameter seed: folds to start from over what `memory` holds — a
    ///   gallery story's — recorded only once something else changes.
    public init(memory: FoldMemory = .inMemory(), seed: [String: Bool] = [:]) {
        self.memory = memory
        self.seed = seed
        folded = memory.folds.merging(seed) { _, seeded in seeded }
    }

    /// Whether `key` is open: as last folded, opened or revealed, else
    /// `byDefault`.
    public func isOpen(_ key: String, byDefault: Bool = true) -> Bool {
        folded[key].map { !$0 } ?? byDefault
    }

    /// Fold or open `key`, as the user's click does.
    public func set(_ key: String, folded isFolded: Bool) {
        folded[key] = isFolded
        memory.save(folded)
    }

    /// Carry `from`'s fold over to `to` — a milestone renamed keeps its fold.
    public func rename(_ from: String, to: String) {
        guard let isFolded = folded.removeValue(forKey: from) else { return }
        folded[to] = isFolded
        memory.save(folded)
    }

    /// Open every one of `keys`, recording each as opened, so it stays open
    /// after the selection moves on. Folds nothing.
    public func reveal(_ keys: [String]) {
        for key in keys { folded[key] = false }
        memory.save(folded)
    }

    /// Reveal `selection`, once: `keys` are the folds over its row
    /// (`sidebarRevealKeys`), nil while the sidebar does not draw it yet — a
    /// plan still to load — in which case the next call for the same
    /// selection tries again. Once revealed, the same selection reveals
    /// nothing more, however often it is followed; a different one starts
    /// afresh.
    public func follow(_ selection: SidebarSelection?, keys: [String]?) {
        if selection != following {
            following = selection
            revealed = false
        }
        guard !revealed, let keys else { return }
        revealed = true
        reveal(keys)
        guard !seed.isEmpty else { return }
        folded.merge(seed) { _, seeded in seeded }
        seed = [:]
    }
}

/// The folds over `selection`'s row in the sidebar, outermost first — what
/// opening shows it: its section (`work`, `scratch` or its source fold), its
/// project row, the Done folder where its milestone is finished, its
/// milestone or source group, or the Ad hoc sessions folder for an ended
/// session (one of `endedSessions`); for a project with nothing in its tree
/// selected — nothing at all, or a live session — its section and project
/// row. Nil where `model` does not draw the row — a plan not loaded yet, a
/// done slice hidden.
public func sidebarRevealKeys(
    for selection: SidebarSelection, in model: SidebarModel, endedSessions: Set<String> = []
) -> [String]? {
    let id = selection.projectID
    let project = model.projects.first { $0.id == id }
    let scratch = model.scratch?.id == id ? model.scratch : nil
    if let sliceID = selection.sliceID {
        if let project { return project.revealKeys(of: sliceID).map { ["work", "p:\(id)"] + $0 } }
        if let scratch { return scratch.revealKeys(of: sliceID).map { ["scratch"] + $0 } }
        return model.source(projectID: id)?.source?.revealKeys(of: sliceID, projectID: id).map { ["s:\(id)"] + $0 }
    }
    if let sessionID = selection.sessionID, endedSessions.contains(sessionID) {
        if project != nil { return ["work", "p:\(id)", "m:\(id)/~sessions"] }
        if scratch != nil { return ["scratch", "m:\(id)/~sessions"] }
    }
    // The project itself selected — a tab switched to, nothing in it picked
    // yet, or a live session, drawn in Active alone: its tree is what is
    // shown, as its plan's note or failure is.
    if project != nil { return ["work", "p:\(id)"] }
    if scratch != nil { return ["scratch"] }
    return model.source(projectID: id) != nil ? ["s:\(id)"] : nil
}

extension SidebarProject {
    /// The folds inside the project over a slice's row — nil where it files
    /// none by that ID.
    func revealKeys(of sliceID: String) -> [String]? {
        func holds(_ milestone: SidebarMilestone) -> Bool { milestone.slices.contains { $0.sliceID == sliceID } }
        if loose.contains(where: { $0.sliceID == sliceID }) { return [] }
        if let milestone = milestones.first(where: holds) { return ["m:\(id)/\(milestone.name)"] }
        if let milestone = doneMilestones.first(where: holds) { return ["d:\(id)", "m:\(id)/\(milestone.name)"] }
        return nil
    }
}

extension SidebarSource {
    /// The group folds over a task's row, through the first group (and child
    /// group) whose containers hold it. A lazy group has no fold of ours to
    /// open — it lists containers only once expanded — so it adds no key.
    func revealKeys(of sliceID: String, projectID: String) -> [String]? {
        func holds(_ group: SidebarSourceGroup) -> Bool {
            group.containers.contains { $0.tasks.contains { $0.sliceID == sliceID } }
        }
        func key(_ group: SidebarSourceGroup) -> [String] { group.lazy ? [] : ["g:\(projectID)/\(group.id)"] }
        for group in groups {
            if holds(group) { return key(group) }
            if let child = group.children.first(where: holds) { return key(group) + key(child) }
        }
        return nil
    }
}
