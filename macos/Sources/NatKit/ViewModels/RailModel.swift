import Foundation

/// The activity state of an agent working on a slice, as read fresh off the
/// live tmux map. It refines an ACTIVE row's label only — never whether the
/// row is drawn at all, which `buildRailModel` decides from the slice's own
/// page.
public enum AgentActivity {
    case working
    case waiting

    /// The live map's own word for it. An activity nobody could classify
    /// reads as working, which is the TUI's own convention: a session that
    /// is there and has not visibly stopped is one to leave alone.
    public init(_ state: AgentActivityState) {
        self = state == .waiting ? .waiting : .working
    }
}

/// The semantic tint an ACTIVE row's dot and status text take. Named for
/// meaning rather than a color, so this view model stays free of picking a
/// palette — `RailView` is what maps each case onto a `DesignTokens` color.
///
/// One enum for every kind of entry now that the rail has one flight
/// section: a workshop row is launching or being composed where a slice's
/// is blocked or ready to push, and both are drawn by the same row builder.
public enum ActiveTintRole: Equatable {
    case working
    case waiting
    case blocked
    case readyToPush
    /// A branch handed back and waiting to be read — the green the review
    /// affordance already uses, the same family as the TUI's "↑ review" chip.
    case needsReview
    /// The planning agent's own two quiet states: a launch in flight, and a
    /// composer open with nothing started yet.
    case launching
    case new

    /// Whether the row's dot moves. Only work actually in progress does:
    /// movement is what says "busy, nothing needs you", so a row that has
    /// stopped for an answer sits still in its waiting yellow and is told
    /// apart from a working one by more than a colour. The project tab's dot
    /// reads by the same rule — see `ProjectAttention.pulses`.
    public var pulses: Bool {
        self == .working
    }
}

/// What an ACTIVE entry stands for. A slice entry selects its slice; the
/// workshop entry selects the planning pane, which is the only thing the
/// view has to tell them apart for.
public enum ActiveEntryKind: Equatable {
    case slice
    case workshop
}

/// What an entry's right-aligned meta is, so the view need not guess which
/// of the two it was handed: how long a live session has been running, or
/// the diff tally of a branch handed back. Never both.
public enum ActiveMetaRole: Equatable {
    case elapsed
    case stat
}

/// The id the workshop entry is keyed by in the one ACTIVE list — it has no
/// slice of its own, and there is only ever one of it.
public let workshopEntryID = "workshop"

/// An entry in the ACTIVE section — a slice with something happening on it,
/// a branch waiting to be reviewed, or the planning agent.
public struct ActiveEntry: Equatable, Identifiable {
    public let kind: ActiveEntryKind
    /// The slice the entry is about, and "" for the workshop entry, which is
    /// about no slice at all.
    public let sliceID: String
    public let name: String
    /// The row's own status word — "Working", "Waiting for input",
    /// "Blocked", "Ready to push", "Needs review", "Launching…", "New
    /// session" — already resolved here, since the rule it comes from is the
    /// view model's and not the view's to know.
    public let displayState: String
    public let tintRole: ActiveTintRole
    /// The rest of the second line after the status word, each piece drawn
    /// muted and separated by a dot: a slice's milestone, a review's file
    /// count, the workshop's "Planning agent".
    public let detail: [String]
    /// The right-aligned meta, nil for a row with none — an entry with no
    /// live session has no elapsed time, and a branch whose stats have not
    /// been fetched draws no tally rather than a placeholder.
    public let meta: String?
    public let metaRole: ActiveMetaRole

    public var id: String { kind == .workshop ? workshopEntryID : sliceID }

    public init(
        kind: ActiveEntryKind = .slice,
        sliceID: String = "",
        name: String,
        displayState: String,
        tintRole: ActiveTintRole,
        detail: [String] = [],
        meta: String? = nil,
        metaRole: ActiveMetaRole = .elapsed
    ) {
        self.kind = kind
        self.sliceID = sliceID
        self.name = name
        self.displayState = displayState
        self.tintRole = tintRole
        self.detail = detail
        self.meta = meta
        self.metaRole = metaRole
    }
}

