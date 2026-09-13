import SwiftUI
import NatKit

/// The three content panes' first loads, drawn as the content they are about
/// to be rather than as a spinner in an empty rectangle — `RailSkeletonView`
/// for the rail, and the same bargain: `SkeletonBlock`s at the pane's own
/// geometry, so the arriving brief, diff or pull request lands on a layout
/// that is already correct and nothing shifts under it.
///
/// Each of them draws the rail beside its content too, and its footer under
/// it, because those are exactly the pieces a pane grows once its read lands
/// — a skeleton of the reading column alone would settle the prose and then
/// narrow it the moment the content arrived, which is the shift rather than
/// the fix for it.
///
/// Two rules run through all of it. Every placeholder line is sized off the
/// very type it stands in for — `SkeletonType` carries the real view's own
/// `Typo` size and the row is measured by a hidden run of it, the way
/// `RailView`'s empty note reserves its two lines — rather than off a height
/// picked beside the block, which is how these came to be drawn at 8, 9 and
/// 10 points where the views they replace set 11, 12, 13 and 14. And a piece
/// of chrome that is identical before the read and after it — the brief
/// card's own label and Edit button, the file list's commits menu, the PR
/// column's section labels — is drawn as its real self from the first frame,
/// since a block standing in for something that is not being loaded is a
/// placeholder for its own replacement.
///
/// Only a first load gets one: a re-read keeps whatever is already on screen
/// and admits to itself with `RefreshingMark` instead. `QuietLoadingView` is
/// still what a wait with no shape at all gets.

// MARK: - Shared pieces

/// The type a placeholder line stands in for: the font the real text is set
/// in, which the line's row is measured by, and the point size that font was
/// asked for at, which its block's thickness is derived from.
///
/// A pair rather than a `Font` alone, since a `Font` cannot be asked how big
/// it is — and the whole point of naming the type at all is that the line
/// takes the room the real run of it will take.
struct SkeletonType {
    let font: Font
    let size: CGFloat

    static func system(_ size: CGFloat, weight: Font.Weight = .regular) -> SkeletonType {
        SkeletonType(font: .system(size: size, weight: weight), size: size)
    }

    static func mono(_ size: CGFloat) -> SkeletonType {
        SkeletonType(font: Typo.mono(size: size), size: size)
    }

    /// How thick the block is drawn — the ink of a line rather than its whole
    /// box; see `SkeletonLayout.lineThickness(forTextOf:)`.
    var thickness: CGFloat { SkeletonLayout.lineThickness(forTextOf: size) }
}

/// One placeholder line of prose, taking exactly the room one line of the
/// type it stands in for takes: a hidden run of that very font is what gives
/// the row its height, so the text that replaces it lands on the row the
/// block held rather than a point or two off it.
///
/// Its width is its share of the space its parent has left for it, measured
/// through a `GeometryReader` rather than the `containerRelativeFrame` the
/// rail's own blocks use: a container is the scroll view or the window, and a
/// line inside a card inside a scroll view has fixed insets between it and
/// either of them, so a fraction of the container runs out past the card it
/// is drawn in. The reader sits in an overlay on the sizer rather than around
/// it, which is what has it measure the width this line actually has instead
/// of whatever a greedy reader is proposed.
struct SkeletonTextLine: View {
    let width: Double
    var type: SkeletonType = .system(Typo.body)
    var cornerRadius: CGFloat = 3

    var body: some View {
        Text(" ")
            .font(type.font)
            .hidden()
            .frame(maxWidth: .infinity, alignment: .leading)
            .overlay(alignment: .leading) {
                GeometryReader { proxy in
                    SkeletonBlock(
                        width: SkeletonLayout.lineWidth(width, in: proxy.size.width),
                        height: type.thickness,
                        cornerRadius: cornerRadius
                    )
                    .frame(maxHeight: .infinity, alignment: .center)
                }
            }
    }
}

