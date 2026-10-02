import AppKit
import SwiftUI
import NatKit

/// The gnat design's own geometry, in one place, so the three columns and
/// the status bar are built on the same numbers the mock was drawn at.
enum GnatMetrics {
    /// Every titlebar — the sidebar's, the navigator's, the main pane's.
    static let titlebarHeight: CGFloat = 32
    /// A file row, a note.
    static let rowHeight: CGFloat = 24
    /// A sidebar row — a little taller than the design's 24, to breathe.
    static let sidebarRowHeight: CGFloat = 26
    /// A navigator section's header.
    static let sectionHeadHeight: CGFloat = 32
    static let statusBarHeight: CGFloat = 28
    static let sidebarWidth: Double = 260
    static let navigatorWidth: Double = 330
    /// The design's mono `xs` and its body sizes.
    static let xs: CGFloat = 12
    static let body: CGFloat = 14
    /// The window titlebar's text — a step under the body.
    static let titlebarText: CGFloat = 13
    /// Where a traffic-light window's titlebar content starts: past the lights.
    static let lightsInset: CGFloat = 72
}

// MARK: - Marks

/// The disclosure chevron: a small stroked caret pointing right, turned down
/// while open, in the design's `--ink-3`.
struct DisclosureChevron: View {
    let open: Bool

    var body: some View {
        ChevronShape()
            .stroke(style: StrokeStyle(lineWidth: 1.6, lineCap: .round, lineJoin: .round))
            .frame(width: 10, height: 10)
            .rotationEffect(.degrees(open ? 90 : 0))
            .animation(Motion.stateChange, value: open)
            .ink(.tertiary)
            .frame(width: 12)
    }
}

private struct ChevronShape: Shape {
    func path(in rect: CGRect) -> Path {
        var path = Path()
        let scale = rect.width / 10
        path.move(to: CGPoint(x: 3.5 * scale, y: 2 * scale))
        path.addLine(to: CGPoint(x: 6.5 * scale, y: 5 * scale))
        path.addLine(to: CGPoint(x: 3.5 * scale, y: 8 * scale))
        return path
    }
}

/// A slice's dot, by its display state: hollow for todo, a slashed ring for
/// blocked, a muted checkmark for done, the accent for work under way (pulsing while an
/// agent is live on it), `--hot` for whatever needs the user.
struct StateDot: View {
    @Environment(\.ground) private var ground
    @Environment(\.pulsesPaused) private var pulsesPaused
    let state: SliceDisplayState
    var live: Bool = false
    var size: CGFloat = 7

    /// Dropped half a point below the row's centre, onto the middle of the
    /// title's capitals — the line the Active rows' project tags sit on too.
    var body: some View {
        mark.offset(y: 0.5)
    }

    @ViewBuilder
    private var mark: some View {
        switch state {
        case .todo:
            Circle().strokeBorder(DesignTokens.ink(.tertiary, on: ground), lineWidth: 1.5)
                .frame(width: size, height: size)
        case .blocked:
            // A todo ring with a slash through it: the same circle as the
            // rest, marked as one that cannot start.
            let ink = DesignTokens.ink(.tertiary, on: ground)
            ZStack {
                Circle().strokeBorder(ink, lineWidth: 1.5)
                Path { path in
                    path.move(to: CGPoint(x: size * 0.23, y: size * 0.23))
                    path.addLine(to: CGPoint(x: size * 0.77, y: size * 0.77))
                }
                .stroke(ink, style: StrokeStyle(lineWidth: 1.3, lineCap: .round))
            }
            .frame(width: size, height: size)
        case .done:
            Image(systemName: "checkmark")
                .font(.system(size: size + 2, weight: .bold))
                .foregroundStyle(DesignTokens.ink(.tertiary, on: ground))
                .frame(width: size, height: size)
        case .working, .fixing:
            let dot = Circle().fill(DesignTokens.accent).frame(width: size, height: size)
            if live && !pulsesPaused {
                dot.modifier(PulseModifier())
            } else {
                dot
            }
        case .waiting, .review, .pr:
            Circle().fill(DesignTokens.hot).frame(width: size, height: size)
        }
    }
}

/// The design's pulse: down to a third and back, slowly, forever.
struct PulseModifier: ViewModifier {
    @State private var isAnimating = false

