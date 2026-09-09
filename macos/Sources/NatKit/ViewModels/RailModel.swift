import Foundation

/// The activity state of an agent working on a slice, as read fresh off the
/// live tmux map. It refines an ACTIVE row's label only — never whether the
/// row is drawn at all, which `buildRailModel` decides from the slice's own
/// page.
public enum AgentActivity {
    case working
    case waiting
}

/// A slice entry in the NEEDS REVIEW section.
public struct ReviewEntry: Equatable {
    public let sliceID: String
    public let name: String
    /// The branch's own diff totals, "+N −N" tabular — nil until
    /// `ReviewStatsStore` has fetched them (or their fetch failed), in which
    /// case the row simply draws with no stat rather than a placeholder.
    public let stat: String?
    /// The milestone the slice is filed under — the row's second line names
    /// it, since the slice no longer appears inside that milestone's folder.
    public let milestone: String
    /// How many files the branch touched, from the same fetch as `stat` and
    /// nil for the same reasons.
    public let fileCount: Int?

    public init(sliceID: String, name: String, stat: String? = nil, milestone: String = "", fileCount: Int? = nil) {
        self.sliceID = sliceID
        self.name = name
        self.stat = stat
        self.milestone = milestone
        self.fileCount = fileCount
    }
}

/// The semantic tint an ACTIVE row's dot and status text take. Named for
/// meaning rather than a color, so this view model stays free of picking a
/// palette — `RailView` is what maps each case onto a `DesignTokens` color.
public enum ActiveTintRole: Equatable {
    case working
    case waiting
    case blocked
    case readyToPush
}

/// A slice entry in the ACTIVE section.
public struct ActiveEntry: Equatable {
    public let sliceID: String
    public let name: String
    /// The row's own label — "Working", "Waiting for input", "Blocked" or
    /// "Ready to push" — already resolved by `buildRailModel`, since the rule
    /// it comes from is the view model's and not the view's to know.
    public let displayState: String
    public let tintRole: ActiveTintRole
    /// The milestone the slice is filed under, for the row's second line.
    public let milestone: String
    /// How long the agent has been on the slice — "14m", "1h 4m" — measured
    /// from when the activity poll first saw its session, and nil for a row
    /// with no live agent (blocked, or simply ready to push).
    public let elapsed: String?

    public init(
        sliceID: String,
        name: String,
        displayState: String,
        tintRole: ActiveTintRole,
        milestone: String = "",
        elapsed: String? = nil
    ) {
        self.sliceID = sliceID
        self.name = name
        self.displayState = displayState
        self.tintRole = tintRole
        self.milestone = milestone
        self.elapsed = elapsed
    }
}

/// The semantic tint the WORKSHOP row's dot and status text take — its own
/// enum rather than `ActiveTintRole`, since a workshop is never blocked or
/// ready to push and an ACTIVE row is never launching or being composed.
public enum WorkshopTintRole: Equatable {
    case working
    case waiting
    case launching
    /// The composer is open and nothing has started yet.
    case new
}

/// The WORKSHOP section's one row: the planning agent, live or launching.
public struct WorkshopEntry: Equatable {
    /// The row's own label — "Working", "Waiting for input" or "Launching…"
    /// — resolved by `buildWorkshopEntry` the way `ActiveEntry`'s is.
    public let displayState: String
    public let tintRole: WorkshopTintRole
    /// How long the planning agent has been live, measured like an ACTIVE
    /// row's — nil while launching, and for an agent the poll has no stamp
    /// for.
    public let elapsed: String?

    public init(displayState: String, tintRole: WorkshopTintRole, elapsed: String? = nil) {
        self.displayState = displayState
        self.tintRole = tintRole
        self.elapsed = elapsed
    }
}