/// A placeholder for a short run of type whose width is known rather than a
/// share of the row — a line number, a ± tally, a button's label. Its row is
/// the type's own line, measured the way `SkeletonTextLine`'s is, so the
/// block sits where the run's ink will sit instead of at the top of a row it
/// only half fills.
struct SkeletonTextRun: View {
    let width: CGFloat
    var type: SkeletonType = .system(Typo.subhead)
    var cornerRadius: CGFloat = 2

    var body: some View {
        Text(" ")
            .font(type.font)
            .hidden()
            .frame(width: width)
            .overlay {
                SkeletonBlock(width: width, height: type.thickness, cornerRadius: cornerRadius)
            }
    }
}

/// A run of them — one paragraph, one comment's body, one section's values —
/// at the `lineSpacing` the type they stand in for is set at, so the run
/// comes to the height the real paragraph comes to.
struct SkeletonParagraph: View {
    let lines: SkeletonLines
    var type: SkeletonType = .system(Typo.body)
    /// The `lineSpacing` on the real `Text`, which SwiftUI adds between its
    /// lines and nowhere else — hence a `VStack` spacing of exactly it.
    var lineSpacing: CGFloat = 2

    var body: some View {
        VStack(alignment: .leading, spacing: lineSpacing) {
            ForEach(Array(lines.enumerated()), id: \.offset) { _, width in
                SkeletonTextLine(width: width, type: type)
            }
        }
    }
}

/// The busy mark a pane wears while a read runs over content already on
/// screen: a small spinner in a slot that is there whether it is spinning or
/// not, so admitting to the refresh moves nothing beside it. Not the rule
/// `AsyncActionLabel` follows for a button — a button grows by its spinner,
/// because a press is what started the work and the growth says so; nobody
/// pressed anything for a background read.
struct RefreshingMark: View {
    let isRefreshing: Bool

    var body: some View {
        BusySlot(isBusy: isRefreshing, label: "Refreshing…")
    }
}

/// A properties rail's section: its all-caps heading and the rows under it,
/// at the 8pt spacing every real section in either rail uses.
///
/// The heading is drawn as itself — see the note at the top of the file —
/// and only the rows under it are blocks, since what a section says about
/// the slice or the pull request is exactly what is being read.
///
/// `rowHeight` is the PR rail's: its check, review and changes rows are laid
/// out at a fixed 26pt each rather than left to their type, so a section
/// standing in for one reserves that and a section standing in for the
/// brief's plain value lines (nil) reserves the line itself.
private struct SkeletonRailSection: View {
    let section: SkeletonRailSectionShape
    var type: SkeletonType = .system(Typo.subhead)
    var rowHeight: CGFloat?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // The heading itself: which sections either rail has is fixed,
            // so what stands over the rows is the label rather than a block
            // the label replaces.
            Text(section.title)
                .font(.system(size: Typo.subhead, weight: .semibold))
                .ink(.tertiary)

            VStack(alignment: .leading, spacing: 8) {
                ForEach(Array(section.rows.enumerated()), id: \.offset) { _, width in
                    row(width)
                }
            }
        }
    }

    @ViewBuilder
    private func row(_ width: Double) -> some View {
        if let rowHeight {
            SkeletonTextLine(width: width, type: type)
                .frame(height: rowHeight)
        } else {
            SkeletonTextLine(width: width, type: type)
        }
    }
}

/// The rail itself, at the width the pane's own `@AppStorage` holds and with
/// the same leading hairline and insets the real one draws, so the reading
/// column beside it is exactly as wide as it will be when the content lands.
private struct SkeletonRail: View {
    let sections: [SkeletonRailSectionShape]
    let width: Double
    var type: SkeletonType = .system(Typo.subhead)
    var rowHeight: CGFloat?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                ForEach(Array(sections.enumerated()), id: \.offset) { _, section in
                    SkeletonRailSection(section: section, type: type, rowHeight: rowHeight)
                }
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 18)
            .frame(maxWidth: .infinity, alignment: .leading)
            .inelastic()
        }
        .frame(width: width)
        .rule(.separator, edges: [.leading], width: 0.5)
    }
}