/// Builds the workshop's ACTIVE entry, or nil when there is no planning
/// agent to draw: none live, no launch in flight, and the entry not selected
/// — a selected entry with nothing running is the composer being typed into,
/// drawn so the rail's selection stays visible while it is. A live agent
/// wins over both flags — it is the only reading taken fresh.
public func buildWorkshopEntry(
    activity: AgentActivity?,
    isLaunching: Bool,
    isSelected: Bool = false,
    firstSeen: Date? = nil,
    now: Date = Date()
) -> ActiveEntry? {
    let state: (String, ActiveTintRole)
    var elapsed: String?
    switch activity {
    case .working:
        state = ("Working", .working)
        elapsed = firstSeen.map { elapsedLabel(from: $0, to: now) }
    case .waiting:
        state = ("Waiting for input", .waiting)
        elapsed = firstSeen.map { elapsedLabel(from: $0, to: now) }
    case nil:
        if isLaunching {
            state = ("Launching…", .launching)
        } else if isSelected {
            state = ("New session", .new)
        } else {
            return nil
        }
    }
    return ActiveEntry(
        kind: .workshop,
        name: "Workshop the plan",
        displayState: state.0,
        tintRole: state.1,
        detail: ["Planning agent"],
        meta: elapsed,
        metaRole: .elapsed
    )
}

/// A slice glyph type.
public enum SliceGlyph: String {
    case todo = "circle"
    case inProgress = "circle.lefthalf.filled"
    case done = "checkmark.circle"
    case blocked = "nosign"
}

/// A slice row within a milestone folder.
public struct MilestoneSliceRow: Equatable {
    public let sliceID: String
    public let name: String
    public let glyph: SliceGlyph
    public let isBlocked: Bool

    public init(sliceID: String, name: String, glyph: SliceGlyph, isBlocked: Bool) {
        self.sliceID = sliceID
        self.name = name
        self.glyph = glyph
        self.isBlocked = isBlocked
    }
}

/// A milestone as a folder in the rail's file tree. The same shape serves
/// both sections: a TODO folder holds the milestone's remaining slices — not
/// Done and not drawn in a session section — and a DONE folder holds its
/// finished ones, so a milestone part-way through appears in both, each side
/// listing only its own.
public struct MilestoneFolder: Equatable {
    public let milestoneID: String
    public let title: String
    /// Done slices out of the milestone's total — the folder's right-aligned
    /// count, whichever section it is drawn in.
    public let done: Int
    public let total: Int
    public let isCurrent: Bool
    /// The slices this folder lists when expanded — see the type comment for
    /// which slices those are per section.
    public let slices: [MilestoneSliceRow]

    /// Whether every slice of the milestone is Done — what earns a DONE
    /// folder its green checkmark.
    public var isComplete: Bool {
        total > 0 && done == total
    }

    /// The milestone's slices drawn in the ACTIVE section instead of
    /// under this folder — never listed here, but what seeds a folder open,
    /// since work in flight is a milestone moving.
    public var inFlightCount: Int {
        max(0, total - done - slices.count)
    }

    public init(
        milestoneID: String,
        title: String,
        done: Int,
        total: Int,
        isCurrent: Bool,
        slices: [MilestoneSliceRow]
    ) {
        self.milestoneID = milestoneID
        self.title = title
        self.done = done
        self.total = total
        self.isCurrent = isCurrent
        self.slices = slices
    }
}

/// The DONE heading's own count: finished slices out of the whole plan.
public struct DoneSummary: Equatable {
    public let doneCount: Int
    public let totalCount: Int

    public init(doneCount: Int, totalCount: Int) {
        self.doneCount = doneCount
        self.totalCount = totalCount
    }
}

/// The data model for the left rail.
public struct RailModel: Equatable {
    /// The one flight section: the workshop entry first, then the branches
    /// waiting to be reviewed, then the slices something is happening on.
    /// One list rather than three sections, with the prominence the separate
    /// headings gave the first two kept as their place in the order.
    public let active: [ActiveEntry]

