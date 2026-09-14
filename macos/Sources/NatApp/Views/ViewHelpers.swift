import AppKit
import SwiftUI
import NatKit

// MARK: - Hover

/// The design system's hover treatment: the theme's own hover surface laid
/// on borderless, toolbar and sidebar items — nothing moves, nothing scales.
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
                    .fill(enabled && hovering ? DesignTokens.fill(.hover) : Color.clear)
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
/// shown by taking room rather than by filling room already held, so the
/// button sits at its text's own width when idle and grows by the spinner
/// for exactly as long as one is spinning.
///
/// The label stays where it is throughout: the spinner joins it rather than
/// replacing it, so the button always says what it is doing. (Swapping the
/// label for a bare `ProgressView` lays out at the spinner's full control
/// size whatever `scaleEffect` draws it at — which is how the merge button
/// used to grow into a square — and leaves the button reading as nothing but
/// a spinner while the work runs.)
///
/// The slot was held either way once, so nothing beside the button moved when
/// the work started; what that cost was every idle button wearing a blank
/// 10pt column and the reserved `frame(width:)` paddings that made the gap
/// look deliberate. Moving a neighbour for the length of a launch is the
/// cheaper of the two, and the growth is itself a sign the press landed.
struct AsyncActionLabel<Label: View>: View {
    let isBusy: Bool
    @ViewBuilder let label: () -> Label

    var body: some View {
        HStack(spacing: 5) {
            if isBusy {
                BusySpinner()
            }
            label()
        }
    }
}

/// The spinner itself: sized down to 10 by 10 rather than laid out at the
/// control's own size, which `scaleEffect` draws smaller without ever
/// shrinking, so it sits at the height of the text beside it.
struct BusySpinner: View {
    var label: String = "Working…"

    var body: some View {
        ProgressView()
            .controlSize(.small)
            .scaleEffect(0.55)
            .accessibilityLabel(label)
            .frame(width: 10, height: 10)
    }
}

/// The spinner's slot for what wants the room held whether anything is
/// spinning in it or not — a mark on a pane rather than a button's label,
/// where there is no press to explain a line of content shifting. Idle it is
/// empty rather than a hidden `ProgressView`, since a spinner nobody can see
/// still animates, and a screenful of them would be running for nothing.
struct BusySlot: View {
    let isBusy: Bool
    var label: String = "Working…"

    var body: some View {
        Group {
            if isBusy {
                BusySpinner(label: label)
            } else {
                Color.clear
                    .accessibilityHidden(true)
                    .frame(width: 10, height: 10)
            }
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

    /// Where the strip currently sits in the window, kept live because the
    /// divider moves under the pointer as the drag resizes the pane — it is
    /// what a finished drag's last location is tested against; see
    /// `paneDragEndedOverHandle`.
    @State private var frame: CGRect = .zero

    var body: some View {
        Color.clear
            .frame(width: 9)
            .contentShape(Rectangle())
            // SwiftUI's own pointer style rather than an AppKit cursor of our
            // own. A cursor rect only applies to the view AppKit finds under
            // the pointer, so the one this handle used to carry — mounted
            // behind it and opting out of hit testing, the better to leave the
            // drag gesture the mouse — was never the view found, and the
            // window's cursor floor answered every hover with the arrow.
            // SwiftTerm's I-beam is the same mechanism done the other way and
            // is exactly why it wins: an ordinary, hit-testable view. This is
            // the SwiftUI-level equivalent, applied to the strip itself, which
            // is hit-testable by construction — `contentShape` above is what
            // the drag already relies on.
            .pointerStyle(.columnResize)
            .background(
                GeometryReader { proxy in
                    Color.clear
                        .onChange(of: proxy.frame(in: .global), initial: true) { _, rect in
                            frame = rect
                        }
                }
            )
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
                    .onEnded { value in
                        startWidth = nil
                        // The cursor the drag kept setting stays up until the
                        // pointer crosses into a cursor rect again, so a drag
                        // that ended away from the strip has to hand the arrow
                        // back itself.
                        if paneDragEndedOverHandle(handleFrame: frame, endLocation: value.location) {
                            NSCursor.resizeLeftRight.set()
                        } else {
                            NSCursor.arrow.set()
                        }
                    }
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
/// rather than swapping the label out: with the height fixed here, the only
/// thing a spinner can change is the width — which it does, for as long as it
/// shows, and never the label, which stays visible under it.

/// The one submit style: a flat accent fill, not the brand gradient. The
/// app's icon and its progress bars already carry the gradient, and a button
/// shouting it too was one gradient too many — the agent-launch control had
/// already gone flat on its own, and this is the rest of the app following
/// it rather than the two disagreeing.
extension View {
    /// A rule along some edges, at the weight the separation calls for and in
    /// the colour of whatever ground it has landed on — `rectBorder` with the
    /// colour decided rather than passed, which is the last place a view had
    /// to name one to draw a line.
    func rule(_ weight: RuleWeight = .separator, edges: RectEdgeSet, width: CGFloat = 0.5) -> some View {
        modifier(RuleBorderModifier(weight: weight, edges: edges, width: width))
    }
}

private struct RuleBorderModifier: ViewModifier {
    let weight: RuleWeight
    let edges: RectEdgeSet
    let width: CGFloat
    @Environment(\.ground) private var ground

    func body(content: Content) -> some View {
        content.rectBorder(width: width, edges: edges, color: DesignTokens.rule(weight, on: ground))
    }
}

struct PrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: Typo.subhead, weight: .semibold))
            .ink(.onAccent)
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
            .ink(.primary)
            .padding(.horizontal, ButtonMetrics.horizontalPadding)
            .frame(height: ButtonMetrics.height)
            .control(radius: ButtonMetrics.cornerRadius, border: .hairline)
            .opacity(buttonOpacity(isPressed: configuration.isPressed, isEnabled: isEnabled))
            .contentShape(RoundedRectangle(cornerRadius: ButtonMetrics.cornerRadius))
    }
}

