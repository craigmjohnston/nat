import AppKit
import SwiftUI
import NatKit

// MARK: - Hover

/// The design system's hover treatment: a quiet quaternary-label wash on
/// borderless, toolbar and sidebar items — nothing moves, nothing scales.
/// Drawn as a `background`, so a row that paints its own selection fill after
/// this modifier covers the wash while selected and shows it again when not.
struct HoverWash: ViewModifier {
    var cornerRadius: CGFloat = 6
    var enabled: Bool = true
    @State private var hovering = false

    func body(content: Content) -> some View {
        content
            .background(
                RoundedRectangle(cornerRadius: cornerRadius)
                    .fill(enabled && hovering ? DesignTokens.labelQuaternary : Color.clear)
            )
            .onHover { hovering = $0 }
    }
}

extension View {
    /// See `HoverWash`. `enabled: false` keeps the tracking but draws nothing
    /// — for an item whose hover would promise a click it will not honour.
    func hoverWash(cornerRadius: CGFloat = 6, enabled: Bool = true) -> some View {
        modifier(HoverWash(cornerRadius: cornerRadius, enabled: enabled))
    }
}

// MARK: - Async actions

/// The label of a button whose action runs async: while the work is in
/// flight a small spinner appears beside the label at the text's own height,
/// so the button keeps its size. (Swapping the label for a bare
/// `ProgressView` lays out at the spinner's full control size whatever
/// `scaleEffect` draws it at — which is how the merge button used to grow
/// into a square.)
struct AsyncActionLabel<Label: View>: View {
    let isBusy: Bool
    @ViewBuilder let label: () -> Label

    var body: some View {
        HStack(spacing: 5) {
            if isBusy {
                ProgressView()
                    .controlSize(.small)
                    .scaleEffect(0.55)
                    .frame(width: 10, height: 10)
            }
            label()
        }
    }
}

// MARK: - Pane resizing

/// The draggable strip along a resizable pane's divider: invisible, a few
/// points wide, wearing the horizontal-resize cursor and dragging the bound
/// width between its bounds. Overlaid on the divider edge of the view beside
/// the line and offset to straddle it — the width itself is the caller's
/// state, which is what lets each pane persist its own.
struct PaneResizeHandle: View {
    @Binding var width: Double
    let minWidth: Double
    let maxWidth: Double
    /// Which edge of the *resized* pane this handle sits on — see
    /// `PaneResizeEdge`.
    let edge: PaneResizeEdge

    /// The width when the current drag began — nil between drags. Deltas are
    /// measured from it rather than accumulated per event; see
    /// `paneResizedWidth`.
    @State private var startWidth: Double?

    var body: some View {
        Color.clear
            .frame(width: 9)
            .contentShape(Rectangle())
            .background(ResizeCursorView())
            .gesture(
                DragGesture(minimumDistance: 1, coordinateSpace: .global)
                    .onChanged { value in
                        let base = startWidth ?? width
                        startWidth = base
                        width = paneResizedWidth(
                            startWidth: base,
                            translation: value.translation.width,
                            edge: edge,
                            minWidth: minWidth,
                            maxWidth: maxWidth
                        )
                        // Cursor updates pause during a drag, but a drag that
                        // began just outside the tracking area (the handle is
                        // 9pt wide) would otherwise run under the arrow.
                        NSCursor.resizeLeftRight.set()
                    }
                    .onEnded { _ in startWidth = nil }
            )
    }
}

// MARK: - Rect Edge Set

struct RectEdgeSet: OptionSet {
    let rawValue: Int

    static let top = RectEdgeSet(rawValue: 1)
    static let leading = RectEdgeSet(rawValue: 2)
    static let bottom = RectEdgeSet(rawValue: 4)
    static let trailing = RectEdgeSet(rawValue: 8)
    static let all: RectEdgeSet = [.top, .leading, .bottom, .trailing]
}

// MARK: - View Extensions

extension View {
    func rectBorder(width: CGFloat, edges: RectEdgeSet, color: Color) -> some View {
        overlay(alignment: .topLeading) {
            VStack(spacing: 0) {
                if edges.contains(.top) {
                    color.frame(height: width)
                }
                HStack(spacing: 0) {
                    if edges.contains(.leading) {
                        color.frame(width: width)
                    }
                    Spacer()
                    if edges.contains(.trailing) {
                        color.frame(width: width)
                    }
                }
                if edges.contains(.bottom) {
                    color.frame(height: width)
                }
            }
        }
    }

    func rectBorderTrailing(width: CGFloat, color: Color) -> some View {
        self.rectBorder(width: width, edges: [.trailing], color: color)
    }
}

// MARK: - Button grammar

/// The button type system: primary is the one gradient action on a screen,
/// secondary is real but quiet, and ghost is present but receding. Hover
/// washes stay the caller's job via .hoverWash().
struct PrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: Typo.subhead, weight: .semibold))
            .foregroundColor(.white)
            .padding(.horizontal, 12)
            .padding(.vertical, 5)
            .background(DesignTokens.brandGradient, in: RoundedRectangle(cornerRadius: 6))
            .opacity(configuration.isPressed ? 0.85 : (isEnabled ? 1.0 : 0.5))
    }
}

struct SecondaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: Typo.subhead, weight: .regular))
            .foregroundColor(DesignTokens.label)
            .padding(.horizontal, 12)
            .padding(.vertical, 5)
            .background(DesignTokens.controlFace, in: RoundedRectangle(cornerRadius: 6))
            .overlay(
                RoundedRectangle(cornerRadius: 6)
                    .stroke(DesignTokens.hairline, lineWidth: 1)
            )
            .opacity(configuration.isPressed ? 0.8 : (isEnabled ? 1.0 : 0.5))
    }
}

struct GhostButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: Typo.subhead, weight: .regular))
            .foregroundColor(DesignTokens.labelSecondary)
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .opacity(configuration.isPressed ? 0.7 : (isEnabled ? 1.0 : 0.5))
    }
}
