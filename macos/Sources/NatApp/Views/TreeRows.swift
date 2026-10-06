import SwiftUI
import NatKit

/// What names a piece of work as the sidebar's Active fold does: the
/// project's badge and a quiet slash, as the breadcrumb sets a project
/// crumb, then its state dot (or, for a workshop, its `symbol` in the dot's
/// ink) directly left of its title. No tag, no badge and no slash (the
/// breadcrumb's last crumb, whose project crumb already names the project).
/// The Active rows and the navigator's titlebar both draw it, so the two
/// read alike.
struct ActiveIdentityLabel: View {
    let tag: String
    /// The project's colour, the badge's; nil for the quiet chip.
    var color: ProjectColor?
    /// The project's full name, the badge's tooltip.
    var projectName: String?
    /// A source project's plugin icon, its badge's.
    var projectIcon: SourceIcon?
    let state: SliceDisplayState
    let live: Bool
    let title: String
    var symbol: String?
    var size: CGFloat = GnatMetrics.body
    var titleInk: InkRole = .secondary

    var body: some View {
        HStack(spacing: 6) {
            if !tag.isEmpty {
                ProjectBadgeView(tag: tag, color: color, name: projectName, icon: projectIcon)
                CrumbSlash()
            }
            Group {
                if let symbol {
                    StateSymbol(symbol: symbol, state: state, live: live)
                } else {
                    StateDot(state: state, live: live)
                }
            }
            .frame(width: GnatMetrics.treeGlyphColumn)
            Text(title)
                .ink(titleInk)
                .lineLimit(1)
        }
        .font(.system(size: size))
    }
}

/// The quiet slash after a crumb — the breadcrumb's, and the one after an
/// identity's badge. A slash descends below the baseline, so its glyph's
/// middle sits a point under the text's; it is lifted that point, without
/// moving its frame, onto the line the rest share.
struct CrumbSlash: View {
    var body: some View {
        Text("/")
            .ink(.quaternary)
            .frame(height: CrumbLine.height)
            .offset(y: -1)
            .fixedSize()
    }
}

/// The breadcrumb's one line: every crumb's glyph is framed to the crumb
/// text's line height, so the row centres them all on the text's middle
/// rather than each on its own bounds.
enum CrumbLine {
    static let height: CGFloat = 16
}

/// A milestone line of a plan tree, as the sidebar draws it: its folder,
/// outlined or open, its name and its count. Folding, where there is any, is
/// the caller's; a line that `folds` draws as a project row does under the
/// pointer — washed, its folder giving way to the chevron the click works.
struct TreeMilestoneLine: View {
    @Environment(\.ground) private var ground
    @Environment(\.hoverForced) private var hoverForced
    @State private var hovering = false
    let name: String
    let count: String
    var open = true
    var indent: CGFloat = 26
    var isDone = false
    /// A proposed milestone the proposal creates: NEW beside its name, as
    /// the workshop's Plan tab marks it.
    var isNew = false
    /// Whether a click folds it — what earns it the hover chevron and wash.
    var folds = false
    /// The row's right-click menu, for the three-dot button that takes the
    /// count's slot under the pointer — nil, no button.
    var menu: (() -> AnyView)?