/// A pane's footer bar: the diff's and the pull request's are the same band
/// — a hairline, the busy mark's slot, a line of state and the actions — and
/// a pane without it would give the reading column those rows and take them
/// back.
///
/// The mark and the actions are the real ones — the same slot and the same
/// buttons before the read lands as after, and the buttons' own height is
/// what decides how deep the band is, so a block standing in for them would
/// be a placeholder for something already drawable. Only the line between
/// them is a block: what it says is counted off the very reading still in
/// flight.
private struct SkeletonFooter<Actions: View>: View {
    @ViewBuilder let actions: () -> Actions

    var body: some View {
        VStack(spacing: 0) {
            Divider().frame(height: 0.5)

            HStack(spacing: 8) {
                RefreshingMark(isRefreshing: true)

                // Greedy, so it is the `Spacer` the loaded footer puts
                // between its line and its actions as well as the line.
                SkeletonTextLine(width: 0.24, type: .system(Typo.subhead))

                actions()
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
        }
    }
}

// MARK: - Brief

/// The Brief tab's first load: the brief card's prose inside the very card
/// the brief is drawn in, and the properties rail beside it — which the real
/// pane draws only once there is a detail to read it off, so leaving it out
/// here would narrow the reading column the moment the brief landed.
///
/// The card's own header is `BriefCardHeader` itself rather than blocks
/// standing in for it: the label and the Edit button read the same before
/// the brief lands and after, and the button is disabled either way while
/// there is no detail to edit.
struct BriefSkeletonView: View {
    /// The rail's width, read from the same key `BriefTabView` writes, so
    /// the skeleton's rail is exactly the one that replaces it.
    @AppStorage("briefSidebarWidth") private var sidebarWidth = 216.0

    var body: some View {
        HStack(spacing: 0) {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    VStack(alignment: .leading, spacing: 16) {
                        BriefCardHeader()

                        // The brief's own prose, at the body type and the
                        // 2pt leading `BriefTabView` renders its markdown
                        // with, inside the 8pt-spaced stack that holds it.
                        VStack(alignment: .leading, spacing: 8) {
                            ForEach(Array(BriefSkeleton.paragraphs.enumerated()), id: \.offset) { _, paragraph in
                                SkeletonParagraph(lines: paragraph)
                            }
                        }
                    }
                    .padding(20)
                    .card(radius: 10, border: .hairline)

                    Spacer()
                }
                .padding(.horizontal, 22)
                .padding(.vertical, 18)
                .frame(maxWidth: 640)
                .inelastic()
            }

            SkeletonRail(sections: BriefSkeleton.sidebarSections, width: sidebarWidth)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(BriefSkeleton.accessibilityLabel)
    }
}

// MARK: - Diff

/// The Diff tab's first load: file boxes at `DiffFileBoxView`'s own geometry
/// beside the file list, over the footer the loaded pane draws.
///
/// The list's commits menu is the real `DiffCommitsMenu` — with no commits
/// read yet, which is exactly what it says while the read is in flight —
/// since it is the same control before the diff lands and after.
struct DiffSkeletonView: View {
    /// Whether the slice is still waiting on an approval, which is the one
    /// thing the footer's actions differ by — the loaded footer reads it off
    /// the very same slice.
    var handedBack = false

    @AppStorage("diffSidebarWidth") private var sidebarWidth = 232.0

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                ScrollView {
                    VStack(alignment: .leading, spacing: 10) {
                        ForEach(Array(DiffSkeleton.files.enumerated()), id: \.offset) { _, file in
                            fileBox(file)
                        }
                    }
                    .padding(14)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .inelastic()
                }