    func body(content: Content) -> some View {
        content
            .opacity(isAnimating ? 0.35 : 1)
            .animation(.easeInOut(duration: 0.9).repeatForever(autoreverses: true), value: isAnimating)
            .onAppear { isAnimating = true }
    }
}

private struct PulsesPausedKey: EnvironmentKey {
    static let defaultValue = false
}

extension EnvironmentValues {
    /// Set by a gallery story so a capture never lands mid-pulse.
    var pulsesPaused: Bool {
        get { self[PulsesPausedKey.self] }
        set { self[PulsesPausedKey.self] = newValue }
    }
}

// MARK: - Rows

/// A row's neutral fills: `--sel-2` behind the selected one, `--sel` under
/// the pointer — square and edge to edge, as the design draws them.
struct GnatRowWash: ViewModifier {
    @Environment(\.ground) private var ground
    let selected: Bool
    var hoverable: Bool = true
    @State private var hovering = false

    func body(content: Content) -> some View {
        content
            .background(fill)
            .onHover { hovering = $0 }
    }

    private var fill: Color {
        if selected { return DesignTokens.rowWash(selected: true, on: ground) }
        if hoverable && hovering { return DesignTokens.rowWash(selected: false, on: ground) }
        return .clear
    }
}

extension View {
    func gnatRow(selected: Bool = false, hoverable: Bool = true) -> some View {
        modifier(GnatRowWash(selected: selected, hoverable: hoverable))
    }

    /// The design's mono `xs`: 12pt in the app's monospaced face.
    func monoXS(weight: Font.Weight = .regular) -> some View {
        font(Typo.mono(size: GnatMetrics.xs, weight: weight))
    }
}

// MARK: - Titlebars

/// A column's titlebar: its own header surface, 32pt tall, a line under it. With
/// the system title bar hidden this band *is* the title bar, so its bare
/// parts drag the window and a double-click zooms it, as a real one would.
struct GnatTitlebar<Content: View>: View {
    var leading: CGFloat = 10
    var trailing: CGFloat = 10
    @ViewBuilder let content: () -> Content

    var body: some View {
        HStack(spacing: 10) {
            content()
        }
        .padding(.leading, leading)
        .padding(.trailing, trailing)
        .frame(height: GnatMetrics.titlebarHeight)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            DesignTokens.fill(.header)
                .gesture(TapGesture(count: 2).onEnded {
                    TitlebarDoubleClick.perform(on: NSApp.keyWindow)
                })
                .gesture(WindowDragGesture())
        )
        .environment(\.ground, .header)
        // Drawn straight onto the bottom edge: `rule(edges: [.bottom])`
        // alone lays its line at the top of a row with no height of its own.
        .overlay(alignment: .bottom) {
            DesignTokens.rule(.separator, on: .header).frame(height: 1)
        }
    }
}

// MARK: - Buttons

/// A button label that knows whether the pointer is over it — a style's
/// `makeBody` cannot hold state of its own, so its hover wash is drawn from
/// here. Never hovered while disabled, since a dead control should not
/// answer the pointer.
struct HoverReader<Content: View>: View {
    @Environment(\.isEnabled) private var isEnabled
    @ViewBuilder let content: (Bool) -> Content
    @State private var hovering = false

    var body: some View {
        content(hovering && isEnabled).onHover { hovering = $0 }
    }
}

/// The lift an accent-filled button takes under the pointer: its own fill a
/// shade brighter, so the hover never introduces a colour of its own.
let hoverBrightness = 0.06

extension View {
    /// A filled control's hover: the whole of it a shade brighter.
    func hoverBrightens() -> some View {
        HoverReader { hovering in brightness(hovering ? hoverBrightness : 0) }
    }
}

/// A bare glyph button — a heading's `+`, a card's ✕, a comment's pencil:
/// no chrome at rest, the hover wash behind it under the pointer.
struct GnatIconButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .hoverWash(cornerRadius: 4, enabled: isEnabled)
            .opacity(isEnabled ? (configuration.isPressed ? 0.7 : 1) : 0.4)
            .contentShape(Rectangle())
    }
}

/// The design's `.btn`: a 22pt bordered button, or `.primary`'s accent-dim
/// fill with the accent's own ink; dimmed to 40% when disabled. Under the
/// pointer, a bordered one takes the row wash and a primary one brightens.
struct GnatButtonStyle: ButtonStyle {
    @Environment(\.ground) private var ground
    @Environment(\.isEnabled) private var isEnabled
    var primary = false

