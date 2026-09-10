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
/// Only a first load gets one: a re-read keeps whatever is already on screen
/// and admits to itself with `RefreshingMark` instead. `QuietLoadingView` is
/// still what a wait with no shape at all gets.

// MARK: - Shared pieces

/// One placeholder line of prose at its share of the space its parent has
/// left for it, measured through a `GeometryReader` rather than the
/// `containerRelativeFrame` the rail's own blocks use: a container is the
/// scroll view or the window, and a line inside a card inside a scroll view
/// has fixed insets between it and either of them, so a fraction of the
/// container runs out past the card it is drawn in. The reader measures what
/// this line actually has.
///
/// It is greedy horizontally and sized to the line's own height vertically,
/// so it takes the room a real line of type would take and no more.
struct SkeletonTextLine: View {
    let width: Double
    var height: CGFloat = 9
    var cornerRadius: CGFloat = 3

    var body: some View {
        GeometryReader { proxy in
            SkeletonBlock(
                width: SkeletonLayout.lineWidth(width, in: proxy.size.width),
                height: height,
                cornerRadius: cornerRadius
            )
        }
        .frame(height: height)
    }
}

/// A run of them — one paragraph, one comment's body, one section's values —
/// at the line spacing the type they stand in for is set at. One reader for
/// the paragraph rather than one per line, since every line of it is a
/// fraction of the same width.
struct SkeletonParagraph: View {
    let lines: SkeletonLines
    var height: CGFloat = 9
    var spacing: CGFloat = 8

    var body: some View {
        GeometryReader { proxy in
            VStack(alignment: .leading, spacing: spacing) {
                ForEach(Array(lines.enumerated()), id: \.offset) { _, width in
                    SkeletonBlock(
                        width: SkeletonLayout.lineWidth(width, in: proxy.size.width),
                        height: height
                    )
                }
            }
        }
        .frame(height: SkeletonLayout.paragraphHeight(lines.count, height: height, spacing: spacing))
    }
}

/// The busy mark a pane wears while a read runs over content already on
/// screen: a small spinner in a slot that is there whether it is spinning or
/// not, so admitting to the refresh moves nothing beside it. The same rule
/// `AsyncActionLabel` follows for a button, and for the same reason.
struct RefreshingMark: View {
    let isRefreshing: Bool

    var body: some View {
        BusySlot(isBusy: isRefreshing, label: "Refreshing…")
    }
}

/// A properties rail's section: its all-caps heading and the value lines
/// under it, at the 8pt spacing every real section in either rail uses.
private struct SkeletonRailSection: View {
    let lines: SkeletonLines

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            SkeletonTextLine(width: 0.42, height: 8, cornerRadius: 2)
            SkeletonParagraph(lines: lines, height: 9, spacing: 8)
        }
    }
}

/// The rail itself, at the width the pane's own `@AppStorage` holds and with
/// the same leading hairline the real one draws, so the reading column
/// beside it is exactly as wide as it will be when the content lands.
private struct SkeletonRail: View {
    let sections: [SkeletonLines]
    let width: Double

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            ForEach(Array(sections.enumerated()), id: \.offset) { _, section in
                SkeletonRailSection(lines: section)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 18)
        .frame(width: width)
        .frame(maxHeight: .infinity, alignment: .top)
        .rectBorder(width: 0.5, edges: [.leading], color: DesignTokens.separator)
    }
}

