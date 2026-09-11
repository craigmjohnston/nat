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

/// The label of a button whose action runs async: a small spinner beside the
/// label at the text's own height, shown while the work is in flight — and
/// its slot held whether it is spinning or not, so the button is exactly the
/// same size busy and idle and nothing beside it moves when the work starts.
///
/// (Swapping the label for a bare `ProgressView` lays out at the spinner's
/// full control size whatever `scaleEffect` draws it at — which is how the
/// merge button used to grow into a square. Showing the spinner only while
/// busy fixed the height and left the width jumping by the spinner's own,
/// which in a trailing-aligned row of buttons shoves every button before it
/// sideways for as long as the work runs.)
struct AsyncActionLabel<Label: View>: View {
    let isBusy: Bool
    @ViewBuilder let label: () -> Label

    var body: some View {
        HStack(spacing: 5) {
            BusySlot(isBusy: isBusy)
            label()
        }
    }
}

/// The spinner's slot: 10 by 10 whether anything is spinning in it or not,
/// so what it sits beside is laid out the same either way. Idle it is empty
/// rather than a hidden `ProgressView`, since a spinner nobody can see still
/// animates, and an app with a dozen async buttons on screen would be
/// running a dozen of them for nothing.
struct BusySlot: View {
    let isBusy: Bool
    var label: String = "Working…"

    var body: some View {
        Group {
            if isBusy {
                ProgressView()
                    .controlSize(.small)
                    .scaleEffect(0.55)
                    .accessibilityLabel(label)
            } else {
                Color.clear
                    .accessibilityHidden(true)
            }
        }
        .frame(width: 10, height: 10)
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

/// The button type system: primary is the one confirming action on a screen,
/// secondary is real but quiet, and ghost is present but receding. All three
/// are drawn to `ButtonMetrics`, so a submit is the same shape wherever it is
/// pressed and a secondary sits beside it on the same baseline. Hover washes
/// stay the caller's job via .hoverWash().
///
/// A button whose action runs async wraps its label in `AsyncActionLabel`
/// rather than swapping the label out: with the height fixed here and the
/// spinner's slot held there, a button is exactly the same size busy and
/// idle.

/// The one submit style: a flat accent fill, not the brand gradient. The
/// app's icon and its progress bars already carry the gradient, and a button
/// shouting it too was one gradient too many — the agent-launch control had
/// already gone flat on its own, and this is the rest of the app following
/// it rather than the two disagreeing.
struct PrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: Typo.subhead, weight: .semibold))
            .foregroundStyle(DesignTokens.accentText)
            .padding(.horizontal, ButtonMetrics.horizontalPadding)
            .frame(height: ButtonMetrics.height)
            .background(
                DesignTokens.accent,
                in: RoundedRectangle(cornerRadius: ButtonMetrics.cornerRadius)
            )
            .opacity(buttonOpacity(isPressed: configuration.isPressed, isEnabled: isEnabled))
            .contentShape(RoundedRectangle(cornerRadius: ButtonMetrics.cornerRadius))
    }
}

/// The quiet half of the pair: cancels, retries and the actions that sit
/// beside a submit without being one. Same height and radius as primary, so
/// a row of the two lines up and differs in weight alone.
struct SecondaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: Typo.subhead, weight: .regular))
            .foregroundStyle(DesignTokens.label)
            .padding(.horizontal, ButtonMetrics.horizontalPadding)
            .frame(height: ButtonMetrics.height)
            .background(
                DesignTokens.controlFace,
                in: RoundedRectangle(cornerRadius: ButtonMetrics.cornerRadius)
            )
            .overlay(
                RoundedRectangle(cornerRadius: ButtonMetrics.cornerRadius)
                    .stroke(DesignTokens.hairline, lineWidth: 1)
            )
            .opacity(buttonOpacity(isPressed: configuration.isPressed, isEnabled: isEnabled))
            .contentShape(RoundedRectangle(cornerRadius: ButtonMetrics.cornerRadius))
    }
}

struct GhostButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: Typo.subhead, weight: .regular))
            .foregroundStyle(DesignTokens.labelSecondary)
            .padding(.horizontal, ButtonMetrics.ghostHorizontalPadding)
            .frame(height: ButtonMetrics.height)
            .opacity(buttonOpacity(isPressed: configuration.isPressed, isEnabled: isEnabled))
            .contentShape(Rectangle())
    }
}

/// What all three styles dim to, so pressed and disabled read the same
/// whichever one was pressed. Pressed wins over disabled because a disabled
/// button cannot be pressed at all.
func buttonOpacity(isPressed: Bool, isEnabled: Bool) -> Double {
    if isPressed { return ButtonMetrics.pressedOpacity }
    return isEnabled ? 1.0 : ButtonMetrics.disabledOpacity
}