    /// Folders for non-done milestones, in plan order, each listing its
    /// remaining slices.
    public let todoFolders: [MilestoneFolder]

    /// Folders under the DONE heading, each listing a milestone's finished
    /// slices: the milestones still open in TODO first, in plan order, then
    /// the fully finished ones newest-first (reverse plan order), so what was
    /// finished most recently reads at the top.
    public let doneFolders: [MilestoneFolder]

    /// The DONE heading's count — nil when nothing is done, which is when
    /// the section is not drawn at all.
    public let doneSummary: DoneSummary?

    public init(
        active: [ActiveEntry],
        todoFolders: [MilestoneFolder],
        doneFolders: [MilestoneFolder] = [],
        doneSummary: DoneSummary? = nil
    ) {
        self.active = active
        self.todoFolders = todoFolders
        self.doneFolders = doneFolders
        self.doneSummary = doneSummary
    }
}

// MARK: - ACTIVE Membership

/// Whether the ACTIVE section's review half would hold this slice: a branch
/// handed back and not yet approved, or — `openPRSliceIDs` being the slices
/// whose pull request is positively read as open — one approved and waiting
/// on the merge.
public func isReviewSlice(_ slice: Slice, openPRSliceIDs: Set<String>) -> Bool {
    slice.handedBack || openPRSliceIDs.contains(slice.id)
}

/// Whether the ACTIVE section's working half would hold this slice. Mirrors
/// the gate `domain.StateOf` applies before a live agent ever enters into it:
/// In progress, not handed back, and no pull request recorded. It is never
/// "has a live tmux session" — a session can outlive the slice it was
/// launched on.
public func isActiveSlice(_ slice: Slice) -> Bool {
    slice.status == "In progress" && !slice.handedBack && slice.pr.isEmpty
}

/// The slices the ACTIVE section would draw: the union of the two halves
/// above. One rule, shared by the rail that draws the section and by
/// `projectAttention`, which may only read a live agent whose slice is in it
/// — so the tab's dot and the rail can never disagree about what is in
/// flight. It takes the open-pull-request slice IDs rather than a readiness
/// map, since the two callers hold that reading in different shapes and only
/// its key set is the membership question.
public func inFlightSliceIDs(slices: [Slice], openPRSliceIDs: Set<String>) -> Set<String> {
    var ids = Set<String>()
    for slice in slices where isReviewSlice(slice, openPRSliceIDs: openPRSliceIDs)
        || isActiveSlice(slice) {
        ids.insert(slice.id)
    }
    return ids
}

// MARK: - Rail Model Builder

