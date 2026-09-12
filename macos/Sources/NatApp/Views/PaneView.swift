import SwiftUI
import NatKit

struct PaneView: View {
    @Environment(\.ground) private var ground
    @Bindable var appModel: AppModel
    @State private var currentTab: WorkflowTab = .brief

    var selectedSlice: Slice? {
        guard let sliceID = appModel.selectedSliceID else { return nil }
        return appModel.projectStore?.state.projectInfo?.slices.first { $0.id == sliceID }
    }

    var workflowState: WorkflowTabState? {
        guard let slice = selectedSlice else { return nil }
        let hasLiveAgent = appModel.selectedSliceID.flatMap { sliceID in
            appModel.activityStore?.agents[sliceID] != nil
        } ?? false
        return buildWorkflowTabState(for: slice, hasLiveAgent: hasLiveAgent)
    }

    /// The slice's milestone name, for the breadcrumb above the title — nil
    /// where the milestone can't be found, which drops the line entirely
    /// rather than showing a blank one.
    func milestoneName(for slice: Slice) -> String? {
        appModel.projectStore?.state.projectInfo?.milestones.first { $0.id == slice.milestoneID }?.name
    }

    var body: some View {
        VStack(spacing: 0) {
            // The workshop row's pane: no workflow strip, just the planning
            // agent — see WorkshopPaneView.
            if appModel.workshopSelected {
                WorkshopPaneView(appModel: appModel)
            } else if let slice = selectedSlice, let tabState = workflowState {
                // Header: the shared pane chrome — the identity block
                // (breadcrumb + title, which wraps rather than truncating)
                // with the pipeline stepper as its trailing content, reading
                // the slice's progress through the pipeline. The workshop
                // pane opens with the same header and nothing on the right.
                PaneHeader(breadcrumb: milestoneName(for: slice), title: slice.name) {
                    // The stepper: stages rather than tabs, each drawn by
                    // where the slice actually stands (complete/current/
                    // reachable/locked) instead of an equal row of labels.
                    HStack(spacing: 10) {
                        ForEach(Array(tabState.tabs.enumerated()), id: \.offset) { index, tab in
                            if index > 0 {
                                stepperSeparator(lit: tabState.isSeparatorLit(before: tab))
                            }

                            stepperStage(
                                tab,
                                isCurrentTab: currentTab == tab,
                                isReachable: tabState.isReachable(tab),
                                isComplete: tabState.isComplete(tab)
                            )
                        }
                    }
                }

                // Content area
                switch currentTab {
                case .brief:
                    BriefTabView(appModel: appModel, slice: slice, onTabChange: { tab in
                        currentTab = tab
                    })
                case .agent:
                    AgentTabView(appModel: appModel, slice: slice)
                case .diff:
                    DiffTabView(appModel: appModel, slice: slice, onApproved: {
                        currentTab = .pr
                    })
                case .pr:
                    PRTabView(appModel: appModel, slice: slice)
                }
            } else {
                // Empty state — "select a slice" only where there are slices
                // to select. A project just opened or created from the "+"
                // tab has none, and what it needs said is what to do next.
                VStack {
                    VStack(spacing: 8) {
                        Image(systemName: "doc.text")
                            .font(.system(size: 32, weight: .regular))
                            .ink(.secondary)

                        if appModel.activePlanIsEmpty {
                            Text(EmptyProjectNote.title)
                                .font(.system(size: Typo.body, weight: .regular))
                                .ink(.secondary)

                            Text(EmptyProjectNote.subtitle(needsWorkingDir: appModel.activeProjectNeedsWorkingDir))
                                .font(.system(size: Typo.subhead, weight: .regular))
                                .ink(.tertiary)
                                .multilineTextAlignment(.center)
                                .frame(maxWidth: 380)
                        } else {
                            Text("Select a slice to begin")
                                .font(.system(size: Typo.body, weight: .regular))
                                .ink(.secondary)
                        }
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                }
                .surface(.card)
            }
        }
        .onChange(of: appModel.selectedSliceID) { _, _ in
            // Reset tab when slice changes
            currentTab = workflowState?.defaultTab ?? .brief
        }
    }

    // MARK: - Stepper Separator

    /// The step from one stage to the next: an arrow rather than a rule,
    /// since what it marks is a direction and not a division. It is lit
    /// where both stages it joins are reachable — the step is one that can
    /// actually be taken — and muted otherwise, in the same quaternary the
    /// locked stages' own glyphs are drawn in, so a run of locked stages
    /// recedes as one thing rather than as stages behind lit arrows.
    private func stepperSeparator(lit: Bool) -> some View {
        Image(systemName: "chevron.right")
            .font(.system(size: 9, weight: .semibold))
            .ink(lit ? .secondary : .quaternary)
            .frame(width: 12)
            .accessibilityHidden(true)
    }

    // MARK: - Stepper Stage

    /// One stage of the pipeline stepper. State is read off the same
    /// `WorkflowTabState` the tabs always used — a stage can be both complete
    /// and current (its check stays green, the current wash still applies) —
    /// so there's nothing new to keep in sync with the content switch below.
    /// Unlike the old tabs, a stage isn't sized to a fixed width: the
    /// glyph+label take only the room they need.
    private func stepperStage(
        _ tab: WorkflowTab,
        isCurrentTab: Bool,
        isReachable: Bool,
        isComplete: Bool
    ) -> some View {
        // Priority is complete, then current, then plain-reachable, then
        // locked — except the overlap the design calls out by name: a stage
        // that is both complete and current keeps the green check (complete's
        // glyph) under the current wash (current's background), rather than
        // one of the two states winning outright.
        let labelColor: InkRole
        let labelWeight: Font.Weight
        let showsCurrentWash: Bool

        if isCurrentTab {
            labelColor = .primary
            labelWeight = .semibold
            showsCurrentWash = true
        } else if isComplete {
            labelColor = .secondary
            labelWeight = .regular
            showsCurrentWash = false
        } else if isReachable {
            labelColor = .secondary
            labelWeight = .regular
            showsCurrentWash = false
        } else {
            labelColor = .quaternary
            labelWeight = .regular
            showsCurrentWash = false
        }

        let glyphColor: InkRole = isComplete
            ? .success
            : isCurrentTab ? .accent
            : isReachable ? .secondary
            : .quaternary

        return HStack(spacing: 5) {
            Group {
                if isComplete {
                    Image(systemName: "checkmark.circle.fill")
                        .font(.system(size: 13))
                } else {
                    Image(systemName: isCurrentTab ? "circle.fill" : "circle")
                        .font(.system(size: 8))
                }
            }
            .ink(glyphColor)

            Text(tab.rawValue)
                .font(.system(size: Typo.subhead, weight: labelWeight))
                .ink(labelColor)
                .lineLimit(1)
        }
        .padding(.horizontal, 9)
        .padding(.vertical, 4)
        .background(
            showsCurrentWash
                ? RoundedRectangle(cornerRadius: 6).fill(DesignTokens.wash(.selection, tone: .accent, on: ground))
                : nil
        )
        .hoverWash(cornerRadius: 6, enabled: isReachable)
        .contentShape(Rectangle())
        .onTapGesture {
            if isReachable {
                currentTab = tab
            }
        }
    }
}

#Preview {
    let appModel = AppModel()
    PaneView(appModel: appModel)
        .frame(height: 400)
}
