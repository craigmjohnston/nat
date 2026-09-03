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

    public init(sliceID: String, name: String, stat: String? = nil) {
        self.sliceID = sliceID
        self.name = name
        self.stat = stat
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

    public init(sliceID: String, name: String, displayState: String, tintRole: ActiveTintRole) {
        self.sliceID = sliceID
        self.name = name
        self.displayState = displayState
        self.tintRole = tintRole
    }
}

/// A slice glyph type.
public enum SliceGlyph: String {
    case todo = "circle"
    case inProgress = "circle.lefthalf.filled"
    case done = "checkmark.circle"
    case blocked = "nosign"
}

/// A slice row within a milestone.
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

/// A card representing a milestone in the plan.
public struct MilestoneCard: Equatable {
    public let milestoneID: String
    public let number: String // 1-based position
    public let title: String
    public let done: Int
    public let total: Int
    public let isCurrent: Bool
    public let visibleSlices: [MilestoneSliceRow]
    public let hiddenDoneCount: Int
    public let inFlightElsewhereCount: Int
    /// The Done slices `hiddenDoneCount` counts, in the plan's own order —
    /// what the "N done" row expands in place to list.
    public let doneSlices: [MilestoneSliceRow]
    /// The slices `inFlightElsewhereCount` counts — the ones already drawn in
    /// NEEDS REVIEW or ACTIVE — in the plan's own order, what the "N in
    /// flight" row expands in place to list.
    public let inFlightSlices: [MilestoneSliceRow]

    public var fraction: Double {
        guard total > 0 else { return 0 }
        return Double(done) / Double(total)
    }

    public init(
        milestoneID: String,
        number: String,
        title: String,
        done: Int,
        total: Int,
        isCurrent: Bool,
        visibleSlices: [MilestoneSliceRow],
        hiddenDoneCount: Int,
        inFlightElsewhereCount: Int,
        doneSlices: [MilestoneSliceRow] = [],
        inFlightSlices: [MilestoneSliceRow] = []
    ) {
        self.milestoneID = milestoneID
        self.number = number
        self.title = title
        self.done = done
        self.total = total
        self.isCurrent = isCurrent
        self.visibleSlices = visibleSlices
        self.hiddenDoneCount = hiddenDoneCount
        self.inFlightElsewhereCount = inFlightElsewhereCount
        self.doneSlices = doneSlices
        self.inFlightSlices = inFlightSlices
    }
}

/// A summary of done milestones (collapsed into one row).
public struct DoneSummary: Equatable {
    public let milestoneCount: Int
    public let sliceCount: Int

    public init(milestoneCount: Int, sliceCount: Int) {
        self.milestoneCount = milestoneCount
        self.sliceCount = sliceCount
    }
}

/// The data model for the left rail.
public struct RailModel: Equatable {
    /// Slices waiting for review (handed_back).
    public let needsReview: [ReviewEntry]

    /// Slices with active agents.
    public let active: [ActiveEntry]

    /// Milestone cards for non-done milestones, in plan order.
    public let milestoneCards: [MilestoneCard]

    /// Summary of fully-done milestones, if any.
    public let doneSummary: DoneSummary?

    public init(
        needsReview: [ReviewEntry],
        active: [ActiveEntry],
        milestoneCards: [MilestoneCard],
        doneSummary: DoneSummary?
    ) {
        self.needsReview = needsReview
        self.active = active
        self.milestoneCards = milestoneCards
        self.doneSummary = doneSummary
    }
}

// MARK: - Rail Model Builder

/// Builds a rail model from project info and live agents.
///
/// `reviewStats` is `ReviewStatsStore.stats`, keyed by slice id — passed in
/// rather than read here, since a view model has no business reaching for a
/// store of its own (mirrors how `liveAgents` is already handed in rather
/// than read off `ActivityStore` directly).
public func buildRailModel(
    from projectInfo: ProjectInfo,
    liveAgents: [String: AgentActivity],
    reviewStats: [String: String] = [:]
) -> RailModel {
    let slices = projectInfo.slices
    let milestones = projectInfo.milestones
    let reviewSlices = slices.filter { $0.handedBack }

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

    // NEEDS REVIEW section
    let needsReview = reviewSlices
        .sorted { $0.name < $1.name }
        .map { ReviewEntry(sliceID: $0.id, name: $0.name, stat: reviewStats[$0.id]) }

    // ACTIVE section
    let active = activeSlices
        .sorted { $0.name < $1.name }
        .map { slice -> ActiveEntry in
            let (displayState, tintRole) = activeDisplay(for: slice, liveAgent: liveAgents[slice.id])
            return ActiveEntry(sliceID: slice.id, name: slice.name, displayState: displayState, tintRole: tintRole)
        }

    // Milestone cards and done summary
    let sortedMilestones = milestones.sorted { $0.order < $1.order }

    var milestoneCards: [MilestoneCard] = []
    var doneMilestones: [Milestone] = []
    var doneSliceCount = 0

    for (index, milestone) in sortedMilestones.enumerated() {
        let milestoneDone = milestone.status == "Done"

        if milestoneDone {
            doneMilestones.append(milestone)
            // Count done slices in this milestone
            let doneSluces = slices.filter { $0.milestoneID == milestone.id && $0.status == "Done" }
            doneSliceCount += doneSluces.count
        } else {
            // Build milestone card for active milestones
            let milestoneSlices = slices.filter { $0.milestoneID == milestone.id }
            let doneCount = milestoneSlices.filter { $0.status == "Done" }.count
            let totalCount = milestoneSlices.count

            // Find the first non-done milestone with any non-done slice
            let isCurrent = milestoneSlices.contains { $0.status != "Done" } && !doneMilestones.isEmpty == false && milestoneCards.isEmpty

            // Visible slices: not Done, in plan order
            let visibleSlices = milestoneSlices
                .filter { $0.status != "Done" }
                .map { sliceRow(for: $0) }

            // The Done slices this milestone hides, in plan order — what the
            // "N done" row expands to.
            let doneSlices = milestoneSlices
                .filter { $0.status == "Done" }
                .map { sliceRow(for: $0) }

            // The slices of this milestone already drawn in NEEDS REVIEW or
            // ACTIVE, in plan order — what the "N in flight" row expands to.
            let inFlightSlices = milestoneSlices
                .filter { slice in
                    reviewSlices.contains { $0.id == slice.id } || activeSlices.contains { $0.id == slice.id }
                }
                .map { sliceRow(for: $0) }

            let card = MilestoneCard(
                milestoneID: milestone.id,
                number: String(index + 1),
                title: milestone.name,
                done: doneCount,
                total: max(1, totalCount),
                isCurrent: isCurrent,
                visibleSlices: visibleSlices,
                hiddenDoneCount: doneSlices.count,
                inFlightElsewhereCount: inFlightSlices.count,
                doneSlices: doneSlices,
                inFlightSlices: inFlightSlices
            )
            milestoneCards.append(card)
        }
    }

    let doneSummary = doneMilestones.isEmpty ? nil : DoneSummary(
        milestoneCount: doneMilestones.count,
        sliceCount: doneSliceCount
    )

    return RailModel(
        needsReview: needsReview,
        active: active,
        milestoneCards: milestoneCards,
        doneSummary: doneSummary
    )
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

/// A slice as a milestone-card row: the glyph is read off its own status
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
