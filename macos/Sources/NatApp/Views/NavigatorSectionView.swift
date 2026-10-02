import SwiftUI
import NatKit

/// One of the navigator's stacked foldouts: a 32pt header — chevron, label,
/// and the section's own actions flush against its trailing edge — over a
/// body on the window's ground that takes an equal share of what is left
/// while open. A dead section (nothing to show yet) is drawn greyed and
/// cannot be opened.
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
    var live = true
    let onHead: () -> Void
    var onFold: (() -> Void)?
    @ViewBuilder var actions: () -> Actions
    @ViewBuilder var content: () -> Content

    var body: some View {
        VStack(spacing: 0) {
            header
            content()
                .frame(maxWidth: .infinity, maxHeight: open ? .infinity : 0, alignment: .top)
                .surface(.window)
                .overlay(alignment: .top) { DesignTokens.rule(.separator, on: .chrome).frame(height: 1) }
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
                .frame(width: 24, height: GnatMetrics.sectionHeadHeight)
                .padding(.horizontal, -6)
                .contentShape(Rectangle())
                .onTapGesture { if live { (onFold ?? onHead)() } }
            Text(label)
                .font(.system(size: GnatMetrics.body))
                .ink(live ? .primary : .tertiary)
                .frame(width: 58, alignment: .leading)
            Spacer(minLength: 0)
            HStack(spacing: 0) { actions() }
                .frame(maxHeight: .infinity)
        }
        .padding(.leading, 10)
        .frame(height: GnatMetrics.sectionHeadHeight)
        .background(selected ? DesignTokens.rowWash(selected: true, on: .chrome) : DesignTokens.fill(.chrome))
        .contentShape(Rectangle())
        .onTapGesture { if live { onHead() } }
    }
}

extension NavSectionView where Actions == EmptyView {
    init(
        label: String, open: Bool, selected: Bool = false, live: Bool = true,
        onHead: @escaping () -> Void, onFold: (() -> Void)? = nil, @ViewBuilder content: @escaping () -> Content
    ) {
        self.init(
            label: label, open: open, selected: selected, live: live, onHead: onHead, onFold: onFold,
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

/// One card of the Thread log: who, the toned meta, when; then the body;
/// then a mono foot on the chrome ground under a line.
struct ThreadEventCard: View {
    let event: ThreadEvent

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(event.who).monoXS(weight: .medium).ink(.secondary)
                if let meta = event.meta {
                    Text(meta).monoXS().ink(tone).lineLimit(1)
                }
                Spacer(minLength: 0)
            }
            .padding(.horizontal, 10)
            .padding(.top, 8)
            .padding(.bottom, event.body == nil && event.foot == nil ? 8 : 0)

            if let body = event.body {
                Text(markdownAttributed(body, size: 13.5))
                    .font(.system(size: 13.5))
                    .lineSpacing(2)
                    .ink(.primary)
                    .textSelection(.enabled)
                    .padding(.horizontal, 10)
                    .padding(.top, 4)
                    .padding(.bottom, 8)
            }

            if let foot = event.foot {
                Text(foot)
                    .monoXS()
                    .ink(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 5)
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

/// A bordered mono chip: the launch form's model and effort pickers.
struct NavChipMenu<Items: View>: View {
    let title: String
    @ViewBuilder var items: () -> Items

    var body: some View {
        Menu {
            items()
        } label: {
            Text("\(title) \u{25BE}")
                .monoXS()
                .ink(.secondary)
                .padding(.horizontal, 8)
                .padding(.vertical, 2)
                .overlay {
                    RoundedRectangle(cornerRadius: 4).strokeBorder(DesignTokens.rule(.border, on: .window), lineWidth: 1)
                }
        }
        .menuStyle(.borderlessButton)
        .menuIndicator(.hidden)
        .fixedSize()
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
