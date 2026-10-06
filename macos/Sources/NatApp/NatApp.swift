import AppKit
import SwiftUI
import NatKit

/// Started by `main.swift` rather than by `@main`, which the gallery's
/// argument parsing has to run ahead of — see the comment there.
struct NatApp: App {
    @State private var appModel: AppModel
    @StateObject private var updaterViewModel = UpdaterViewModel()
    /// Only for the dock menu (`DockAttention`).
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate

    /// The chosen theme, as the settings window writes it. Reading it here
    /// is what makes the switch live: the scene re-renders when the stored
    /// value changes, `NSApp.appearance` moves with it, and every token
    /// in `DesignTokens` re-resolves under the new appearance.
    @AppStorage(Theme.storageKey) private var storedTheme = Theme.system.rawValue

    /// Which palette each of the two schemes draws with, as the settings
    /// window writes them.
    @AppStorage(PaletteChoice.darkStorageKey) private var storedDarkPalette = PaletteChoice.defaultDark.rawValue
    @AppStorage(PaletteChoice.lightStorageKey) private var storedLightPalette = PaletteChoice.defaultLight.rawValue

    /// The UI and code text sizes, as the settings window writes them.
    @AppStorage(TypeSize.uiStorageKey) private var storedUISize = TypeSize.defaultUI
    @AppStorage(TypeSize.monoStorageKey) private var storedMonoSize = TypeSize.defaultMono

    /// View ▸ Show/Hide Done Items: whether the sidebar draws finished work
    /// — done slices, ended sessions and each project's Done folder.
    @AppStorage(showsDoneItemsKey) private var showsDoneItems = true

    /// View ▸ Wrap lines in diffs: whether a long line of code breaks at the
    /// pane's edge or runs on, the diff scrolling sideways to it.
    @AppStorage(diffWrapsLinesKey) private var diffWrapsLines = true

    private var theme: Theme { Theme(stored: storedTheme) }

    /// The two stored palettes, put in their slots, as one identity for the
    /// window's content.
    ///
    /// Selecting here — while the body that is about to draw with them is
    /// being computed — is what guarantees the new subtree's first frame
    /// already resolves against them; an `onChange` runs after that frame.
    /// It is idempotent, so a body re-run for any other reason re-selects
    /// what is already selected.
    ///
    /// The identity is what makes the switch take: SwiftUI keeps the colours
    /// it has resolved for an appearance, and a palette change is not an
    /// appearance change, so nothing under the old tree re-asks. Flipping
    /// the appearance and back repaints only part of the window; rebuilding
    /// the content repaints all of it, at the cost of the views' own state
    /// (folds, scroll offsets) — which a palette pick, made rarely and from
    /// Settings, can afford. The app's model lives above this and survives.
    ///
    /// The two text sizes join it for the same reason: every size on the
    /// `Typo` ramp is read as a plain number when a view is built, and the
    /// diff and the terminal measure their geometry from theirs once, so
    /// only a rebuild draws everything at a new size.
    private var paletteIdentity: String {
        let dark = PaletteChoice(stored: storedDarkPalette, dark: true)
        let light = PaletteChoice(stored: storedLightPalette, dark: false)
        PaletteSelection.shared.select(dark)
        PaletteSelection.shared.select(light)
        let size = TypeSize(ui: storedUISize, mono: storedMonoSize)
        TypeSizeSelection.shared.select(size)
        return "\(dark.rawValue)/\(light.rawValue)/\(size.identity)"
    }

    init() {
        // The very first thing the process does: compose the real PATH —
        // the bundled nat's directory, the login shell's entries, whatever
        // launchd gave us — and set it, before anything reads the
        // environment or spawns a child. A Finder launch has no Homebrew
        // and no nat on its PATH without this; see PathBootstrap. It runs
        // ahead of the AppModel below, which is why the property has no
        // default of its own — a default would be initialised first.
        PathBootstrap.bootstrap()
        // The bundled Fira Code, handed to CoreText before any window
        // draws — the terminal, the diff, markdown code and every input
        // resolve the face by name, and a face registered after the first
        // frame is a frame drawn in the fallback. It is idempotent and
        // `Typo.mono` calls it too, so this is only about when it happens.
        MonoFont.register()
        let model = AppModel(
            readsGitHubOnATick: true,
            mirrorNudgeMemory: MirrorNudgeMemory(), seenMemory: SeenMemory(),
            closedTabMemory: ClosedTabMemory(), workshopCache: DiskWorkshopCache(), makesSourceProjects: true,
            assignsProjectColors: true)
        _appModel = State(initialValue: model)
        // A workshop brief is written once typing pauses; quitting (an
        // update's relaunch included) writes whatever the pause had not yet.
        NotificationCenter.default.addObserver(
            forName: NSApplication.willTerminateNotification, object: nil, queue: .main
        ) { _ in
            MainActor.assumeIsolated { model.flushWorkshops() }
        }
        // A bare executable launched from a terminal (swift run, or
        // .build/debug/gnat directly) has no bundle, and AppKit leaves such
        // a process at the `.prohibited` activation policy: its window draws,
        // but the app can never become active, so the window never becomes
        // key — clicks fail to land, and the cursor over the window stays
        // whichever app is actually active. Saying `.regular` here is what a
        // bundled app's Info.plist would have said for it. The matching
        // activate happens when the window first appears — this early, there
        // is no window yet and the request is ignored.
        NSApplication.shared.setActivationPolicy(.regular)
        // One window, never tabbed: no tab bar, and no View ▸ Show Tab Bar or
        // Show All Tabs for AppKit to add on the window's behalf.
        NSWindow.allowsAutomaticWindowTabbing = false
        // Scroll bars always shown wherever there is something to scroll,
        // whatever the Mac's own Appearance setting: the app's own domain
        // outranks the global one AppKit otherwise reads the style from.
        UserDefaults.standard.set("Always", forKey: "AppleShowScrollBars")
        Self.setDockIcon()
        // The dock's badge, menu and bounce follow the model from here on.
        DockAttention.shared.start(model)
    }