/// A pane's footer bar, drawn as the hairline and the 32pt-ish band it will
/// be: the diff's and the pull request's both grow one as their content
/// lands, and a pane without it would give the reading column those rows and
/// take them back.
private struct SkeletonFooter: View {
    var body: some View {
        VStack(spacing: 0) {
            Divider().frame(height: 0.5)

            HStack(spacing: 8) {
                SkeletonBlock(width: 140, height: 9)
                Spacer(minLength: 0)
                SkeletonBlock(width: 92, height: 20, cornerRadius: 6)
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
struct BriefSkeletonView: View {
    /// The rail's width, read from the same key `BriefTabView` writes, so
    /// the skeleton's rail is exactly the one that replaces it.
    @AppStorage("briefSidebarWidth") private var sidebarWidth = 216.0

    var body: some View {
        HStack(spacing: 0) {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    VStack(alignment: .leading, spacing: 16) {
                        // The card's own "Brief" label row and its Edit
                        // button, at the height the real pair comes to.
                        HStack {
                            SkeletonBlock(width: 34, height: 9)
                            Spacer(minLength: 0)
                            SkeletonBlock(width: 38, height: 9)
                        }
                        .frame(height: 16)

                        ForEach(Array(BriefSkeleton.paragraphs.enumerated()), id: \.offset) { _, paragraph in
                            SkeletonParagraph(lines: paragraph, height: 10, spacing: 9)
                        }
                    }
                    .padding(20)
                    .background(
                        RoundedRectangle(cornerRadius: 10)
                            .fill(DesignTokens.controlBg)
                    )
                    .overlay(
                        RoundedRectangle(cornerRadius: 10)
                            .stroke(DesignTokens.hairline, lineWidth: 1)
                    )

                    Spacer()
                }
                .padding(.horizontal, 22)
                .padding(.vertical, 18)
                .frame(maxWidth: 640)
            }

            SkeletonRail(sections: BriefSkeleton.sidebarSections.map { [$0] }, width: sidebarWidth)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(BriefSkeleton.accessibilityLabel)
    }
}

// MARK: - Diff

/// The Diff tab's first load: file boxes at the real box geometry — a 32pt
/// header row over a run of code lines — beside the file list, over the
/// footer the loaded pane draws.
struct DiffSkeletonView: View {
    @AppStorage("diffSidebarWidth") private var sidebarWidth = 232.0

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                ScrollView {
                    VStack(alignment: .leading, spacing: 10) {
                        ForEach(Array(DiffSkeleton.files.enumerated()), id: \.offset) { _, file in
                            fileBox(file)
                        }
                        Spacer(minLength: 0)
                    }
                    .padding(14)
                }

                VStack(alignment: .leading, spacing: 2) {
                    // The "All commits" menu the file list opens with.
                    SkeletonBlock(height: 24, cornerRadius: 6)
                        .padding(.bottom, 4)

                    ForEach(Array(DiffSkeleton.sidebarRows.enumerated()), id: \.offset) { _, width in
                        SkeletonTextLine(width: width, height: 9)
                            .frame(height: 22, alignment: .center)
                    }
                    Spacer(minLength: 0)
                }
                .padding(8)
                .frame(width: sidebarWidth)
                .frame(maxHeight: .infinity, alignment: .top)
                .rectBorder(width: 0.5, edges: [.leading], color: DesignTokens.separator)
            }

            SkeletonFooter()
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(DiffSkeleton.accessibilityLabel)
    }

    /// One file's box: `DiffFileBoxView`'s own 10pt corner, hairline border
    /// and 32pt header row, with the code lines at the 19pt each real row
    /// comes to.
    private func fileBox(_ file: DiffSkeletonFile) -> some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                SkeletonBlock(width: 14, height: 12, cornerRadius: 2)
                SkeletonTextLine(width: file.pathWidth, height: 9)
                Spacer(minLength: 0)
                SkeletonBlock(width: 52, height: 9)
            }
            .padding(.horizontal, 12)
            .frame(height: 32)

            Divider().frame(height: 0.5)

            VStack(alignment: .leading, spacing: 0) {
                ForEach(Array(file.rows.enumerated()), id: \.offset) { _, width in
                    HStack(spacing: 12) {
                        SkeletonBlock(width: 22, height: 8, cornerRadius: 2)
                        SkeletonTextLine(width: width, height: 8)
                        Spacer(minLength: 0)
                    }
                    .padding(.horizontal, 8)
                    .frame(height: 19)
                }
            }
            .padding(.vertical, 4)
        }
        .background(DesignTokens.controlBg)
        .clipShape(RoundedRectangle(cornerRadius: 10))
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(DesignTokens.hairline, lineWidth: 1)
        )
    }
}

// MARK: - PR

/// The PR tab's first load: the state chip and title, the branch line, the
/// description and a conversation, beside the checks/review/changes rail and
/// over the composer and footer the loaded pane draws.
struct PRSkeletonView: View {
    @AppStorage("prSidebarWidth") private var sidebarWidth = 216.0

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                VStack(spacing: 0) {
                    ScrollView {
                        VStack(alignment: .leading, spacing: 16) {
                            HStack(spacing: 8) {
                                SkeletonBlock(width: 58, height: 18, cornerRadius: 9)
                                SkeletonTextLine(width: PRSkeleton.titleWidth, height: 13)
                                Spacer(minLength: 0)
                            }

                            SkeletonTextLine(width: PRSkeleton.branchLineWidth, height: 9)

                            SkeletonParagraph(lines: PRSkeleton.descriptionLines, height: 10, spacing: 9)

                            ForEach(Array(PRSkeleton.conversationEntries.enumerated()), id: \.offset) { _, entry in
                                HStack(alignment: .top, spacing: 8) {
                                    SkeletonBlock(width: 20, height: 20, cornerRadius: 10)
                                    SkeletonParagraph(lines: entry, height: 9, spacing: 8)
                                }
                            }

                            Spacer(minLength: 0)
                        }
                        .padding(.horizontal, 22)
                        .padding(.vertical, 18)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }

                    Divider().frame(height: 0.5)

                    // The composer pinned at the tab's foot: 36pt of editor
                    // over its send row, which is the height the real one
                    // opens at.
                    SkeletonBlock(height: 76, cornerRadius: 8)
                        .padding(.horizontal, 22)
                        .padding(.vertical, 12)
                }
                .frame(maxWidth: .infinity)

                SkeletonRail(sections: PRSkeleton.sidebarSections, width: sidebarWidth)
            }

            SkeletonFooter()
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(PRSkeleton.accessibilityLabel)
    }
}

#Preview("Brief") {
    BriefSkeletonView()
        .frame(width: 900, height: 560)
        .background(DesignTokens.windowBg)
}

#Preview("Diff") {
    DiffSkeletonView()
        .frame(width: 900, height: 560)
        .background(DesignTokens.windowBg)
}

#Preview("PR") {
    PRSkeletonView()
        .frame(width: 900, height: 560)
        .background(DesignTokens.windowBg)
}
