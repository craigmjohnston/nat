import AppKit
import SwiftTerm
import SwiftUI
import NatKit

/// What a story draws to show the agent terminal's type: a real SwiftTerm
/// view, styled exactly as `AgentTerminalHostView` styles the live one, fed
/// a canned Claude Code screen rather than attached to a tmux session. The
/// stub views stand in for the terminal everywhere else in the gallery; this
/// is the one story where the terminal's own rasterisation — its font, its
/// smoothing, its cell grid — is the thing under review.
///
/// The capture leaves the terminal's ground transparent: the live view's
/// ground is its layer's `backgroundColor`, which a `cacheDisplay` pass
/// does not paint, and SwiftTerm's `draw` is not open to add it. The PNG
/// is read over the palette's `terminalBg`, which is what the window shows.
struct TerminalTranscriptStoryView: NSViewRepresentable {
    @Environment(\.colorScheme) private var colorScheme

    func makeNSView(context: Context) -> TerminalView {
        let view = TerminalView(frame: .zero)
        TerminalTheme.apply(DesignTokens.palette(for: colorScheme), to: view)
        view.feed(text: Self.transcript.joined(separator: "\r\n"))
        return view
    }

    func updateNSView(_ nsView: TerminalView, context: Context) {
        TerminalTheme.apply(DesignTokens.palette(for: colorScheme), to: nsView)
    }

    /// A turn of a Claude Code session on a slice, as the pane would show it:
    /// the banner, the brief, a read, an edit with its hunk, a test run, the
    /// hand-back line and the prompt. Fixed rather than live so a render is
    /// the same twice; the escape codes are the ones Claude Code itself uses.
    static let transcript: [String] = {
        let b = "\u{1B}[1m", d = "\u{1B}[2m", r = "\u{1B}[0m"
        let green = "\u{1B}[32m", red = "\u{1B}[31m", blue = "\u{1B}[34m", yellow = "\u{1B}[33m"
        return [
            "╭──────────────────────────────────────────────────────────────────╮",
            "│ \(yellow)✻\(r) Welcome to \(b)Claude Code\(r)!                                          │",
            "│   /help for help, /status for your current setup                 │",
            "│   cwd: ~/Projects/nat-review-flow                                │",
            "╰──────────────────────────────────────────────────────────────────╯",
            "",
            "\(d)> Wire the diff pane to the store. Each file's hunks come off",
            "  ReviewStore now; the pane must stop re-reading the branch itself.\(r)",
            "",
            "\(green)●\(r) \(b)Read\(r)(macos/Sources/NatKit/Review/ReviewStore.swift)",
            "  ⎿  Read 212 lines",
            "",
            "\(green)●\(r) \(b)Update\(r)(macos/Sources/NatApp/Views/DiffPaneView.swift)",
            "  ⎿  Updated \(b)DiffPaneView.swift\(r) with 14 additions and 3 removals",
            "       \(d)41\(r)    struct DiffPaneView: View {",
            "       \(d)42\(r) \(red)-      let files: [DiffFile]\(r)",
            "       \(d)42\(r) \(green)+      @Environment(ReviewStore.self) private var store\(r)",
            "       \(d)43\(r) \(green)+      var files: [DiffFile] { store.files }\(r)",
            "       \(d)44\(r)        let onComment: (DiffLineID) -> Void",
            "       \(d)45\(r) ",
            "       \(d)46\(r) \(red)-      var body: some View { ForEach(files) { file in\(r)",
            "       \(d)46\(r) \(green)+      var body: some View { ForEach(files, id: \\.path) { file in\(r)",
            "       \(d)…\(r)",
            "",
            "\(green)●\(r) \(b)Bash\(r)(swift test --package-path macos --filter DiffPane)",
            "  ⎿  Test Suite 'DiffPaneTests' passed at 2026-10-04 15:02:11.418.",
            "     \t Executed 12 tests, with 0 failures (0 unexpected) in 0.418 (0.421) seconds",
            "",
            "\(green)●\(r) Done. The pane reads \(blue)store.files\(r) now and never touches git itself;",
            "  \(blue)nat slice-show --json\(r) stays the one source of what is handed back.",
            "",
            "\(green)●\(r) \(b)Bash\(r)(nat complete-slice --project 2a1f…c9e4 --branch slice/wire-the-diff-pane)",
            "  ⎿  Handed back: Wire the diff pane to the store (M2: Review flow)",
            "",
            "╭──────────────────────────────────────────────────────────────────╮",
            "│ \(d)>\(r)                                                                │",
            "╰──────────────────────────────────────────────────────────────────╯",
            "  \(d)? for shortcuts                                    ⏵⏵ accept edits on\(r)",
        ]
    }()
}
