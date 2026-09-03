import SwiftUI
import NatKit

/// Fixed-width slots shared by every row in a milestone card (and the
/// done-summary card above them), so that expanding, collapsing or flipping a
/// chevron between `chevron.right` and `chevron.down` never shifts anything
/// beside it. `glyph` is also the width of the "N done"/"N in flight" rows'
/// own status icon — chosen so that icon lands in the same column as a slice
/// row's glyph: those rows draw their chevron in a leading indent shrunk by
/// exactly `chevron + spacing`, so the chevron they add sits to the left of
/// where a plain slice row starts rather than pushing everything over.
private enum RailSlot {
    static let chevron: CGFloat = 12
    static let glyph: CGFloat = 16
    static let ring: CGFloat = 20
    /// The horizontal padding every card row shares.
    static let rowHPad: CGFloat = 8
    /// The indent a slice row (and, after subtracting its own chevron, a
    /// summary row) is drawn at.
    static let indent: CGFloat = 24
    static let spacing: CGFloat = 8
}

struct RailView: View {
    @Bindable var appModel: AppModel
    @State private var expandedMilestones: Set<String> = []
    @State private var expandedSeeded = false
    /// Milestone IDs whose "N done" row is expanded in place. Keyed by
    /// milestone rather than cleared on any read of the plan: a fold here is
    /// the user's own, exactly like `expandedMilestones`.
    @State private var expandedDoneLists: Set<String> = []
    /// Milestone IDs whose "N in flight" row is expanded in place.
    @State private var expandedInFlightLists: Set<String> = []

