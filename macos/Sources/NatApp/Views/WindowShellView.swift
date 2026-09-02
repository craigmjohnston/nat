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
        // With the system title bar hidden, SwiftUI still reserves its height
        // as a top safe-area inset by default — without this, `board`'s own
        // header would be pushed down below where the traffic lights float,
        // leaving a bare strip of window above it instead of the header
        // being what sits there.
        .ignoresSafeArea(.container, edges: .top)
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
            // Header — with the system title bar hidden (`.windowStyle(.hiddenTitleBar)`
            // in NatApp.swift), this row IS the title bar: the leading padding
            // is where macOS draws the traffic lights over it, and the whole
            // row is window-draggable the way a title bar always was.
            VStack(spacing: 0) {
                HStack(spacing: 0) {
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
                .padding(.leading, 78)
                .background(
                    ZStack {
                        Color(hex: "1e1e23").opacity(0.85)
                        // color-mix(in srgb, var(--accent) 9%, var(--material-header-bg))
                        // approximated: the accent laid over the header's own
                        // material at 9% opacity. Flat fill — the mock's blur
                        // is a backdrop material over what sits behind the
                        // window, not a blur of the band's own paint.
                        DesignTokens.accent.opacity(0.09)
                    }
                    // The drag lives on the background rather than the row
                    // itself: SwiftUI still routes a tap to a Button or
                    // onTapGesture target on top of it (the project tabs, the
                    // toolbar buttons), and only the bare parts of the row
                    // fall through to this gesture — which is what makes the
                    // header behave like a title bar without swallowing its
                    // own controls' clicks.
                    .gesture(WindowDragGesture())
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