    /// The gnat on the dock for a bare executable, which has no Info.plist
    /// for AppKit to read an icon from — the bundled app names AppIcon.icns
    /// there and needs none of this. The icns is found by hand rather than
    /// through `Bundle.module`, whose generated accessor traps when the
    /// resource bundle is missing, and missing is not an error here: an app
    /// with no icon set still runs, it just keeps the generic one.
    ///
    /// It also follows the appearance: the paper icon while the app is light,
    /// the dark-navy one while it is dark — for the bundled app too, whose
    /// plist icon (the light one) is only what Finder shows.
    ///
    /// Except on macOS 26, where the bundled app's layered icon
    /// (`CFBundleIconName`) is the system's to draw, light or dark, whether
    /// the app is running or not: an image set here would replace it for as
    /// long as the app ran, so nothing is set.
    private static func setDockIcon() {
        if #available(macOS 26, *), Bundle.main.object(forInfoDictionaryKey: "CFBundleIconName") != nil {
            return
        }
        applyDockIcon()
        appearanceObservation = NSApplication.shared.observe(\.effectiveAppearance) { _, _ in
            DispatchQueue.main.async { applyDockIcon() }
        }
    }

    /// Held for the life of the app, so the dock icon keeps following.
    nonisolated(unsafe) private static var appearanceObservation: NSKeyValueObservation?

    private static func applyDockIcon() {
        let dark = NSApplication.shared.effectiveAppearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua
        guard let image = iconImage(dark: dark) else { return }
        NSApplication.shared.applicationIconImage = image
    }

    /// The paper icon, or the dark-navy one, wherever this build keeps it —
    /// nil where it keeps neither. Settings ▸ About draws it too.
    static func iconImage(dark: Bool) -> NSImage? {
        let file = dark ? "AppIconDark.icns" : "AppIcon.icns"
        let candidates = [
            // Beside the bare executable, where SwiftPM builds it.
            Bundle.main.bundleURL.appendingPathComponent("nat_NatApp.bundle/\(file)"),
            // A bundled app's own Resources.
            Bundle.main.resourceURL?.appendingPathComponent(file),
        ].compactMap { $0 }
        guard let url = candidates.first(where: { FileManager.default.fileExists(atPath: $0.path) }) else { return nil }
        return NSImage(contentsOf: url)
    }

    var body: some Scene {
        // The mock's canvas is 1360×840 and every metric in it was chosen at
        // that size — opening there is what makes the proportions read as
        // designed. A single `Window` rather than a group: gnat is one window
        // over every project, so there is no New Window to offer.
        Window("gnat", id: "main") {
            // NAT_TERM_SESSION is a debug affordance only: it lets a session
            // name be smoke-tested against a real tmux session before the
            // Agent tab has anywhere of its own to launch one from. Anyone
            // launching NatApp normally sees the full shell view.
            Group {
                if let session = ProcessInfo.processInfo.environment["NAT_TERM_SESSION"] {
                    AgentTerminalDebugView(session: session)
                } else {
                    WindowShellView(appModel: appModel)
                        .environment(\.showsDoneItems, showsDoneItems)
                        .task { await Self.snapshotIfAsked(appModel) }
                }
            }
            .id(paletteIdentity)
            // Diagnostics only: a no-op unless NAT_CURSOR_DEBUG=1 is set, for
            // tracking down the persistent I-beam cursor bug live. `.task`
            // is a View modifier and so goes on the window's content, not on
            // the Window scene below.
            .task { CursorDebugWalker.startIfAsked() }
            // Likewise a no-op unless NAT_KEY_DEBUG=1 and NAT_KEY_DEBUG_SYNTH=1.
            .onAppear { KeyDebug.synthesizeIfAsked() }
            // Likewise a no-op unless NAT_MENU_DEBUG=1 or the NatMenuDebug
            // default is set; installs once however often this reappears.
            .onAppear { MenuDebug.startIfAsked() }
            // The other half of init's `.regular` policy: brings the window
            // to the front the way launching a bundled app would, now that
            // there is a window to bring.
            .onAppear { NSApplication.shared.activate() }
            // Both palettes are carried by the tokens themselves, so all
            // this does is say which one the app asks for, at AppKit level
            // (`preferredColorScheme(nil)` never un-pins) — nil for `system`,
            // which follows the Mac and goes on following it. Runs once at
            // startup and on every change, for every window at once.
            .onChange(of: storedTheme, initial: true) {
                NSApp.appearance = theme.nsAppearanceName.flatMap(NSAppearance.init(named:))
            }
        }
        // The header row IS the title bar (WindowShellView reserves room for
        // the traffic lights and makes itself draggable) — hiding the system
        // one is what lets the project tabs sit where the mock puts them,
        // flush with the top of the window rather than below a bar of their
        // own.
        .windowStyle(.hiddenTitleBar)
        .defaultSize(width: 1360, height: 840)
        .commands {
            CommandGroup(after: .appInfo) {
                CheckForUpdatesView(model: updaterViewModel)
            }
            GnatCommands(showsDoneItems: $showsDoneItems, diffWrapsLines: $diffWrapsLines)
        }

        // The settings window takes the app's appearance (`NSApp.appearance`,
        // set above) like every other window, so it cannot disagree with main.
        Settings {
            SettingsView(appModel: appModel, updater: updaterViewModel)
        }
    }

    /// NAT_SNAPSHOT is the headless eye on the *live* window: with it set to
    /// a file path, the app waits for the first loads to land, renders the
    /// shell offscreen at the mock's canvas size, writes the PNG there and
    /// exits. It exists because screencapture needs a permission a build
    /// agent does not have, and a screen nobody can look at is a screen
    /// nobody checks.
    ///
    /// It draws whatever the app is actually showing and nothing else: the
    /// selector that used to drive it to a slice or to the workshop is gone,
    /// because every state it could reach is a story in `AppStories` now, and
    /// a story says which state it is in its own name rather than in an
    /// environment variable read three seconds after launch. What is left
    /// here is the one thing a story cannot be — the real app, over a real
    /// project, as it stands.
    @MainActor
    private static func snapshotIfAsked(_ appModel: AppModel) async {
        guard let path = ProcessInfo.processInfo.environment["NAT_SNAPSHOT"] else { return }
        for _ in 0..<30 {
            try? await Task.sleep(nanoseconds: 1_000_000_000)
            guard let state = appModel.projectStore?.state else { continue }
            if state.projectInfo != nil || state.errorMessage != nil { break }
        }
        NSLog("nat snapshot: store state = %@",
              String(describing: appModel.projectStore?.state).prefix(300) as CVarArg)
        try? await Task.sleep(nanoseconds: 1_000_000_000)
        // The window's own drawn pixels, not an offscreen ImageRenderer pass:
        // the renderer skips scrollable containers' content, and a snapshot
        // that lies about the window is worse than none.
        if let window = NSApp.windows.first(where: { $0.isVisible }),
           let view = window.contentView {
            window.setContentSize(NSSize(width: 1360, height: 840))
            window.layoutIfNeeded()
            try? await Task.sleep(nanoseconds: 1_000_000_000)
            if let rep = view.bitmapImageRepForCachingDisplay(in: view.bounds) {
                view.cacheDisplay(in: view.bounds, to: rep)
                if let png = rep.representation(using: .png, properties: [:]) {
                    try? png.write(to: URL(fileURLWithPath: path))
                }
            }
        }
        exit(0)
    }
}