/// Builds the WORKSHOP row, or nil when the section has nothing to draw: no
/// planning agent live, no launch in flight, and the row not selected — a
/// selected row with nothing running is the composer being typed into, drawn
/// so the rail's selection stays visible while it is. A live agent wins over
/// both flags — it is the only reading taken fresh.
public func buildWorkshopEntry(
    activity: AgentActivity?,
    isLaunching: Bool,
    isSelected: Bool = false,
    firstSeen: Date? = nil,
    now: Date = Date()
) -> WorkshopEntry? {
    switch activity {
    case .working:
        return WorkshopEntry(
            displayState: "Working",
            tintRole: .working,
            elapsed: firstSeen.map { elapsedLabel(from: $0, to: now) }
        )
    case .waiting:
        return WorkshopEntry(
            displayState: "Waiting for input",
            tintRole: .waiting,
            elapsed: firstSeen.map { elapsedLabel(from: $0, to: now) }
        )
    case nil:
        if isLaunching {
            return WorkshopEntry(displayState: "Launching…", tintRole: .launching)
        }
        guard isSelected else { return nil }
        return WorkshopEntry(displayState: "New session", tintRole: .new)
    }
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

    /// The milestone's slices drawn in NEEDS REVIEW or ACTIVE instead of
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
    /// Slices waiting for review (handed_back).
    public let needsReview: [ReviewEntry]

    /// Slices with active agents.
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
        needsReview: [ReviewEntry],
        active: [ActiveEntry],
        todoFolders: [MilestoneFolder],
        doneFolders: [MilestoneFolder] = [],
        doneSummary: DoneSummary? = nil
    ) {
        self.needsReview = needsReview
        self.active = active
        self.todoFolders = todoFolders
        self.doneFolders = doneFolders
        self.doneSummary = doneSummary
    }
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
public func buildRailModel(
    from projectInfo: ProjectInfo,
    liveAgents: [String: AgentActivity],
    reviewStats: [String: String] = [:],
    reviewFileCounts: [String: Int] = [:],
    prReadiness: [String: String] = [:],
    agentStarts: [String: Date] = [:],
    now: Date = Date()
) -> RailModel {
    let slices = projectInfo.slices
    let milestones = projectInfo.milestones

    // The slices whose pull request is positively read as open — what
    // `sliceWorkDone` gates a Done status behind, so the DONE folders, their
    // counts and the summary all read done-ness by the same rule the
    // progress bar does: merged is done.
    let openPRs = Set(prReadiness.keys)

    // NEEDS REVIEW holds the work a review still owes something: a branch
    // handed back and not yet approved, and — `prReadiness` being the slices
    // whose pull request is positively read as open — a slice approved and
    // waiting on the merge, since the board marks a slice Done as it opens
    // the pull request and the review is not over until that lands. With no
    // reading taken (app just opened, gh unreachable) the second kind is
    // simply absent, which is also what keeps every Done slice a project
    // ever finished from flooding the section.
    let reviewSlices = slices.filter { $0.handedBack || prReadiness[$0.id] != nil }

    // A milestone's name off its ID, for the session rows' second lines.
    let milestoneNames: [String: String] = milestones.reduce(into: [:]) { $0[$1.id] = $1.name }

    // ACTIVE membership mirrors the gate `domain.StateOf` applies before a
    // live agent ever enters into it: In progress, not handed back, and no
    // pull request recorded. It is never "has a live tmux session" — a
    // session can outlive the slice it was launched on (left idle on a Done
    // slice, or on one already handed back), and none of that is this
    // section's to draw. What a live agent refines is the label alone, in
    // `activeDisplay` below.
    let activeSlices = slices.filter {
        $0.status == "In progress" && !$0.handedBack && $0.pr.isEmpty
    }

    // NEEDS REVIEW section. A handed-back slice's meta is its diff tally;
    // a slice in the section for its open pull request has no branch stats
    // to show, and its meta is the reading's own words — "awaiting review",
    // "ready to merge" — which is exactly what is being waited on.
    let needsReview = reviewSlices
        .sorted { $0.name < $1.name }
        .map {
            ReviewEntry(
                sliceID: $0.id,
                name: $0.name,
                stat: reviewStats[$0.id] ?? prReadiness[$0.id],
                milestone: milestoneNames[$0.milestoneID] ?? "",
                fileCount: reviewFileCounts[$0.id]
            )
        }

    // ACTIVE section
    let active = activeSlices
        .sorted { $0.name < $1.name }
        .map { slice -> ActiveEntry in
            let liveAgent = liveAgents[slice.id]
            let (displayState, tintRole) = activeDisplay(for: slice, liveAgent: liveAgent)
            // Elapsed rides the live reading: a row with no agent on it has
            // no session to have started, whatever `agentStarts` still says.
            let elapsed = liveAgent == nil
                ? nil
                : agentStarts[slice.id].map { elapsedLabel(from: $0, to: now) }
            return ActiveEntry(
                sliceID: slice.id,
                name: slice.name,
                displayState: displayState,
                tintRole: tintRole,
                milestone: milestoneNames[slice.milestoneID] ?? "",
                elapsed: elapsed
            )
        }

    // The slices already drawn in a session section — never repeated inside
    // a TODO folder, so a slice is one row of the rail and not two.
    let inFlightIDs = Set(reviewSlices.map(\.id)).union(activeSlices.map(\.id))

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
        needsReview: needsReview,
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
