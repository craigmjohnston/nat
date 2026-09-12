import AppKit
import SwiftUI
import NatKit

/// The headless half of the gallery: mounts a story in an offscreen window,
/// captures the pixels the window actually drew, and writes them out.
///
/// It captures through `bitmapImageRepForCachingDisplay`/`cacheDisplay` — the
/// window's own drawn pixels — rather than through `ImageRenderer`, which
/// renders the view tree afresh and skips scrollable containers' content.
/// Half the states worth drawing here are inside a scroll view, so a
/// renderer pass would hand back a gallery of empty panes. It is the same
/// path `NAT_SNAPSHOT` in `NatApp.swift` already trusts, with the window made
/// for the capture rather than borrowed from the running app.
///
/// The AppKit lives here, in the executable, and not in `NatKit`: a window is
/// not business logic, and `NatKit` has tests that run without a
/// WindowServer.
@MainActor
enum GalleryRunner {
    /// How long a story is given to settle before it is captured — its
    /// stores' own loads land in `.task`s the runner cannot await, and a
    /// pane captured before them is a pane of skeletons. Long enough for a
    /// fixture load, which touches nothing outside the process, and short
    /// enough that a named story is a couple of seconds' wait.
    static let settle = Duration.milliseconds(900)

    /// Runs the command and exits. It never returns: every ending here is
    /// the process's, since a gallery run is the whole of what the process
    /// was started for.
    static func run(_ command: GalleryCommand, catalog: StoryCatalog = AppStories.catalog) -> Never {
        if case .list = command {
            for name in catalog.names { print(name) }
            exit(0)
        }

        // The fonts, before any window draws: everything monospaced in the
        // app resolves JetBrains Mono by name, and a face registered after
        // the first frame is a frame drawn in the fallback — which in a PNG
        // nobody re-renders is a wrong reference for good.
        MonoFont.register()

        let app = NSApplication.shared
        // No dock icon and no menu bar: the run is headless, and a gallery
        // that steals the user's focus for two seconds is a gallery nobody
        // runs while working.
        app.setActivationPolicy(.prohibited)
        let delegate = GalleryDelegate(command: command, catalog: catalog)
        app.delegate = delegate
        // AppKit needs its run loop for a window to lay out and draw at all;
        // the delegate does the work and exits, so this call does not return.
        app.run()
        exit(0)
    }

    /// Draws one story and hands back its PNG.
    static func png(of story: Story) async throws -> Data {
        let content = await story.content()
        let frame = NSRect(origin: .zero, size: story.size)

        let window = NSWindow(
            contentRect: frame, styleMask: [.borderless], backing: .buffered, defer: false)
        // Pinned rather than followed: `Theme.system` would make the PNG a
        // fact about the machine that rendered it. Both halves are said —
        // the window's appearance is what a dynamic `NSColor` resolves
        // against, and `preferredColorScheme` is what SwiftUI's own
        // environment reads.
        window.appearance = NSAppearance(named: story.colorScheme == .dark ? .darkAqua : .aqua)
        window.contentView = NSHostingView(
            rootView: AnyView(content.preferredColorScheme(story.colorScheme)))
        window.setFrame(frame, display: false)
        // Far off any screen: the window has to be ordered in for SwiftUI to
        // lay it out and draw it, and a window that flashes up in front of
        // whatever the user is doing is not headless.
        window.setFrameOrigin(NSPoint(x: -50_000, y: -50_000))
        window.orderFrontRegardless()
        window.layoutIfNeeded()
        try await Task.sleep(for: settle)
        window.layoutIfNeeded()

        defer { window.orderOut(nil) }
        guard let view = window.contentView,
              let rep = view.bitmapImageRepForCachingDisplay(in: view.bounds) else {
            throw GalleryRenderError.noBitmap(story: story.name)
        }
        view.cacheDisplay(in: view.bounds, to: rep)
        guard let png = rep.representation(using: .png, properties: [:]) else {
            throw GalleryRenderError.noPNG(story: story.name)
        }
        return png
    }
}

/// Why a story did not become a file. Each one names the story, because a
/// sweep stops at the first and the name is where to go and look.
enum GalleryRenderError: Error, CustomStringConvertible {
    case unknownStory(name: String, known: [String])
    case noBitmap(story: String)
    case noPNG(story: String)

    var description: String {
        switch self {
        case .unknownStory(let name, let known):
            return "no story called \(name); the catalog holds: \(known.joined(separator: ", "))"
        case .noBitmap(let story):
            return "\(story): the window would not give up a bitmap"
        case .noPNG(let story):
            return "\(story): the bitmap would not encode as PNG"
        }
    }
}

/// The app delegate a gallery run has instead of a scene: AppKit finishes
/// launching, the work happens, the process exits.
@MainActor
final class GalleryDelegate: NSObject, NSApplicationDelegate {
    private let command: GalleryCommand
    private let catalog: StoryCatalog

    init(command: GalleryCommand, catalog: StoryCatalog) {
        self.command = command
        self.catalog = catalog
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        Task { @MainActor in
            do {
                try await render()
                exit(0)
            } catch {
                FileHandle.standardError.write(Data("gnat gallery: \(error)\n".utf8))
                exit(1)
            }
        }
    }

    private func render() async throws {
        switch command {
        case .list:
            // Answered before the app was ever started; kept exhaustive
            // rather than defaulted so a fourth command has to be decided
            // here too.
            break
        case .one(let name, let out):
            guard let story = catalog.story(named: name) else {
                throw GalleryRenderError.unknownStory(name: name, known: catalog.names)
            }
            try await write(story, to: URL(fileURLWithPath: out))
            print(out)
        case .all(let directory):
            let dir = URL(fileURLWithPath: directory, isDirectory: true)
            try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
            for story in catalog.stories {
                let url = dir.appendingPathComponent(story.fileName)
                try await write(story, to: url)
                print(url.path)
            }
        }
    }

    private func write(_ story: Story, to url: URL) async throws {
        let png = try await GalleryRunner.png(of: story)
        try png.write(to: url)
    }
}
