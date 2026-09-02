import SwiftUI
import NatKit

struct WindowShellView: View {
    @Bindable var appModel: AppModel
    @State private var showWorkshopOverlay = false
    @State private var showNewSliceSheet = false

    var body: some View {
        ZStack {
            DesignTokens.windowBg
                .ignoresSafeArea()

            if appModel.needsOnboarding {
                OnboardingView(appModel: appModel)
            } else {
                board
            }

            if showWorkshopOverlay, let projectID = appModel.activeProjectID {
                WorkshopOverlayView(
                    projectID: projectID,
                    projectName: activeProjectName,
                    model: appModel.config?.workshopAgent?.model,
                    effort: appModel.config?.workshopAgent?.effort,
                    onClose: { showWorkshopOverlay = false }
                )
            }
        }
        .sheet(isPresented: $showNewSliceSheet) {
            NewSliceSheetView(
                projectID: appModel.activeProjectID ?? "",
                milestones: appModel.projectStore?.state.projectInfo?.milestones ?? [],
                onClose: { showNewSliceSheet = false },
                onCreated: {
                    showNewSliceSheet = false
                    Task { await appModel.refresh() }
                }
            )
        }
        .task {
            await appModel.start()
        }
    }

    private var board: some View {
        VStack(spacing: 0) {
            // Header
            VStack(spacing: 0) {
                HStack(spacing: 0) {
                    // Traffic lights - left-aligned (handled by system)
                    Spacer()
                        .frame(maxWidth: .infinity, alignment: .leading)

                    // Project tabs
                    ProjectTabsView(appModel: appModel)

                    // Right-side toolbar
                    HStack(spacing: 12) {
                        // Slice count
                        if let projectInfo = appModel.projectStore?.state.projectInfo {
                            Text("\(projectInfo.slices.filter { $0.status != "Done" }.count)/\(projectInfo.slices.count) slices")
                                .font(.system(size: 11, weight: .regular, design: .monospaced))
                                .foregroundStyle(DesignTokens.labelTertiary)
                        }

                        Button(action: { showNewSliceSheet = true }) {
                            Image(systemName: "plus.rectangle.on.rectangle")
                                .font(.system(size: 12, weight: .regular))
                                .foregroundStyle(DesignTokens.labelTertiary)
                        }
                        .buttonStyle(.plain)
                        .disabled(appModel.projectStore == nil)
                        .opacity(appModel.projectStore == nil ? 0.5 : 1)
                        .help("New Slice…")

                        Button(action: { showWorkshopOverlay = true }) {
                            Image(systemName: "wand.and.stars")
                                .font(.system(size: 12, weight: .regular))
                                .foregroundStyle(DesignTokens.labelTertiary)
                        }
                        .buttonStyle(.plain)
                        .disabled(appModel.projectStore == nil)
                        .opacity(appModel.projectStore == nil ? 0.5 : 1)
                        .help("Workshop the Plan")
                    }
                    .padding(.horizontal, 16)
                }
                .background(
                    Color(hex: "1e1e23").opacity(0.85)
                        .blur(radius: 8)
                )
            }
            .frame(height: 40)

            // Main content: Rail | Pane
            HStack(spacing: 0) {
                RailView(appModel: appModel)
                    .frame(maxWidth: 372)

                PaneView(appModel: appModel)
                    .frame(maxWidth: .infinity)
            }
            .frame(maxHeight: .infinity)

            // Progress border
            ProgressBorderView(appModel: appModel)
                .frame(height: 7)
        }
    }

    private var activeProjectName: String {
        guard let activeID = appModel.activeProjectID else { return "" }
        return appModel.projectTabs.first(where: { $0.id == activeID })?.name ?? activeID
    }
}

#Preview {
    WindowShellView(appModel: AppModel())
        .frame(width: 1360, height: 840)
}
