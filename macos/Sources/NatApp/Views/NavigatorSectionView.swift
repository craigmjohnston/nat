import SwiftUI
import NatKit

/// One of the navigator's stacked foldouts: a 32pt header — chevron, label,
/// and the section's own actions flush against its trailing edge — and its
/// rule, over a
/// body on the window's ground that takes an equal share of what is left
/// while open. A section with nothing to show yet is not drawn at all — the
/// navigator leaves it out.
///
/// The chevron only folds (`onFold`); the rest of the header is `onHead`,
/// which may also put the section's view up in the main pane. The body is
/// built once and kept while folded — collapsed to nothing rather than torn
/// down — so unfolding it again redraws nothing and reloads nothing: its
/// scroll position, its reads and its rendered markdown are all as left.
struct NavSectionView<Actions: View, Content: View>: View {
    let label: String
    let open: Bool
    var selected = false
    let onHead: () -> Void
    var onFold: (() -> Void)?
    @ViewBuilder var actions: () -> Actions
    @ViewBuilder var content: () -> Content

    var body: some View {
        VStack(spacing: 0) {
            header
            // Under the band, not inside it — open or folded — so the band is
            // the full height the sidebar's and the main pane's headings are.
            DesignTokens.rule(.separator, on: .chrome).frame(height: 1)
            content()
                .frame(maxWidth: .infinity, maxHeight: open ? .infinity : 0, alignment: .top)
                .surface(.window)
                .clipped()
                .opacity(open ? 1 : 0)
                .allowsHitTesting(open)
                .accessibilityHidden(!open)
        }
        .frame(maxHeight: open ? .infinity : nil)
        .overlay(alignment: .bottom) { DesignTokens.rule(.separator, on: .chrome).frame(height: 1) }
    }

    private var header: some View {
        HStack(spacing: 8) {
            DisclosureChevron(open: open)
                // Turned at once, as the section snaps.
                .transaction { $0.animation = nil }
                .frame(width: 24, height: GnatMetrics.sectionHeadHeight)
                .padding(.horizontal, -6)
                .contentShape(Rectangle())
                .onTapGesture { (onFold ?? onHead)() }
            Text(label)
                .font(.system(size: GnatMetrics.body))
                .ink(.primary)
                // A label longer than the column ("Visual changes") takes
                // the room it needs rather than truncating.
                .fixedSize(horizontal: true, vertical: false)
                .frame(minWidth: 58, alignment: .leading)
            Spacer(minLength: 0)
            HStack(spacing: 0) { actions() }
                .frame(maxHeight: .infinity)
        }
        .padding(.leading, 10)
        .frame(height: GnatMetrics.sectionHeadHeight)
        .background(selected ? DesignTokens.rowWash(selected: true, on: .chrome) : DesignTokens.fill(.chrome))
        .contentShape(Rectangle())
        .onTapGesture { onHead() }
    }
}

extension NavSectionView where Actions == EmptyView {
    init(
        label: String, open: Bool, selected: Bool = false,
        onHead: @escaping () -> Void, onFold: (() -> Void)? = nil, @ViewBuilder content: @escaping () -> Content
    ) {
        self.init(
            label: label, open: open, selected: selected, onHead: onHead, onFold: onFold,
            actions: { EmptyView() }, content: content)
    }
}


/// The navigator column: whatever sections the selection has, and a filler
/// taking the column's slack when every section is folded. Its title is the
/// window titlebar's, drawn by the shell.
struct NavigatorColumn<Sections: View>: View {
    var anyOpen: Bool
    @ViewBuilder var sections: () -> Sections

    var body: some View {
        VStack(spacing: 0) {
            sections()
            if !anyOpen {
                Spacer(minLength: 0)
            }
        }
        .frame(maxHeight: .infinity, alignment: .top)
        .rule(.separator, edges: [.trailing], width: 1)
        // The last section's bottom line sits on the status bar's own
        // top line rather than one point above it, so the two are one.
        .padding(.bottom, -1)
        .surface(.chrome)
    }
}

/// A navigator body's prose block: the design's `.prose` padding and type.
struct NavProse<Content: View>: View {
    @ViewBuilder var content: () -> Content

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            content()
        }
        .font(.system(size: GnatMetrics.body))
        .lineSpacing(3)
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// The design's small uppercase heading inside a section body.
struct NavHeading: View {
    let text: String