struct GhostButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: Typo.subhead, weight: .regular))
            .ink(.secondary)
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

// MARK: - Inspector actions

/// The primary and secondary styles a content pane's CTAs are drawn in at
/// the top of its inspector — `PrimaryButtonStyle`/`SecondaryButtonStyle`'s
/// own fill, ink and weight, but full-width and at `InspectorActionMetrics`'
/// taller height rather than `ButtonMetrics`', since these stand alone atop
/// the rail rather than beside another control on the same baseline.
struct InspectorPrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: Typo.subhead, weight: .semibold))
            .ink(.onAccent)
            .frame(maxWidth: .infinity)
            .frame(height: InspectorActionMetrics.height)
            .background(
                DesignTokens.accent,
                in: RoundedRectangle(cornerRadius: InspectorActionMetrics.cornerRadius)
            )
            .opacity(buttonOpacity(isPressed: configuration.isPressed, isEnabled: isEnabled))
            .contentShape(RoundedRectangle(cornerRadius: InspectorActionMetrics.cornerRadius))
    }
}

struct InspectorSecondaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: Typo.subhead, weight: .regular))
            .ink(.primary)
            .frame(maxWidth: .infinity)
            .frame(height: InspectorActionMetrics.height)
            .control(radius: InspectorActionMetrics.cornerRadius, border: .hairline)
            .opacity(buttonOpacity(isPressed: configuration.isPressed, isEnabled: isEnabled))
            .contentShape(RoundedRectangle(cornerRadius: InspectorActionMetrics.cornerRadius))
    }
}

/// The split control an inspector opens with when its primary action needs a
/// menu of options beside it — the Launch Agent button's own shape (a flat
/// accent fill in two halves, split by a hairline drawn on the accent rather
/// than a surface), pulled out of `BriefTabView` so it is full-width at the
/// top of a rail instead of one control among others in a footer.
///
/// Dimmed as a whole rather than through each half's own disabled state,
/// since a split control half-dimmed would read as only one half of it being
/// unavailable.
struct InspectorSplitButton<Menu: View>: View {
    let title: String
    var isBusy: Bool = false
    var isEnabled: Bool = true
    let onPrimary: () -> Void
    @Binding var showMenu: Bool
    @ViewBuilder var menu: () -> Menu

    var body: some View {
        HStack(spacing: 0) {
            Button(action: onPrimary) {
                AsyncActionLabel(isBusy: isBusy) {
                    Text(title)
                }
                .font(.system(size: Typo.subhead, weight: .semibold))
                .ink(.onAccent)
                .frame(maxWidth: .infinity)
                .frame(height: InspectorActionMetrics.height)
            }
            .buttonStyle(.plain)
            .disabled(!isEnabled || isBusy)

            Rectangle()
                .fill(DesignTokens.onAccentSeparator)
                .frame(width: 1, height: InspectorActionMetrics.height)

            Button(action: { showMenu.toggle() }) {
                Image(systemName: "chevron.down")
                    .font(.system(size: 12, weight: .medium))
                    .ink(.onAccent)
                    .frame(width: InspectorActionMetrics.height, height: InspectorActionMetrics.height)
            }
            .buttonStyle(.plain)
            .disabled(!isEnabled)
        }
        .background(DesignTokens.accent)
        .clipShape(RoundedRectangle(cornerRadius: InspectorActionMetrics.cornerRadius))
        .opacity(isEnabled ? 1 : ButtonMetrics.disabledOpacity)
        .popover(isPresented: $showMenu, arrowEdge: .bottom) {
            menu().padding(10)
        }
    }
}