/// The debug host for NAT_TERM_SESSION: attaches the terminal view straight
/// to the named session, with a real (if minimal) way of telling a detach
/// from the session having gone — `tmux has-session` run after the attach
/// process ends. That check is plain command-running with no branching
/// logic of its own worth testing in NatKit, unlike the state it feeds.
struct AgentTerminalDebugView: View {
    let session: String

    var body: some View {
        AgentTerminalHostView(
            attachSpec: AttachSpec(session: session),
            sessionExists: { Self.tmuxHasSession(session) },
            onExit: { reason in
                NSLog("nat: terminal for session \(session) exited: \(reason)")
            }
        )
        .ignoresSafeArea()
    }

    private static func tmuxHasSession(_ session: String) -> Bool {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/env")
        process.arguments = [AttachSpec.executable, "has-session", "-t", session]
        do {
            try process.run()
            process.waitUntilExit()
            return process.terminationStatus == 0
        } catch {
            return false
        }
    }
}

struct ContentView: View {
    var body: some View {
        ZStack {
            DesignTokens.fill(.window)
                .ignoresSafeArea()

            VStack(spacing: 16) {
                Text("nat")
                    .font(.system(size: Typo.scaled(32), weight: .semibold))
                    .ink(.primary)

                Text("board loading will land here")
                    .font(.system(size: Typo.body, weight: .regular))
                    .ink(.secondary)
            }
        }
    }
}