    func makeBody(configuration: Configuration) -> some View {
        HoverReader { hovering in
            configuration.label
                .font(.system(size: 13))
                .lineLimit(1)
                .padding(.horizontal, 10)
                .frame(height: 22)
                .foregroundStyle(primary ? DesignTokens.accentInk(on: ground) : DesignTokens.ink(.primary, on: ground))
                .background(
                    RoundedRectangle(cornerRadius: 4)
                        .fill(fill(pressed: configuration.isPressed, hovering: hovering))
                )
                .overlay {
                    if !primary {
                        RoundedRectangle(cornerRadius: 4)
                            .strokeBorder(DesignTokens.rule(.border, on: ground), lineWidth: 1)
                    }
                }
                .brightness(primary && hovering ? hoverBrightness : 0)
                .opacity(isEnabled ? (configuration.isPressed ? 0.85 : 1) : 0.4)
                .contentShape(Rectangle())
        }
    }

    private func fill(pressed: Bool, hovering: Bool) -> Color {
        if primary { return DesignTokens.accentDim(on: ground) }
        return pressed || hovering ? DesignTokens.rowWash(selected: false, on: ground) : .clear
    }
}

/// A link-like button: bare text in the accent ink, no chrome — "Show more",
/// a card's Edit — underlined under the pointer. Dimmed when disabled.
struct GnatLinkButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        HoverReader { hovering in
            configuration.label
                .font(.system(size: 13))
                .underline(hovering)
                .ink(isEnabled ? .accent : .tertiary)
                .opacity(configuration.isPressed ? 0.7 : 1)
                .contentShape(Rectangle())
        }
    }
}

/// A section header's own action: flush to the header's full height, the
/// label beside an optional glyph. A primary one stands on the accent-dim
/// ground with a line on its left; a secondary one is bare on the header
/// and takes the row wash under the pointer.
struct GnatHeaderButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled
    var primary = false

    func makeBody(configuration: Configuration) -> some View {
        HoverReader { hovering in
            configuration.label
                .font(.system(size: 13))
                .lineLimit(1)
                .padding(.horizontal, 12)
                .frame(maxHeight: .infinity)
                .foregroundStyle(primary ? DesignTokens.accentInk(on: .window) : DesignTokens.ink(.primary, on: .chrome))
                .background(background(hovering: hovering))
                .overlay(alignment: .leading) {
                    if primary {
                        DesignTokens.rule(.separator, on: .chrome).frame(width: 1)
                    }
                }
                .brightness(primary && hovering ? hoverBrightness : 0)
                .opacity(isEnabled ? (configuration.isPressed ? 0.85 : 1) : 0.4)
                .contentShape(Rectangle())
        }
    }

    private func background(hovering: Bool) -> Color {
        if primary {
            return isEnabled ? DesignTokens.accentDim(on: .window) : DesignTokens.fill(.window)
        }
        return hovering ? DesignTokens.rowWash(selected: false, on: .chrome) : .clear
    }
}

/// A header action's label: the words, then the design's small glyph.
struct HeaderActionLabel: View {
    let title: String
    var systemImage: String?
    var isBusy = false
    /// A second line of small text under the title — what the action will
    /// run with, such as a launch's model and effort.
    var detail: String?

    var body: some View {
        HStack(spacing: 7) {
            if let detail, !detail.isEmpty {
                VStack(alignment: .leading, spacing: 0) {
                    Text(title).fixedSize()
                    Text(detail)
                        .font(Typo.mono(size: 10))
                        .opacity(0.7)
                        .fixedSize()
                }
            } else {
                Text(title).fixedSize()
            }
            if isBusy {
                ProgressView().controlSize(.mini).frame(width: 11, height: 11)
            } else if let systemImage {
                Image(systemName: systemImage).font(.system(size: 10, weight: .semibold))
            }
        }
    }
}

// MARK: - Notes

/// A muted line standing in for content that is not there: "nothing
/// running", "no slices", a failed read's words.
struct GnatNote: View {
    let text: String
    var role: InkRole = .tertiary
    var leading: CGFloat = 18
    var height: CGFloat = GnatMetrics.rowHeight

    var body: some View {
        Text(text)
            .font(.system(size: GnatMetrics.body))
            .ink(role)
            .lineLimit(1)
            .truncationMode(.tail)
            .padding(.leading, leading)
            .padding(.trailing, 10)
            .frame(height: height)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}
