import Foundation

/// The activity state of an agent working on a slice.
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

/// A slice entry in the ACTIVE section.
public struct ActiveEntry: Equatable {
    public let sliceID: String
    public let name: String
    public let activity: AgentActivity

    public var displayState: String {
        switch activity {
        case .working:
            return "Working"
        case .waiting:
            return "Waiting for input"
        }
    }

    public init(sliceID: String, name: String, activity: AgentActivity) {
        self.sliceID = sliceID
        self.name = name
        self.activity = activity
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
        inFlightElsewhereCount: Int
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
    let activeSlices = slices.filter { liveAgents[$0.id] != nil }

    // NEEDS REVIEW section
    let needsReview = reviewSlices
        .sorted { $0.name < $1.name }
        .map { ReviewEntry(sliceID: $0.id, name: $0.name, stat: reviewStats[$0.id]) }

    // ACTIVE section
    let active = activeSlices
        .sorted { $0.name < $1.name }
        .compactMap { slice -> ActiveEntry? in
            guard let activity = liveAgents[slice.id] else { return nil }
            return ActiveEntry(sliceID: slice.id, name: slice.name, activity: activity)
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
                .map { slice -> MilestoneSliceRow in
                    let glyph: SliceGlyph
                    if slice.blocked {
                        glyph = .blocked
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

            // Count hidden done slices in this milestone
            let hiddenDoneCount = milestoneSlices.filter { $0.status == "Done" }.count

            // Count slices from this milestone already shown in needsReview/active
            let inFlightElsewhereCount = milestoneSlices.filter { slice in
                reviewSlices.contains { $0.id == slice.id } || activeSlices.contains { $0.id == slice.id }
            }.count

            let card = MilestoneCard(
                milestoneID: milestone.id,
                number: String(index + 1),
                title: milestone.name,
                done: doneCount,
                total: max(1, totalCount),
                isCurrent: isCurrent,
                visibleSlices: visibleSlices,
                hiddenDoneCount: hiddenDoneCount,
                inFlightElsewhereCount: inFlightElsewhereCount
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