/// Builds a rail model from project info and live agents.
///
/// `reviewStats` and `reviewFileCounts` are `ReviewStatsStore`'s two maps and
/// `agentStarts` is `ActivityStore.firstSeen`, all keyed by slice id — passed
/// in rather than read here, since a view model has no business reaching for
/// a store of its own (mirrors how `liveAgents` is already handed in rather
/// than read off `ActivityStore` directly). `now` is only consulted to
/// format elapsed times, and is a parameter so a test can pin it.
///
/// `workshop` is `buildWorkshopEntry`'s answer, handed in for the same
/// reason: the planning agent is read off live app state rather than off the
/// plan, and this is where the one ACTIVE list is assembled so the view has
/// nothing left to merge.
public func buildRailModel(
    from projectInfo: ProjectInfo,
    liveAgents: [String: AgentActivity],
    reviewStats: [String: String] = [:],
    reviewFileCounts: [String: Int] = [:],
    prReadiness: [String: String] = [:],
    agentStarts: [String: Date] = [:],
    workshop: ActiveEntry? = nil,
    now: Date = Date()
) -> RailModel {
    let slices = projectInfo.slices
    let milestones = projectInfo.milestones

    // The slices whose pull request is positively read as open — what
    // `sliceWorkDone` gates a Done status behind, so the DONE folders, their
    // counts and the summary all read done-ness by the same rule the
    // progress bar does: merged is done.
    let openPRs = Set(prReadiness.keys)

    // The review entries hold the work a review still owes something: a branch
    // handed back and not yet approved, and — `prReadiness` being the slices
    // whose pull request is positively read as open — a slice approved and
    // waiting on the merge, since the board marks a slice Done as it opens
    // the pull request and the review is not over until that lands. With no
    // reading taken (app just opened, gh unreachable) the second kind is
    // simply absent, which is also what keeps every Done slice a project
    // ever finished from flooding the section.
    let reviewSlices = slices.filter { isReviewSlice($0, openPRSliceIDs: openPRs) }

    // A milestone's name off its ID, for the session rows' second lines.
    let milestoneNames: [String: String] = milestones.reduce(into: [:]) { $0[$1.id] = $1.name }

    // The working half of the section — `isActiveSlice`'s rule. What a live
    // agent refines is the label alone, in `activeDisplay` below.
    let activeSlices = slices.filter(isActiveSlice)

    // The review entries. A handed-back slice's meta is its diff tally; a
    // slice here for its open pull request has no branch stats to show, and
    // its meta is the reading's own words — "awaiting review", "ready to
    // merge" — which is exactly what is being waited on. Their status word
    // is the one the section they used to have their own heading for said.
    let needsReview = reviewSlices
        .sorted { $0.name < $1.name }
        .map { slice -> ActiveEntry in
            var detail: [String] = []
            if let milestone = milestoneNames[slice.milestoneID], !milestone.isEmpty {
                detail.append(milestone)
            }
            if let files = reviewFileCounts[slice.id] {
                detail.append("\(files) file\(files == 1 ? "" : "s")")
            }
            return ActiveEntry(
                sliceID: slice.id,
                name: slice.name,
                displayState: "Needs review",
                tintRole: .needsReview,
                detail: detail,
                meta: reviewStats[slice.id] ?? prReadiness[slice.id],
                metaRole: .stat
            )
        }

    // The rows for those slices.
    let working = activeSlices
        .sorted { $0.name < $1.name }
        .map { slice -> ActiveEntry in
            let liveAgent = liveAgents[slice.id]
            let (displayState, tintRole) = activeDisplay(for: slice, liveAgent: liveAgent)
            // Elapsed rides the live reading: a row with no agent on it has
            // no session to have started, whatever `agentStarts` still says.
            let elapsed = liveAgent == nil
                ? nil
                : agentStarts[slice.id].map { elapsedLabel(from: $0, to: now) }
            let milestone = milestoneNames[slice.milestoneID] ?? ""
            return ActiveEntry(
                sliceID: slice.id,
                name: slice.name,
                displayState: displayState,
                tintRole: tintRole,
                detail: milestone.isEmpty ? [] : [milestone],
                meta: elapsed,
                metaRole: .elapsed
            )
        }

    // The one section, in the order the three that came before it were read
    // in: the workshop, then what is waiting on a review, then the rest.
    let active = (workshop.map { [$0] } ?? []) + needsReview + working

    // The slices already drawn in a session section — never repeated inside
    // a TODO folder, so a slice is one row of the rail and not two.
    let inFlightIDs = inFlightSliceIDs(slices: slices, openPRSliceIDs: openPRs)

    let sortedMilestones = milestones.sorted { $0.order < $1.order }

    var todoFolders: [MilestoneFolder] = []
    var partialDoneFolders: [MilestoneFolder] = []
    var completeDoneFolders: [MilestoneFolder] = []
    var currentAssigned = false

    for milestone in sortedMilestones {
        // A milestone Notion reads as Done still holds moving work while any
        // of its slices waits on a merge — the same gate the progress bar
        // applies before folding one into its Done run.
        let milestoneSlices = slices.filter { $0.milestoneID == milestone.id }
        let milestoneDone = milestone.status == "Done"
            && milestoneSlices.allSatisfy { sliceWorkDone($0, openPRSliceIDs: openPRs) }
        let doneCount = milestoneSlices.filter { sliceWorkDone($0, openPRSliceIDs: openPRs) }.count
        let totalCount = milestoneSlices.count

        if !milestoneDone {
            // The first milestone still holding work is the current one.
            let isCurrent = !currentAssigned
                && milestoneSlices.contains { !sliceWorkDone($0, openPRSliceIDs: openPRs) }
            if isCurrent { currentAssigned = true }

            // Remaining slices: work not yet done and not drawn in a session
            // section (a Done slice awaiting its merge is the section's, not
            // this folder's).
            let remaining = milestoneSlices
                .filter { !sliceWorkDone($0, openPRSliceIDs: openPRs) && !inFlightIDs.contains($0.id) }
                .map { sliceRow(for: $0) }

            todoFolders.append(MilestoneFolder(
                milestoneID: milestone.id,
                title: milestone.name,
                done: doneCount,
                total: max(1, totalCount),
                isCurrent: isCurrent,
                slices: remaining
            ))
        }

        // Every finished slice lives under DONE, so a milestone part-way
        // through appears there too, count shown and checkmark withheld.
        if doneCount > 0 {
            let folder = MilestoneFolder(
                milestoneID: milestone.id,
                title: milestone.name,
                done: doneCount,
                total: max(1, totalCount),
                isCurrent: false,
                slices: milestoneSlices
                    .filter { sliceWorkDone($0, openPRSliceIDs: openPRs) }
                    .map { sliceRow(for: $0) }
            )
            if folder.isComplete {
                completeDoneFolders.append(folder)
            } else {
                partialDoneFolders.append(folder)
            }
        }
    }

    // Partly-done milestones first in plan order — they are the ones still
    // being fed — then the finished ones newest-first.
    let doneFolders = partialDoneFolders + completeDoneFolders.reversed()

    let doneSliceCount = slices.filter { sliceWorkDone($0, openPRSliceIDs: openPRs) }.count
    let doneSummary = doneFolders.isEmpty ? nil : DoneSummary(
        doneCount: doneSliceCount,
        totalCount: slices.count
    )

    return RailModel(
        active: active,
        todoFolders: todoFolders,
        doneFolders: doneFolders,
        doneSummary: doneSummary
    )
}

