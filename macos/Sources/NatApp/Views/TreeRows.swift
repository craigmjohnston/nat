import SwiftUI
import NatKit

/// What names a piece of work as the sidebar's Active fold does: its state
/// dot (or, for a workshop, its `symbol` in the dot's ink), the project's
/// short tag, then its title. The Active rows and the navigator's titlebar
/// both draw it, so the two read alike.
struct ActiveIdentityLabel: View {
    @Environment(\.ground) private var ground
    let tag: String
    let state: SliceDisplayState
    let live: Bool
    let title: String
    var symbol: String?
    var size: CGFloat = GnatMetrics.body
    var titleInk: InkRole = .secondary

    var body: some View {
        HStack(spacing: 6) {
            Group {
                if let symbol {
                    StateSymbol(symbol: symbol, state: state, live: live)
                } else {
                    StateDot(state: state, live: live)
                }
            }
            .frame(width: GnatMetrics.treeGlyphColumn)
            (identityTag(tag, on: ground) + Text(title))
                .font(.system(size: size))
                .ink(titleInk)
                .lineLimit(1)
        }
    }
}

/// An identity's project tag and the gap after it, ahead of its title — or
/// nothing at all for no tag (the breadcrumb's last crumb, whose project
/// crumb already names the project).
func identityTag(_ tag: String, on ground: Ground) -> Text {
    guard !tag.isEmpty else { return Text("") }
    return Text(tag)
        .font(Typo.mono(size: Typo.scaled(10), weight: .medium))
        .tracking(1)
        // Raised off the shared baseline so the small capitals sit
        // on the title's middle rather than its foot.
        .baselineOffset(1.5)
        .foregroundStyle(DesignTokens.ink(.secondary, on: ground))
        + Text("  \u{2009}")
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

    var body: some View {
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
            Text(count).monoXS().ink(isDone ? .quaternary : .tertiary)
        }
        .padding(.leading, indent)
        .padding(.trailing, 10)
        .frame(height: GnatMetrics.sidebarRowHeight)
        .gnatRow(hoverable: folds)
        .onHover { if folds { hovering = $0 } }
    }
}

/// A slice line of a plan tree, as the sidebar draws it: its dot and title.
struct TreeSliceLine: View {
    let title: String
    let state: SliceDisplayState
    var live = false
    var selected = false
    var indent: CGFloat = 34
    /// Its pull request's marks, at the trailing edge.
    var marks: PRMarks = .none

    var body: some View {
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
        }
        .opacity(state == .done ? 0.7 : 1)
        .padding(.leading, indent)
        .padding(.trailing, 10)
        .frame(height: GnatMetrics.sidebarRowHeight)
        .gnatRow(selected: selected)
        .contentShape(Rectangle())
    }
}
