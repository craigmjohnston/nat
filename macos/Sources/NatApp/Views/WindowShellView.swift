import AppKit
import SwiftUI
import NatKit
import NatFixtures

struct WindowShellView: View {
    @Bindable var appModel: AppModel
    @State private var showNewProjectSheet = false

    /// The header row's height — and, through TrafficLightAlignerView, the
    /// band the traffic lights are centred in.
    private static let headerHeight: CGFloat = 34

    /// The rail's width, draggable at its divider and remembered across
    /// launches. The default is the `maxWidth` the rail was fixed at before
    /// it was resizable.
    @AppStorage("railWidth") private var railWidth = 372.0
    @State private var liveRailWidth: Double?

    var body: some View {
        ZStack {
            DesignTokens.fill(.window)
                .ignoresSafeArea()

            if appModel.needsOnboarding {
                OnboardingView(appModel: appModel, onNewProject: { showNewProjectSheet = true })
            } else {
                board
            }
        }
        // With the system title bar hidden, SwiftUI still reserves its height
        // as a top safe-area inset by default — without this, `board`'s own
        // header would be pushed down below where the traffic lights float,
        // leaving a bare strip of window above it instead of the header
        // being what sits there.
        .ignoresSafeArea(.container, edges: .top)
        // The cursor floor: without it, nothing in the window claims cursor
        // updates and the cursor stays whatever the window behind last set.
        .background(DefaultCursorView().ignoresSafeArea())
        // The traffic lights, recentred in the header band: macOS lays them
        // out for the standard title bar's height, which in a 40pt header
        // sits them high and tight to the left edge.
        .background(TrafficLightAlignerView(headerHeight: Self.headerHeight))
        // The add-a-project sheet: the welcome pane's button and the starter
        // card's From Notion tile both open it, presented from the window
        // rather than from whatever view asked.
        .sheet(isPresented: $showNewProjectSheet) {
            NewProjectSheetView(
                onClose: { showNewProjectSheet = false },
                onAdded: { id, name in
                    showNewProjectSheet = false
                    // From an Untitled tab the project takes that tab over;
                    // from the welcome pane there is none to take.
                    let untitled = appModel.activeTabIsUntitled ? appModel.activeProjectID : nil
                    Task { await appModel.addProject(id: id, name: name, replacing: untitled) }
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
                ProjectTabsView(appModel: appModel)
                    .padding(.leading, 78)
                .background(
                    // The mock's `color-mix(in srgb, accent 9%, header)` as
                    // one opaque colour rather than two stacked layers: the
                    // veil is mixed into the ground in `Palette.headerBg`,
                    // where it can be seen beside every other derived
                    // colour. Flat fill — the mock's blur is a backdrop
                    // material over what sits behind the window, not a blur
                    // of the band's own paint, and this window paints no
                    // material for anything to show through.
                    DesignTokens.fill(.header)
                    // The drag lives on the background rather than the row
                    // itself: SwiftUI still routes a tap to a Button or
                    // onTapGesture target on top of it (the project tabs, the
                    // toolbar buttons), and only the bare parts of the row
                    // fall through to this gesture — which is what makes the
                    // header behave like a title bar without swallowing its
                    // own controls' clicks. The double-click rides the same
                    // bare parts: a real title bar zooms on it, and hiding
                    // the system bar is not a reason to lose that.
                    .gesture(TapGesture(count: 2).onEnded {
                        TitlebarDoubleClick.perform(on: NSApp.keyWindow)
                    })
                    .gesture(WindowDragGesture())
                )
            }
            .frame(height: Self.headerHeight)

            // Main content: Rail | Pane
            HStack(spacing: 0) {
                RailView(appModel: appModel)
                    .frame(width: liveRailWidth ?? railWidth)

                Group {
                    // An Untitled tab has no project for a pane to be of.
                    if appModel.activeTabIsUntitled {
                        StarterView(onFromNotion: { showNewProjectSheet = true })
                    } else {
                        PaneView(appModel: appModel)
                    }
                }
                    .frame(maxWidth: .infinity)
                    // The rail's resize handle, straddling the divider the
                    // rail draws as its trailing border. It hangs off the
                    // pane rather than the rail so the sliver it covers is
                    // the pane's quiet left margin, not the tail of the
                    // rail's clickable rows.
                    .overlay(alignment: .leading) {
                        PaneResizeHandle(width: railWidth, liveWidth: $liveRailWidth, onCommit: { railWidth = $0 }, minWidth: 240, maxWidth: 560, edge: .trailing)
                            .offset(x: -4.5)
                    }
            }
            .frame(maxHeight: .infinity)

            // Status bar — full window width, split at the same x-position
            // as the rail/pane divider above it.
            StatusBarView(appModel: appModel, railWidth: liveRailWidth ?? railWidth)
        }
    }

}

#Preview {
    @Previewable @State var appModel = Fixtures.appModel()
    WindowShellView(appModel: appModel)
        .frame(width: 1360, height: 840)
        .task { await Fixtures.start(appModel) }
}