    var railModel: RailModel {
        if let projectInfo = appModel.projectStore?.state.projectInfo {
            // Map ActivityStore agents to the rail model's format
            var liveAgents: [String: AgentActivity] = [:]
            for (sliceID, status) in appModel.activityStore?.agents ?? [:] {
                switch status.activity {
                case .working:
                    liveAgents[sliceID] = .working
                case .waiting:
                    liveAgents[sliceID] = .waiting
                case .unknown:
                    // Treat unknown as working (TUI convention)
                    liveAgents[sliceID] = .working
                }
            }
            return buildRailModel(
                from: projectInfo, liveAgents: liveAgents,
                reviewStats: appModel.reviewStatsStore?.stats ?? [:]
            )
        }
        return RailModel(needsReview: [], active: [], milestoneCards: [], doneSummary: nil)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                // The load's own states come first: a board that swallowed
                // its failure would read as an empty tracker, which is worse
                // than any error. A first load spins, a failed first load
                // says what nat said and offers the retry, and a failed
                // refresh keeps the stale plan under one quiet warning line
                // (the TUI convention: a failure leaves the board as it was).
                if let state = appModel.projectStore?.state {
                    if state.isLoading && state.projectInfo == nil {
                        QuietLoadingView(label: "Loading the plan…")
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 60)
                    } else if let message = state.errorMessage, state.projectInfo == nil {
                        VStack(alignment: .leading, spacing: 10) {
                            Label("The plan could not be loaded", systemImage: "exclamationmark.triangle")
                                .font(.system(size: Typo.body, weight: .semibold))
                                .foregroundStyle(DesignTokens.systemYellow)
                            Text(message)
                                .font(.system(size: Typo.caption))
                                .foregroundStyle(DesignTokens.labelSecondary)
                            Button("Try Again") {
                                Task { await appModel.refresh() }
                            }
                        }
                        .padding(12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(DesignTokens.controlBg)
                        .cornerRadius(8)
                    } else if let message = state.errorMessage {
                        HStack(spacing: 6) {
                            Image(systemName: "exclamationmark.triangle")
                                .font(.system(size: Typo.caption))
                            Text("Refresh failed — showing the last plan")
                                .font(.system(size: Typo.caption))
                        }
                        .foregroundStyle(DesignTokens.systemYellow)
                        .padding(.horizontal, 8)
                        .padding(.bottom, 6)
                        .help(message)
                    }
                }

                // NEEDS REVIEW section
                if !railModel.needsReview.isEmpty {
                    VStack(alignment: .leading, spacing: 0) {
                        HStack(spacing: 6) {
                            Image(systemName: "checkmark.seal")
                                .font(.system(size: Typo.caption, weight: .semibold))
                                .foregroundStyle(DesignTokens.labelTertiary)

                            Text("NEEDS REVIEW")
                                .font(.system(size: Typo.caption, weight: .semibold))
                                .foregroundStyle(DesignTokens.labelTertiary)
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 5)

                        ForEach(railModel.needsReview, id: \.sliceID) { entry in
                            reviewRow(for: entry)
                                .contentShape(Rectangle())
                                .onTapGesture {
                                    appModel.selectedSliceID = entry.sliceID
                                }
                        }
                    }
                    .padding(.bottom, 10)
                }

                // ACTIVE section
                if !railModel.active.isEmpty {
                    VStack(alignment: .leading, spacing: 0) {
                        Text("ACTIVE")
                            .font(.system(size: Typo.caption, weight: .semibold))
                            .foregroundStyle(DesignTokens.labelTertiary)
                            .padding(.horizontal, 8)
                            .padding(.vertical, 10)

                        ForEach(railModel.active, id: \.sliceID) { entry in
                            activeRow(for: entry)
                                .contentShape(Rectangle())
                                .onTapGesture {
                                    appModel.selectedSliceID = entry.sliceID
                                }
                        }
                    }
                    .padding(.bottom, 10)
                }

                // The rule under the flight sections exists only where they
                // do — an empty board opening with a bare line would read as
                // chrome missing its content.
                if !railModel.needsReview.isEmpty || !railModel.active.isEmpty {
                    Divider()
                        .frame(height: 0.5)
                        .foregroundStyle(DesignTokens.separator)
                        .padding(.vertical, 10)
                }

                // Done summary
                if let summary = railModel.doneSummary {
                    doneSummaryCard(summary)
                        .padding(.bottom, 8)
                }

                // Milestone cards
                VStack(spacing: 8) {
                    ForEach(railModel.milestoneCards, id: \.milestoneID) { card in
                        milestoneCard(card)
                    }
                }
            }
            .padding(.vertical, 12)
            .padding(.horizontal, 12)
        }
        .background(DesignTokens.windowBg)
        .rectBorderTrailing(width: 0.5, color: DesignTokens.separator)
        // The current milestone opens itself the first time a plan lands, the
        // way the mock draws it; everything after that is the user's folding.
        .onChange(of: railModel.milestoneCards.isEmpty, initial: true) { _, isEmpty in
            guard !expandedSeeded, !isEmpty else { return }
            expandedSeeded = true
            if let current = railModel.milestoneCards.first(where: { $0.isCurrent }) ?? railModel.milestoneCards.first {
                expandedMilestones.insert(current.milestoneID)
            }
        }
    }

    // MARK: - Row Builders

    private func reviewRow(for entry: ReviewEntry) -> some View {
        let selected = appModel.selectedSliceID == entry.sliceID
        return HStack(spacing: 8) {
            Circle()
                .fill(selected ? DesignTokens.accentText : DesignTokens.systemGreen)
                .frame(width: 9, height: 9)

            Text(entry.name)
                .font(.system(size: Typo.body, weight: .regular))
                .foregroundStyle(selected ? DesignTokens.accentText : DesignTokens.label)
                .lineLimit(1)

            Spacer()

            if let stat = entry.stat {
                Text(stat)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .monospacedDigit()
                    .foregroundStyle(selected ? DesignTokens.accentText : DesignTokens.labelTertiary)
            }
        }
        .frame(height: 30)
        .padding(.horizontal, 8)
        .background(selected ? DesignTokens.accent : Color.clear)
        .cornerRadius(6)
    }

    private func activeRow(for entry: ActiveEntry) -> some View {
        let selected = appModel.selectedSliceID == entry.sliceID
        let tint = tintColor(for: entry.tintRole, selected: selected)
        // Only a live agent is worth pulling the eye to; a row with nothing
        // running on it (blocked, or simply ready to push) sits still.
        let isLive = entry.tintRole == .working || entry.tintRole == .waiting

        return HStack(spacing: 8) {
            dotView(color: tint, pulsing: isLive && !selected)

            Text(entry.name)
                .font(.system(size: Typo.body, weight: .regular))
                .foregroundStyle(selected ? DesignTokens.accentText : DesignTokens.label)
                .lineLimit(1)

            Spacer()

            Text(entry.displayState)
                .font(.system(size: Typo.subhead, weight: .regular))
                .foregroundStyle(tint)
        }
        .frame(height: 30)
        .padding(.horizontal, 8)
        .background(selected ? DesignTokens.accent : Color.clear)
        .cornerRadius(6)
    }

    @ViewBuilder
    private func dotView(color: Color, pulsing: Bool) -> some View {
        if pulsing {
            Circle()
                .fill(color)
                .frame(width: 9, height: 9)
                .modifier(PulseModifier())
        } else {
            Circle()
                .fill(color)
                .frame(width: 9, height: 9)
        }
    }

    private func tintColor(for role: ActiveTintRole, selected: Bool) -> Color {
        if selected { return DesignTokens.accentText }
        switch role {
        case .working: return DesignTokens.systemOrange
        case .waiting: return DesignTokens.systemYellow
        case .blocked: return DesignTokens.labelTertiary
        case .readyToPush: return DesignTokens.systemGreen
        }
    }

    private func doneSummaryCard(_ summary: DoneSummary) -> some View {
        HStack(spacing: 9) {
            Image(systemName: "chevron.right")
                .font(.system(size: 10, weight: .semibold))
                .frame(width: RailSlot.chevron)
                .foregroundStyle(DesignTokens.labelTertiary)

            progressRing(fraction: 1.0, label: "", done: true)

            Text("Done — \(summary.milestoneCount) milestone\(summary.milestoneCount > 1 ? "s" : "")")
                .font(.system(size: Typo.subhead, weight: .regular))
                .foregroundStyle(DesignTokens.labelTertiary)

            Spacer()

            Text("\(summary.sliceCount)/\(summary.sliceCount)")
                .font(.system(size: Typo.subhead, weight: .regular))
                .monospacedDigit()
                .foregroundStyle(DesignTokens.labelTertiary)
        }
        .frame(height: 32)
        .padding(.horizontal, 6)
        .background(DesignTokens.controlBg)
        .rectBorder(width: 0.5, edges: .all, color: DesignTokens.separator)
        .cornerRadius(8)
    }

    private func milestoneCard(_ card: MilestoneCard) -> some View {
        let expanded = expandedMilestones.contains(card.milestoneID)

        return VStack(spacing: 0) {
            HStack(spacing: 9) {
                Image(systemName: expanded ? "chevron.down" : "chevron.right")
                    .font(.system(size: 10, weight: .semibold))
                    .frame(width: RailSlot.chevron)
                    .foregroundStyle(DesignTokens.labelTertiary)

                progressRing(fraction: card.fraction, label: card.number, done: card.done == card.total, current: card.isCurrent)

                Text(card.title)
                    .font(.system(size: Typo.body, weight: .semibold))
                    .lineLimit(1)

                Spacer()

                Text("\(card.done)/\(card.total)")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .monospacedDigit()
                    .foregroundStyle(DesignTokens.labelSecondary)
            }
            .frame(height: 32)
            .padding(.horizontal, 6)
            .contentShape(Rectangle())
            .onTapGesture {
                withAnimation(Motion.stateChange) {
                    if expanded {
                        expandedMilestones.remove(card.milestoneID)
                    } else {
                        expandedMilestones.insert(card.milestoneID)
                    }
                }
            }

            if expanded {
                VStack(spacing: 0) {
                    ForEach(card.visibleSlices, id: \.sliceID) { slice in
                        sliceRow(slice)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                appModel.selectedSliceID = slice.sliceID
                            }
                    }

                    if card.inFlightElsewhereCount > 0 {
                        expandableSummaryRow(
                            count: card.inFlightElsewhereCount,
                            label: "in flight",
                            icon: "sparkles",
                            iconColor: DesignTokens.systemOrange,
                            expanded: expandedInFlightLists.contains(card.milestoneID),
                            onToggle: {
                                withAnimation(Motion.stateChange) {
                                    toggle(card.milestoneID, in: &expandedInFlightLists)
                                }
                            }
                        )

                        if expandedInFlightLists.contains(card.milestoneID) {
                            ForEach(card.inFlightSlices, id: \.sliceID) { slice in
                                sliceRow(slice)
                                    .contentShape(Rectangle())
                                    .onTapGesture {
                                        appModel.selectedSliceID = slice.sliceID
                                    }
                            }
                        }
                    }

                    if card.hiddenDoneCount > 0 {
                        expandableSummaryRow(
                            count: card.hiddenDoneCount,
                            label: "done",
                            icon: "checkmark",
                            iconColor: DesignTokens.systemGreen,
                            expanded: expandedDoneLists.contains(card.milestoneID),
                            onToggle: {
                                withAnimation(Motion.stateChange) {
                                    toggle(card.milestoneID, in: &expandedDoneLists)
                                }
                            }
                        )

                        if expandedDoneLists.contains(card.milestoneID) {
                            ForEach(card.doneSlices, id: \.sliceID) { slice in
                                doneSliceRow(slice)
                                    .contentShape(Rectangle())
                                    .onTapGesture {
                                        appModel.selectedSliceID = slice.sliceID
                                    }
                            }
                        }
                    }
                }
                .padding(.bottom, 4)
            }
        }
        .background(DesignTokens.controlBg)
        .rectBorder(width: 0.5, edges: .all, color: DesignTokens.separator)
        .cornerRadius(8)
    }

    private func toggle(_ id: String, in set: inout Set<String>) {
        if set.contains(id) {
            set.remove(id)
        } else {
            set.insert(id)
        }
    }

    /// The clickable "N done"/"N in flight" row: a chevron in the same
    /// `RailSlot.chevron`-wide frame every other chevron in the card uses, and
    /// a status icon in the same `RailSlot.glyph`-wide frame a slice row's own
    /// glyph takes — reached by shrinking the row's leading indent by the
    /// chevron and the spacing after it, so the icon (not the chevron) is what
    /// lines up with the column above and below it.
    private func expandableSummaryRow(
        count: Int,
        label: String,
        icon: String,
        iconColor: Color,
        expanded: Bool,
        onToggle: @escaping () -> Void
    ) -> some View {
        HStack(spacing: RailSlot.spacing) {
            Image(systemName: expanded ? "chevron.down" : "chevron.right")
                .font(.system(size: 10, weight: .semibold))
                .frame(width: RailSlot.chevron)
                .foregroundStyle(DesignTokens.labelQuaternary)

            Image(systemName: icon)
                .font(.system(size: 12, weight: .medium))
                .frame(width: RailSlot.glyph)
                .foregroundStyle(iconColor)

            Text("\(count) \(label)")
                .font(.system(size: Typo.subhead, weight: .regular))
                .foregroundStyle(DesignTokens.labelTertiary)

            Spacer()
        }
        .frame(height: 26)
        .padding(.horizontal, RailSlot.rowHPad)
        .padding(.leading, RailSlot.indent - RailSlot.chevron - RailSlot.spacing)
        .contentShape(Rectangle())
        .onTapGesture(perform: onToggle)
    }

    private func sliceRow(_ slice: MilestoneSliceRow) -> some View {
        let selected = appModel.selectedSliceID == slice.sliceID
        let contentColor = selected
            ? DesignTokens.accentText
            : (slice.isBlocked ? DesignTokens.labelTertiary : DesignTokens.label)

        return HStack(spacing: RailSlot.spacing) {
            Image(systemName: slice.glyph.rawValue)
                .font(.system(size: 14, weight: .medium))
                .frame(width: RailSlot.glyph)
                .foregroundStyle(contentColor)

            Text(slice.name)
                .font(.system(size: Typo.body, weight: .regular))
                .lineLimit(1)
                .foregroundStyle(contentColor)

            Spacer()
        }
        .frame(height: 32)
        .padding(.horizontal, RailSlot.rowHPad)
        .padding(.leading, RailSlot.indent)
        .background(selected ? DesignTokens.accent : Color.clear)
        .cornerRadius(6)
    }

    /// A Done slice listed under an expanded "N done" row: the checkmark
    /// glyph always green and the name always dimmed, whether or not it is
    /// selected recolors both to `accentText` the way every other row does.
    private func doneSliceRow(_ slice: MilestoneSliceRow) -> some View {
        let selected = appModel.selectedSliceID == slice.sliceID

        return HStack(spacing: RailSlot.spacing) {
            Image(systemName: slice.glyph.rawValue)
                .font(.system(size: 14, weight: .medium))
                .frame(width: RailSlot.glyph)
                .foregroundStyle(selected ? DesignTokens.accentText : DesignTokens.systemGreen)

            Text(slice.name)
                .font(.system(size: Typo.body, weight: .regular))
                .lineLimit(1)
                .foregroundStyle(selected ? DesignTokens.accentText : DesignTokens.labelTertiary)

            Spacer()
        }
        .frame(height: 32)
        .padding(.horizontal, RailSlot.rowHPad)
        .padding(.leading, RailSlot.indent)
        .background(selected ? DesignTokens.accent : Color.clear)
        .cornerRadius(6)
    }

    private func progressRing(fraction: Double, label: String, done: Bool = false, current: Bool = false) -> some View {
        ZStack {
            Circle()
                .stroke(DesignTokens.labelQuaternary, lineWidth: 1)

            Circle()
                .trim(from: 0, to: fraction)
                .stroke(
                    done ? DesignTokens.systemGreen : DesignTokens.accent,
                    style: StrokeStyle(lineWidth: 1, lineCap: .round)
                )
                .rotationEffect(.degrees(-90))

            ZStack {
                Circle()
                    .fill(DesignTokens.controlBg)

                if done {
                    Image(systemName: "checkmark")
                        .font(.system(size: Typo.caption, weight: .semibold))
                        .foregroundStyle(DesignTokens.systemGreen)
                } else if !label.isEmpty {
                    Text(label)
                        .font(.system(size: Typo.caption, weight: .regular))
                        .monospacedDigit()
                        .foregroundStyle(current ? DesignTokens.accent : DesignTokens.labelSecondary)
                }
            }
            .frame(width: 16, height: 16)
        }
        .frame(width: RailSlot.ring, height: RailSlot.ring)
    }
}

#Preview {
    let appModel = AppModel()
    WindowShellView(appModel: appModel)
        .frame(width: 1360, height: 840)
}
