import SwiftUI
import NatKit

struct PaneView: View {
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

    var body: some View {
        VStack(spacing: 0) {
            if let slice = selectedSlice, let tabState = workflowState {
                // Header with slice name and tab strip
                VStack(spacing: 0) {
                    HStack(spacing: 14) {
                        Text(slice.name)
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundStyle(DesignTokens.label)
                            .lineLimit(1)

                        Spacer()

                        // Tab strip
                        HStack(spacing: 10) {
                            ForEach(Array(tabState.tabs.enumerated()), id: \.offset) { index, tab in
                                if index > 0 {
                                    Image(systemName: "arrow.right")
                                        .font(.system(size: 12, weight: .regular))
                                        .foregroundStyle(DesignTokens.labelQuaternary)
                                }

                                tabButton(
                                    tab,
                                    isCurrentTab: currentTab == tab,
                                    isReachable: tabState.isReachable(tab),
                                    isPastTab: tabState.tabs.firstIndex(of: tab)! < tabState.tabs.firstIndex(of: currentTab)!
                                )
                            }
                        }
                    }
                    .frame(height: 46)
                    .padding(.horizontal, 14)

                    Divider()
                        .frame(height: 0.5)
                        .foregroundStyle(DesignTokens.separator)
                }

                // Content area
                switch currentTab {
                case .brief:
                    BriefTabView(appModel: appModel, slice: slice)
                case .agent:
                    AgentTabView(appModel: appModel, slice: slice)
                default:
                    VStack {
                        VStack(spacing: 8) {
                            Image(systemName: "doc.text")
                                .font(.system(size: 32, weight: .regular))
                                .foregroundStyle(DesignTokens.labelSecondary)

                            Text("\(currentTab.rawValue) lands here")
                                .font(.system(size: 13, weight: .regular))
                                .foregroundStyle(DesignTokens.labelSecondary)
                        }
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                        .background(DesignTokens.controlBg)
                    }
                }
            } else {
                // Empty state
                VStack {
                    VStack(spacing: 8) {
                        Image(systemName: "doc.text")
                            .font(.system(size: 32, weight: .regular))
                            .foregroundStyle(DesignTokens.labelSecondary)

                        Text("Select a slice to begin")
                            .font(.system(size: 13, weight: .regular))
                            .foregroundStyle(DesignTokens.labelSecondary)
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                }
                .background(DesignTokens.controlBg)
            }
        }
        .onChange(of: appModel.selectedSliceID) { _, _ in
            // Reset tab when slice changes
            currentTab = workflowState?.defaultTab ?? .brief
        }
    }

    // MARK: - Tab Button

    private func tabButton(
        _ tab: WorkflowTab,
        isCurrentTab: Bool,
        isReachable: Bool,
        isPastTab: Bool
    ) -> some View {
        VStack(spacing: 0) {
            HStack(spacing: 5) {
                Image(systemName: tab.symbolName)
                    .font(.system(size: 12, weight: .regular))
                    .foregroundStyle(isCurrentTab ? DesignTokens.accent : DesignTokens.labelSecondary)

                Text(tab.rawValue)
                    .font(.system(size: 12, weight: isCurrentTab ? .semibold : .regular))
                    .foregroundStyle(isCurrentTab ? DesignTokens.label : DesignTokens.labelSecondary)

                if isPastTab {
                    Image(systemName: "checkmark")
                        .font(.system(size: 9, weight: .bold))
                        .foregroundStyle(DesignTokens.systemGreen)
                }
            }
            .padding(.horizontal, 4)
            .padding(.vertical, 4)
            .opacity(isReachable ? 1 : 0.4)
            .contentShape(Rectangle())
            .onTapGesture {
                if isReachable {
                    withAnimation {
                        currentTab = tab
                    }
                }
            }

            if isCurrentTab {
                DesignTokens.accent
                    .frame(height: 2)
                    .cornerRadius(1)
            }
        }
    }
}

#Preview {
    let appModel = AppModel()
    PaneView(appModel: appModel)
        .frame(height: 400)
}