    var body: some View {
        Text(text.uppercased())
            .font(.system(size: 12))
            .tracking(0.7)
            .ink(.secondary)
            .padding(.top, 4)
    }
}

/// One card of the Thread log: its icon, who, the toned meta; then the body,
/// cut short as the brief is; then its labelled facts on the chrome ground under a
/// line.
struct ThreadEventCard: View {
    let event: ThreadEvent

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                HStack(alignment: .firstTextBaseline, spacing: 6) {
                    ThreadIcon(symbol: event.kind.symbol)
                    Text(event.who).monoXS(weight: .medium).ink(.secondary)
                }
                if let meta = event.meta {
                    Text(meta).monoXS().ink(tone).lineLimit(1)
                }
                Spacer(minLength: 0)
            }
            .padding(.horizontal, 10)
            .padding(.top, 8)
            .padding(.bottom, event.body == nil && event.facts.isEmpty ? 8 : 0)

            if let body = event.body {
                Excerpt(text: body) { shown in
                    Text(markdownAttributed(shown, size: 13.5))
                        .font(.system(size: 13.5))
                        .lineSpacing(2)
                        .ink(.primary)
                        .textSelection(.enabled)
                }
                    .padding(.horizontal, 10)
                    .padding(.top, 4)
                    .padding(.bottom, 8)
            }

            if !event.facts.isEmpty {
                // Labelled values, as the brief's own facts are drawn.
                Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 3) {
                    ForEach(Array(event.facts.enumerated()), id: \.offset) { _, fact in
                        GridRow {
                            Text(fact.key).ink(.tertiary)
                            Text(fact.value)
                                .ink(.primary)
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }
                    }
                }
                .monoXS()
                .padding(.horizontal, 10)
                .padding(.vertical, 6)
                .frame(maxWidth: .infinity, alignment: .leading)
                .surface(.chrome)
                .overlay(alignment: .top) { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
                .padding(.top, event.body == nil ? 8 : 0)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .clipShape(RoundedRectangle(cornerRadius: 4))
        .overlay {
            RoundedRectangle(cornerRadius: 4).strokeBorder(DesignTokens.rule(.separator, on: .window), lineWidth: 1)
        }
    }

    private var tone: InkRole {
        switch event.tone {
        case .muted: return .secondary
        case .accent: return .accent
        case .hot: return .hot
        }
    }
}

/// A Thread fact's value the user can change: the value as a fact draws it,
/// with a small caret after it and the hover wash under it, opening a menu of
/// the choices — the launch card's model and effort. An empty value reads
/// "default", a step quieter than a chosen one. Disabled, it is drawn as a
/// plain fact in the quaternary ink, with no caret.
struct NavFactMenu<Items: View>: View {
    @Environment(\.isEnabled) private var isEnabled
    let value: String
    @ViewBuilder var items: () -> Items

    var body: some View {
        if isEnabled {
            Menu {
                items()
            } label: {
                HStack(spacing: 4) {
                    text
                    Image(systemName: "chevron.down")
                        .font(.system(size: 8, weight: .semibold))
                        .ink(.tertiary)
                }
                .monoXS()
                .padding(.horizontal, 4)
                .hoverWash(cornerRadius: 4)
                .contentShape(Rectangle())
            }
            .menuStyle(.button)
            .buttonStyle(.plain)
            .menuIndicator(.hidden)
            .fixedSize()
            // Back out the padding the wash needs, so the value starts in the
            // column every other fact's value starts in.
            .padding(.horizontal, -4)
        } else {
            // Disabled, a plain fact: a disabled Menu fades its label on top
            // of any ink, which would leave it fainter than the facts beside it.
            text.monoXS()
        }
    }

    private var text: some View {
        Text(value.isEmpty ? "default" : value)
            .ink(isEnabled ? (value.isEmpty ? .secondary : .primary) : .quaternary)
            .lineLimit(1)
    }
}

/// A one-line notice in a section body: a refusal, a warning, a stale read.
struct NavNotice: View {
    let text: String
    var role: InkRole = .danger

    var body: some View {
        Text(text)
            .font(.system(size: 13))
            .ink(role)
            .fixedSize(horizontal: false, vertical: true)
            .textSelection(.enabled)
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}