/// The band the actions live in: stacked full-width, primary over secondary,
/// with the hairline that separates them from the fields below — the
/// standing home for a content pane's CTAs now that the bottom button bar is
/// gone. Any pane with a sidebar/inspector opens it with this.
struct InspectorActionsBar<Content: View>: View {
    @ViewBuilder var content: () -> Content

    var body: some View {
        VStack(spacing: 0) {
            VStack(spacing: 8) {
                content()
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 14)

            Rule(.hairline)
        }
    }
}

/// What a footer bar used to carry besides its buttons — the busy mark, a
/// status line, whatever notice is live — now pinned at the foot of the
/// inspector instead of the window's own bottom edge: a hairline above it
/// and its own faint fill mark it as a band of its own, same as the bar it
/// replaces.
///
/// Draws no slot of its own when `content` has nothing in it: a caller whose
/// notices are all conditional (the Brief tab's launch error/warning, held
/// alongside a busy mark that only shows while refreshing) wraps this in its
/// own `if` rather than mounting it unconditionally, so an idle sidebar
/// shows no band, no rule and no reserved height at all. A caller with an
/// unconditional line of its own (Diff and PR's file-count/PR-status
/// heading) always has something to say and mounts this plainly.
struct InspectorStatusFoot<Content: View>: View {
    @ViewBuilder var content: () -> Content

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            content()
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .overlay(alignment: .top) { Rule(.hairline) }
        .surface(.band)
    }
}

// MARK: - Model picker

/// A menu picker for a model field: `AgentOptions`' own alias set (see
/// `AgentOptions.fallback`), plus "Default" for empty — the config's own
/// spelling of "leave it to Claude Code" — and "Custom…", which reveals a
/// free-text field for a full model ID (`claude-…`). A full ID is always a
/// valid value and there is no API to enumerate every alias
/// (`AgentOptions.fallback`'s own doc comment says why), so Custom is the
/// escape hatch rather than an edge case. A configured value that is not one
/// of the known aliases selects Custom and shows it in the field, so a full
/// ID round-trips instead of landing on a blank selection.
///
/// The custom field is the caller's own view — Settings and the launch
/// popover each draw their model `TextField` slightly differently (a
/// commit-on-blur field with a fixed width in one, a plain bound field at
/// the popover's own font in the other) — so this only decides when to show
/// it, never how.
struct ModelPicker<CustomField: View>: View {
    @Binding var value: String
    let options: [String]
    var commit: () -> Void = {}
    @ViewBuilder var customField: (Binding<String>) -> CustomField

    /// Set the moment "Custom…" is picked, so the field shows even before
    /// anything has been typed into it — `value` alone cannot tell "Custom,
    /// empty so far" apart from "Default"; see `ModelPickerRules`.
    @State private var forcedCustom = false

    private var showsCustomField: Bool {
        ModelPickerRules.showsCustomField(value: value, options: options, forcedCustom: forcedCustom)
    }

    private var selection: Binding<String> {
        Binding(
            get: { ModelPickerRules.selectionTag(value: value, options: options, forcedCustom: forcedCustom) },
            set: { newTag in
                if newTag == ModelPickerRules.customTag {
                    forcedCustom = true
                } else {
                    forcedCustom = false
                    value = newTag
                }
                commit()
            }
        )
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Picker("", selection: selection) {
                Text("Default").tag("")
                ForEach(options, id: \.self) { option in
                    Text(option).tag(option)
                }
                Text("Custom…").tag(ModelPickerRules.customTag)
            }
            .labelsHidden()
            .pickerStyle(.menu)

            if showsCustomField {
                customField($value)
            }
        }
    }
}

/// One line of an inspector's pinned foot: an optional glyph, the message,
/// and the tint both are read in — a stale-read warning, a send/approve/merge
/// error, a comment dropped from under a stale diff.
struct InspectorNotice: View {
    let text: String
    var systemImage: String?
    var role: InkRole = .warning

    var body: some View {
        HStack(spacing: 8) {
            if let systemImage {
                Image(systemName: systemImage)
                    .font(.system(size: 12, weight: .medium))
                    .ink(role)
            }
            Text(text)
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(role)
                .lineLimit(2)
            Spacer()
        }
    }
}