                sidebar
            }

            SkeletonFooter {
                DiffFooterActions(showApprove: handedBack)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(DiffSkeleton.accessibilityLabel)
    }

    /// The file list: `DiffFileSidebarView`'s own scroll, spacing and inset,
    /// its real commits menu, and one placeholder per file at the 28pt every
    /// `DiffFileSidebarRow` is drawn at.
    private var sidebar: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 2) {
                DiffCommitsMenu(commits: [], selectedCommit: nil, onSelectCommit: { _ in })

                ForEach(Array(DiffSkeleton.sidebarRows.enumerated()), id: \.offset) { _, width in
                    HStack(spacing: DiffFileSidebarRow.spacing) {
                        // The viewed tick's own slot, which every row holds
                        // whether it is ticked or not.
                        Color.clear.frame(width: DiffFileSidebarRow.tickWidth)

                        SkeletonTextLine(width: width, type: .system(Typo.subhead))
                    }
                    .padding(.horizontal, 8)
                    .frame(height: DiffFileSidebarRow.height)
                }
            }
            .padding(8)
            .frame(maxWidth: .infinity, alignment: .leading)
            .inelastic()
        }
        .frame(width: sidebarWidth)
        .rule(.separator, edges: [.leading], width: 0.5)
    }

    /// One file's box, at `DiffFileBoxView`'s own shape: its 32pt header row
    /// over the file's rows, each of them the gutter, the +/- slot and the
    /// code at the 19pt a single-line row comes to.
    private func fileBox(_ file: DiffSkeletonFile) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            header(file)

            Divider().frame(height: 0.5)

            VStack(alignment: .leading, spacing: 0) {
                ForEach(Array(file.rows.enumerated()), id: \.offset) { _, width in
                    row(width)
                }
            }
        }
        .card(radius: 10)
    }

    private func header(_ file: DiffSkeletonFile) -> some View {
        HStack(spacing: DiffFileBoxView.headerSpacing) {
            // The fold chevron's slot and the path beside it, at the code
            // face the real header sets a path in.
            SkeletonBlock(width: 12, height: 12, cornerRadius: 2)

            SkeletonTextLine(width: file.pathWidth, type: .mono(Typo.code))

            // The ± tally, and the Viewed button whose own height is what
            // holds the header open at 32.
            SkeletonTextRun(width: 44)

            SkeletonTextRun(width: 42)
                .frame(height: ButtonMetrics.height)
        }
        .padding(.horizontal, 12)
        .frame(height: DiffFileBoxView.headerHeight)
        .surface(.rowAlt)
    }

    private func row(_ width: Double) -> some View {
        let numberColumn = DiffRowView.numberColumnWidth(DiffSkeleton.numberWidth)
        return HStack(alignment: .top, spacing: 0) {
            // The gutter: the two number columns, their gap and their inset,
            // exactly as `DiffRowView` lays them out.
            HStack(spacing: 6) {
                SkeletonTextRun(width: numberColumn, type: .mono(Typo.code))
                SkeletonTextRun(width: numberColumn, type: .mono(Typo.code))
            }
            .padding(.horizontal, 8)

            // The +/- slot, which is a column of its own whether the row has
            // a glyph in it or not.
            Color.clear
                .frame(width: 13)
                .padding(.leading, 12)

            SkeletonTextLine(width: width, type: .mono(Typo.code))
        }
        .padding(.vertical, 1.5)
        .frame(minHeight: DiffRowView.minimumRowHeight)
    }
}

// MARK: - PR

