import SwiftUI
import NatKit

struct RailView: View {
    @Bindable var appModel: AppModel
    @State private var expandedMilestones: Set<String> = []
    @State private var expandedSeeded = false

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
                        VStack(spacing: 10) {
                            ProgressView()
                                .controlSize(.small)
                            Text("Loading the plan…")
                                .font(.system(size: 11))
                                .foregroundStyle(DesignTokens.labelTertiary)
                        }
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 60)
                    } else if let message = state.errorMessage, state.projectInfo == nil {
                        VStack(alignment: .leading, spacing: 10) {
                            Label("The plan could not be loaded", systemImage: "exclamationmark.triangle")
                                .font(.system(size: 13, weight: .semibold))
                                .foregroundStyle(DesignTokens.systemYellow)
                            Text(message)
                                .font(.system(size: 11))
                                .foregroundStyle(DesignTokens.labelSecondary)
                                .textSelection(.enabled)
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
                                .font(.system(size: 10))
                            Text("Refresh failed — showing the last plan")
                                .font(.system(size: 10))
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
                                .font(.system(size: 11, weight: .semibold))
                                .foregroundStyle(DesignTokens.labelTertiary)

                            Text("NEEDS REVIEW")
                                .font(.system(size: 11, weight: .semibold))
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
                            .font(.system(size: 11, weight: .semibold))
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
        HStack(spacing: 8) {
            Circle()
                .fill(DesignTokens.systemGreen)
                .frame(width: 9, height: 9)

            Text(entry.name)
                .font(.system(size: 13, weight: .regular))
                .foregroundStyle(DesignTokens.label)
                .lineLimit(1)

            Spacer()

            if let stat = entry.stat {
                Text(stat)
                    .font(.system(size: 11, weight: .regular, design: .monospaced))
                    .foregroundStyle(
                        appModel.selectedSliceID == entry.sliceID
                            ? DesignTokens.accentText
                            : DesignTokens.labelTertiary
                    )
            }
        }
        .frame(height: 30)
        .padding(.horizontal, 8)
        .background(
            appModel.selectedSliceID == entry.sliceID
                ? DesignTokens.accent
                : Color.clear
        )
        .cornerRadius(6)
        .foregroundStyle(appModel.selectedSliceID == entry.sliceID ? DesignTokens.accentText : DesignTokens.label)
    }

    private func activeRow(for entry: ActiveEntry) -> some View {
        HStack(spacing: 8) {
            // Pulsing dot
            Circle()
                .fill(
                    entry.activity == .working
                        ? DesignTokens.systemOrange
                        : DesignTokens.systemYellow
                )
                .frame(width: 9, height: 9)
                .opacity(appModel.selectedSliceID == entry.sliceID ? 1 : 0.7)

            Text(entry.name)
                .font(.system(size: 13, weight: .regular))
                .lineLimit(1)

            Spacer()

            Text(entry.displayState)
                .font(.system(size: 11, weight: .regular))
                .foregroundStyle(
                    entry.activity == .working
                        ? DesignTokens.systemOrange
                        : DesignTokens.systemYellow
                )
        }
        .frame(height: 30)
        .padding(.horizontal, 8)
        .background(
            appModel.selectedSliceID == entry.sliceID
                ? DesignTokens.accent
                : Color.clear
        )
        .cornerRadius(6)
        .foregroundStyle(appModel.selectedSliceID == entry.sliceID ? DesignTokens.accentText : DesignTokens.label)
    }

    private func doneSummaryCard(_ summary: DoneSummary) -> some View {
        HStack(spacing: 9) {
            Image(systemName: "chevron.right")
                .font(.system(size: 9, weight: .bold))
                .foregroundStyle(DesignTokens.labelTertiary)

            progressRing(fraction: 1.0, label: "", done: true)

            Text("Done — \(summary.milestoneCount) milestone\(summary.milestoneCount > 1 ? "s" : "")")
                .font(.system(size: 11, weight: .regular))
                .foregroundStyle(DesignTokens.labelTertiary)

            Spacer()

            Text("\(summary.sliceCount)/\(summary.sliceCount)")
                .font(.system(size: 11, weight: .regular, design: .monospaced))
                .foregroundStyle(DesignTokens.labelTertiary)
        }
        .frame(height: 32)
        .padding(.horizontal, 10)
        .padding(.vertical, 0)
        .background(DesignTokens.controlBg)
        .rectBorder(width: 0.5, edges: .all, color: DesignTokens.separator)
        .cornerRadius(8)
    }

    private func milestoneCard(_ card: MilestoneCard) -> some View {
        VStack(spacing: 0) {
            HStack(spacing: 9) {
                Image(systemName: expandedMilestones.contains(card.milestoneID) ? "chevron.down" : "chevron.right")
                    .font(.system(size: 9, weight: .bold))
                    .foregroundStyle(DesignTokens.labelTertiary)

                progressRing(fraction: card.fraction, label: card.number, done: card.done == card.total, current: card.isCurrent)

                Text(card.title)
                    .font(.system(size: 13, weight: .semibold))
                    .lineLimit(1)

                Spacer()

                Text("\(card.done)/\(card.total)")
                    .font(.system(size: 11, weight: .regular, design: .monospaced))
                    .foregroundStyle(DesignTokens.labelSecondary)
            }
            .frame(height: 30)
            .padding(.horizontal, 6)
            .contentShape(Rectangle())
            .onTapGesture {
                withAnimation {
                    if expandedMilestones.contains(card.milestoneID) {
                        expandedMilestones.remove(card.milestoneID)
                    } else {
                        expandedMilestones.insert(card.milestoneID)
                    }
                }
            }

            if expandedMilestones.contains(card.milestoneID) {
                VStack(spacing: 0) {
                    ForEach(card.visibleSlices, id: \.sliceID) { slice in
                        sliceRow(slice)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                appModel.selectedSliceID = slice.sliceID
                            }
                    }

                    if card.inFlightElsewhereCount > 0 {
                        HStack(spacing: 8) {
                            Image(systemName: "chevron.right")
                                .font(.system(size: 8, weight: .bold))
                                .foregroundStyle(DesignTokens.labelQuaternary)

                            Image(systemName: "sparkles")
                                .font(.system(size: 8, weight: .bold))
                                .foregroundStyle(DesignTokens.systemOrange)

                            Text("\(card.inFlightElsewhereCount) in flight")
                                .font(.system(size: 11, weight: .regular))
                                .foregroundStyle(DesignTokens.labelTertiary)

                            Spacer()
                        }
                        .frame(height: 26)
                        .padding(.horizontal, 8)
                        .padding(.leading, 24)
                    }

                    if card.hiddenDoneCount > 0 {
                        HStack(spacing: 8) {
                            Image(systemName: "chevron.right")
                                .font(.system(size: 8, weight: .bold))
                                .foregroundStyle(DesignTokens.labelQuaternary)

                            Image(systemName: "checkmark")
                                .font(.system(size: 9, weight: .bold))
                                .foregroundStyle(DesignTokens.systemGreen)

                            Text("\(card.hiddenDoneCount) done")
                                .font(.system(size: 11, weight: .regular))
                                .foregroundStyle(DesignTokens.labelTertiary)

                            Spacer()
                        }
                        .frame(height: 26)
                        .padding(.horizontal, 8)
                        .padding(.leading, 24)
                    }
                }
                .padding(.bottom, 4)
            }
        }
        .background(DesignTokens.controlBg)
        .rectBorder(width: 0.5, edges: .all, color: DesignTokens.separator)
        .cornerRadius(8)
    }

    private func sliceRow(_ slice: MilestoneSliceRow) -> some View {
        HStack(spacing: 8) {
            Image(systemName: slice.glyph.rawValue)
                .font(.system(size: 12, weight: .regular))
                .frame(width: 13)
                .foregroundStyle(
                    slice.isBlocked ? DesignTokens.labelTertiary : DesignTokens.label
                )

            Text(slice.name)
                .font(.system(size: 13, weight: .regular))
                .lineLimit(1)
                .foregroundStyle(
                    slice.isBlocked ? DesignTokens.labelTertiary : DesignTokens.label
                )

            Spacer()
        }
        .frame(height: 28)
        .padding(.horizontal, 8)
        .padding(.leading, 24)
        .background(
            appModel.selectedSliceID == slice.sliceID
                ? DesignTokens.accent
                : Color.clear
        )
        .cornerRadius(6)
        .foregroundStyle(appModel.selectedSliceID == slice.sliceID ? DesignTokens.accentText : .primary)
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
                        .font(.system(size: 8, weight: .bold))
                        .foregroundStyle(DesignTokens.systemGreen)
                } else if !label.isEmpty {
                    Text(label)
                        .font(.system(size: 8, weight: .regular, design: .monospaced))
                        .foregroundStyle(current ? DesignTokens.accent : DesignTokens.labelSecondary)
                }
            }
            .frame(width: 15, height: 15)
        }
        .frame(width: 20, height: 20)
    }
}

#Preview {
    let appModel = AppModel()
    WindowShellView(appModel: appModel)
        .frame(width: 1360, height: 840)
}