/// The ACTIVE row's elapsed label: minutes until an hour, then "Nh Mm" — the
/// coarse reading a glance wants, not a stopwatch.
func elapsedLabel(from start: Date, to now: Date) -> String {
    let seconds = Int(now.timeIntervalSince(start))
    guard seconds >= 60 else { return "<1m" }
    let minutes = seconds / 60
    guard minutes >= 60 else { return "\(minutes)m" }
    return "\(minutes / 60)h \(minutes % 60)m"
}

/// The label and tint an ACTIVE row takes, given a slice already known to
/// qualify for the section. Mirrors `domain.StateOf`'s own precedence for a
/// slice in flight with nothing out yet: a live agent's own reading — the
/// only fact taken fresh — wins over everything else, waiting before working;
/// with no agent at all, what is left on the page is whether it is blocked on
/// a dependency, and a slice with neither is simply ready to push.
private func activeDisplay(for slice: Slice, liveAgent: AgentActivity?) -> (String, ActiveTintRole) {
    switch liveAgent {
    case .waiting:
        return ("Waiting for input", .waiting)
    case .working:
        return ("Working", .working)
    case nil:
        return slice.blocked ? ("Blocked", .blocked) : ("Ready to push", .readyToPush)
    }
}

/// A slice as a folder's file row: the glyph is read off its own status
/// (blocked wins, since a blocked slice's own status is otherwise Todo) with
/// no reference to the section it may also be drawn in.
private func sliceRow(for slice: Slice) -> MilestoneSliceRow {
    let glyph: SliceGlyph
    if slice.blocked {
        glyph = .blocked
    } else if slice.status == "Done" {
        glyph = .done
    } else if slice.status == "In progress" {
        glyph = .inProgress
    } else {
        glyph = .todo
    }

    return MilestoneSliceRow(
        sliceID: slice.id,
        name: slice.name,
        glyph: glyph,
        isBlocked: slice.blocked
    )
}