    var body: some View {
        let showsMenu = menu != nil && (hovering || hoverForced)
        HStack(spacing: 7) {
            // A milestone's fold mark: one folder, outlined or open, always in
            // the muted ink — never the accent.
            Group {
                if folds && (hovering || hoverForced) {
                    DisclosureChevron(open: open)
                } else if isDone {
                    DoneFolderGlyph(open: open, color: DesignTokens.ink(.tertiary, on: ground))
                } else {
                    FolderGlyph(open: open, color: DesignTokens.ink(.tertiary, on: ground))
                }
            }
            .frame(width: GnatMetrics.treeFolderColumn)
            // Every live line of the tree is one ink — milestones, projects
            // and slices alike; only the Done folder recedes with what it holds.
            Text(name)
                .font(.system(size: GnatMetrics.body))
                .ink(isDone ? .tertiary : .secondary)
                .lineLimit(1)
            if isNew { Chip("New", tone: .accent, size: .small) }
            Spacer(minLength: 0)
            // Under the pointer the three-dot takes the count's place — one
            // slot, at least the button's width, holding both — so nothing
            // on the row moves as it comes and goes.
            ZStack(alignment: .trailing) {
                Text(count).monoXS().ink(isDone ? .quaternary : .tertiary)
                    .opacity(showsMenu ? 0 : 1)
                    .accessibilityHidden(showsMenu)
                if let menu {
                    RowMenuButton(items: menu)
                        .opacity(showsMenu ? 1 : 0)
                        .allowsHitTesting(showsMenu)
                        .accessibilityHidden(!showsMenu)
                }
            }
            .frame(minWidth: menu == nil ? 0 : RowMenuSlot.tree, alignment: .trailing)
        }
        .padding(.leading, indent)
        .padding(.trailing, 10)
        .frame(height: GnatMetrics.sidebarRowHeight)
        .gnatRow(hoverable: folds)
        .onHover { if folds || menu != nil { hovering = $0 } }
    }
}

/// A slice line of a plan tree, as the sidebar draws it: its dot and title.
struct TreeSliceLine: View {
    @Environment(\.hoverForced) private var hoverForced
    @State private var hovering = false
    let title: String
    let state: SliceDisplayState
    var live = false
    var selected = false
    var indent: CGFloat = 34
    /// Its pull request's marks, at the trailing edge.
    var marks: PRMarks = .none
    /// The row's right-click menu, for the three-dot button at the trailing
    /// edge under the pointer — nil, no button and no slot for one.
    var menu: (() -> AnyView)?

    var body: some View {
        let showsMenu = menu != nil && (hovering || hoverForced)
        // Done and blocked both recede to the faintest ink — blocked since it
        // is not available at all, done since it is finished — and done
        // fades further still under its strike, so finished work sits back
        // behind everything that is not. Everything else takes the tree's
        // one ink, a step under the primary.
        let ink: InkRole = state == .blocked || state == .done ? .quaternary : .secondary
        HStack(spacing: 6) {
            StateDot(state: state, live: live).frame(width: GnatMetrics.treeGlyphColumn)
            Text(title)
                .font(.system(size: GnatMetrics.body))
                .strikethrough(state == .done)
                .ink(ink)
                .lineLimit(1)
            Spacer(minLength: 0)
            PRMarksView(marks: marks)
            // Its slot always kept, so the title does not re-truncate as
            // the pointer comes and goes, and the marks keep their place.
            if let menu {
                RowMenuButton(items: menu)
                    .opacity(showsMenu ? 1 : 0)
                    .allowsHitTesting(showsMenu)
                    .accessibilityHidden(!showsMenu)
            }
        }
        .opacity(state == .done ? 0.7 : 1)
        .padding(.leading, indent)
        .padding(.trailing, 10)
        .frame(height: GnatMetrics.sidebarRowHeight)
        .gnatRow(selected: selected)
        .contentShape(Rectangle())
        .onHover { if menu != nil { hovering = $0 } }
    }
}

/// A row's three-dot button: the very menu a right-click on the row opens,
/// and nothing of its own — the source heads' ellipsis, as an icon button.
/// Shown or hidden by its row (hidden is `opacity(0)`, its slot kept).
struct RowMenuButton<Items: View>: View {
    /// Its slot: the sidebar's trailing control width, every row's alike,
    /// so the buttons down the tree share one centre.
    var size: CGFloat = RowMenuSlot.tree
    var glyph: CGFloat = 11
    @ViewBuilder let items: () -> Items

    var body: some View {
        Menu {
            items()
        } label: {
            Image(systemName: "ellipsis")
                .font(.system(size: glyph, weight: .medium))
                .ink(.tertiary)
                .frame(width: size, height: size)
                .contentShape(Rectangle())
        }
        .menuStyle(.button)
        .buttonStyle(GnatIconButtonStyle())
        .menuIndicator(.hidden)
        .fixedSize()
        .help("More")
    }
}

enum RowMenuSlot {
    static let tree: CGFloat = GnatMetrics.trailingControl
}