/// The PR tab's first load: the state chip and title, the branch line, the
/// description and a conversation, beside the checks/review/changes rail and
/// over the composer and footer the loaded pane draws.
///
/// The column's own section labels are the real ones: `DESCRIPTION` and
/// `CONVERSATION` head those sections whatever GitHub comes back with.
struct PRSkeletonView: View {
    @AppStorage("prSidebarWidth") private var sidebarWidth = 216.0

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                VStack(spacing: 0) {
                    ScrollView {
                        VStack(alignment: .leading, spacing: 16) {
                            header

                            // `head → base`, in the code face the real line
                            // is set in.
                            SkeletonTextLine(width: PRSkeleton.branchLineWidth, type: .mono(Typo.code))

                            description

                            conversation

                            Spacer(minLength: 0)
                        }
                        .padding(.horizontal, 22)
                        .padding(.vertical, 18)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .inelastic()
                    }

                    Divider().frame(height: 0.5)

                    // The composer pinned at the tab's foot.
                    composer(compact: false)
                        .padding(.horizontal, 22)
                        .padding(.vertical, 12)
                }
                .frame(maxWidth: .infinity)

                SkeletonRail(
                    sections: PRSkeleton.sidebarSections,
                    width: sidebarWidth,
                    type: .mono(Typo.code - 1),
                    rowHeight: PRSidebarView.rowHeight
                )
            }

            SkeletonFooter {
                PRFooterActions()
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(PRSkeleton.accessibilityLabel)
    }

    /// The state chip and the title beside it, at the real header's spacing
    /// — the chip a capsule of the very height the real one is pinned to.
    private var header: some View {
        HStack(spacing: 10) {
            SkeletonBlock(width: 74, height: 22, cornerRadius: 11)

            SkeletonTextLine(width: PRSkeleton.titleWidth, type: .system(Typo.headline, weight: .semibold))
        }
    }

    private var description: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("DESCRIPTION")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .ink(.tertiary)

            SkeletonParagraph(lines: PRSkeleton.descriptionLines)
        }
    }

    private var conversation: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("CONVERSATION")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .ink(.tertiary)

            VStack(alignment: .leading, spacing: 12) {
                ForEach(Array(PRSkeleton.conversationEntries.enumerated()), id: \.offset) { _, entry in
                    entryRow(entry)
                }

                // The thread's own composer, which the loaded card ends with.
                composer(compact: true)
            }
            .padding(12)
            .card(radius: 10)
        }
    }

    /// The composer's band, reserved as the real box lays out rather than at
    /// a number of its own: its editor is flexible between a floor and a
    /// ceiling, and the one at the tab's foot opens at that ceiling wherever
    /// the pane has the room — a block at the floor alone would hand the
    /// reading column rows the composer then took back.
    private func composer(compact: Bool) -> some View {
        SkeletonBlock(cornerRadius: PRComposerMetrics.cornerRadius)
            .frame(
                minHeight: PRComposerMetrics.height(compact: compact),
                maxHeight: PRComposerMetrics.maxHeight(compact: compact)
            )
    }

    /// One entry of it, at `PRConversationEntryView`'s own avatar, gap and
    /// stack: the byline over whatever was said.
    private func entryRow(_ entry: SkeletonLines) -> some View {
        HStack(alignment: .top, spacing: PRConversationEntryView.avatarGap) {
            SkeletonBlock(
                width: PRConversationEntryView.avatarSize,
                height: PRConversationEntryView.avatarSize,
                cornerRadius: PRConversationEntryView.avatarSize / 2
            )
            .padding(.top, 2)

            VStack(alignment: .leading, spacing: 3) {
                SkeletonTextLine(width: 0.3, type: .system(Typo.subhead, weight: .semibold))
                SkeletonParagraph(lines: entry, type: .system(Typo.subhead))
            }
        }
    }
}

#Preview("Brief") {
    BriefSkeletonView()
        .frame(width: 900, height: 560)
        .surface(.window)
}

#Preview("Diff") {
    DiffSkeletonView(handedBack: true)
        .frame(width: 900, height: 560)
        .surface(.window)
}

#Preview("PR") {
    PRSkeletonView()
        .frame(width: 900, height: 560)
        .surface(.window)
}
