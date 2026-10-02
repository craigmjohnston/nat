import AppKit
import SwiftUI
import NatKit

/// The visible rows' vertical extents in the diff scroll view's own space,
/// gathered up to `DiffTabView` to feed `DiffScrollAnchor`.
struct DiffRowFramesKey: PreferenceKey {
    static let space = "diffScroll"
    static let defaultValue: [DiffScrollAnchor.Key: ClosedRange<CGFloat>] = [:]
    static func reduce(
        value: inout [DiffScrollAnchor.Key: ClosedRange<CGFloat>],
        nextValue: () -> [DiffScrollAnchor.Key: ClosedRange<CGFloat>]
    ) {
        value.merge(nextValue()) { _, new in new }
    }
}

/// One file's diff, drawn GitHub-fashion: a rounded, hairline-bordered box
/// with a header row (chevron, path, ± tally, viewed toggle) and the file's
/// body rows beneath it. A collapsed file is the header row alone, ticked —
/// the fold is this view's own state, held by `DiffStore` and cleared by a
/// re-read, never written anywhere.
///
/// Comments are drawn in place, under the last row they cover: a pending
/// comment as a card (avatar, "Pending" badge, edit/delete), and a comment
/// being written or edited as an inline text editor. Both are display rows
/// the line cursor concept has no notion of — this view is purely a
/// renderer, forwarding every click back to `DiffTabView`, which is the one
/// place selection, drafting and the pending review actually live.
struct DiffFileBoxView: View {
    /// The header row's own geometry, named rather than inline so
    /// `DiffSkeletonView` reserves exactly the row this draws — its height is
    /// what holds the box open over the "Viewed" button inside it.
    static let headerSpacing: CGFloat = 10
    static let headerHeight: CGFloat = 28

    let file: DiffFileModel
    let numberWidth: Int
    let isViewed: Bool
    let isCollapsed: Bool
    let comments: [PendingComment]
    let selection: DiffSelection?
    let draft: CommentDraft?
    /// Whether a new comment can be started here at all — false while a
    /// single commit's own diff is on screen, since a comment is about the
    /// branch's diff (`DiffStore.commentsEditable`). Existing pending
    /// comments still draw (a comment left in "All commits" mode is still
    /// there to look at); only starting a new one is what this gates.
    var commentsEnabled: Bool = true
    let authorName: String
    let authorInitials: String
    let onToggleViewed: () -> Void
    let onToggleCollapsed: () -> Void
    let onRowClick: (DiffRow, Bool) -> Void
    let onOpenCommentEditor: () -> Void
    let onEditComment: (PendingComment) -> Void
    let onDeleteComment: (PendingComment) -> Void
    let onSaveDraft: (String) -> Void
    let onCancelDraft: () -> Void

    /// Where a comment card (or the editor) starts: at the gutter's far edge
    /// — its hairline included — so the card lines up with the code area
    /// rather than the box.
    private var commentLeadingInset: CGFloat {
        DiffRowView.gutterWidth(numberWidth: numberWidth) + 0.5
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            DiffFileHeaderView(
                file: file, isViewed: isViewed, isCollapsed: isCollapsed, commentCount: comments.count,
                showsViewed: true, onToggleViewed: onToggleViewed, onToggleCollapsed: onToggleCollapsed)
            if !isCollapsed {
                rows
            }
        }
    }

    /// The file's body rows, with any comment drawn under the last row it
    /// covers — what the main pane lays under each pinned header.
    var rows: some View {
        let commentsByAnchor = Dictionary(grouping: comments) { $0.anchorRowIDs.last }
        return LazyVStack(alignment: .leading, spacing: 0) {
            ForEach(file.rows) { row in
                DiffRowView(
                    row: row,
                    numberWidth: numberWidth,
                    isSelected: selection?.rowIDs.contains(row.id) ?? false,
                    canComment: commentsEnabled && draft == nil && row.kind != .hunkBreak,
                    isSelectionEnd: selection?.rowIDs.last == row.id,
                    onSelect: { shift in onRowClick(row, shift) },
                    onComment: { endsSelection in
                        // A hovered row that is not the end of the marked run
                        // is marked first, so the comment is about it.
                        if !endsSelection { onRowClick(row, false) }
                        onOpenCommentEditor()
                    }
                )
                // Each realised row reports where it sits in the scroll
                // view, and is a scroll target under a path-qualified id —
                // what `DiffScrollAnchor` keeps through a width change.
                .id(DiffScrollAnchor.Key(path: file.path, rowID: row.id).scrollID)
                .background(GeometryReader { proxy in
                    let frame = proxy.frame(in: .named(DiffRowFramesKey.space))
                    Color.clear.preference(
                        key: DiffRowFramesKey.self,
                        value: [DiffScrollAnchor.Key(path: file.path, rowID: row.id): frame.minY...frame.maxY]
                    )
                })

                if let draft, draft.anchorRowIDs.last == row.id {
                    CommentEditorView(initialText: draft.text, onSave: onSaveDraft, onCancel: onCancelDraft)
                        .padding(.leading, commentLeadingInset)
                        .padding(.trailing, 16)
                        .padding(.vertical, 8)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }

                ForEach(commentsByAnchor[row.id] ?? []) { comment in
                    PendingCommentCardView(
                        comment: comment,
                        authorName: authorName,
                        authorInitials: authorInitials,
                        onEdit: { onEditComment(comment) },
                        onDelete: { onDeleteComment(comment) }
                    )
                    .padding(.leading, commentLeadingInset)
                    .padding(.trailing, 16)
                    .padding(.top, 4)
                    .padding(.bottom, 8)
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
        .scrollTargetLayout()
    }
}

/// A file's header in the continuous diff: the `--sel` band with a line
/// above and below, the fold chevron, the path, its tally, and — on a review
/// — the viewed mark at the trailing edge. Pinned by the main pane, so the
/// file a row belongs to is always named above it.
struct DiffFileHeaderView: View {
    let file: DiffFileModel
    let isViewed: Bool
    let isCollapsed: Bool
    var commentCount: Int = 0
    var showsViewed: Bool = true
    /// Whether the band draws its top line — not for the first file, whose
    /// top is the titlebar's own line, as the design's first header has none.
    var showsTopRule: Bool = true
    let onToggleViewed: () -> Void
    let onToggleCollapsed: () -> Void

    var body: some View {
        HStack(spacing: 8) {
            DisclosureChevron(open: !isCollapsed)
            Text(file.path)
                .font(Typo.mono(size: Typo.code, weight: .medium))
                .ink(.primary)
                .lineLimit(1)
                .truncationMode(.head)
            if file.isRenamed {
                Text("was \(file.oldPath)").monoXS().ink(.secondary).lineLimit(1)
            }
            Text(tally).monoXS().ink(.secondary)
            if commentCount > 0 {
                Image(systemName: "text.bubble.fill")
                    .font(.system(size: 10))
                    .ink(.secondary)
                    .help("Pending comments")
            }
            Spacer(minLength: 0)
            if showsViewed {
                Button(action: onToggleViewed) {
                    HStack(spacing: 6) {
                        ViewedCheckbox(checked: isViewed)
                        Text("viewed").monoXS().ink(isViewed ? .primary : .secondary)
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .help(isViewed ? "Mark not viewed" : "Mark viewed")
            }
        }
        .padding(.leading, 10)
        .padding(.trailing, 16)
        .frame(height: DiffFileBoxView.headerHeight)
        .background(DesignTokens.rowWash(selected: false, on: .window))
        .overlay(alignment: .top) {
            if showsTopRule { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
        }
        .overlay(alignment: .bottom) { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
        .contentShape(Rectangle())
        .onTapGesture(perform: onToggleCollapsed)
    }

    private var tally: String {
        "+\(file.adds) \u{2212}\(file.dels)"
    }
}

/// One row of a file's body: the gutter (old/new line numbers), the +/-
/// glyph, and the row's text. `hunkBreak` draws a dashed separator with the
/// hunk header in tertiary instead, and takes no click at all — it stands
/// for a gap in the file, not a line of it. Every other row can be clicked to
/// mark it (or, shift-clicked, to extend the marked range to it) as where a
/// comment would go; the trailing "+"-bubble button only ever appears on the
/// last row of that range, and only while nothing is already being written
/// about it.
struct DiffRowView: View {
    @Environment(\.ground) private var ground
    let row: DiffRow
    let numberWidth: Int
    let isSelected: Bool
    /// Whether a comment can be started on this row at all.
    let canComment: Bool
    /// Whether the row ends the marked run — where a comment on the run goes.
    let isSelectionEnd: Bool
    let onSelect: (Bool) -> Void
    /// Opens the comment editor; told whether the row already ends the run.
    let onComment: (Bool) -> Void
    @State private var hovering = false

    /// The comment button shows on the row under the pointer, and on the end
    /// of a marked run, so a line can be commented on without clicking it
    /// first.
    private var showCommentButton: Bool {
        canComment && (hovering || isSelectionEnd)
    }

    // The per-character width of the gutter's monospaced digits at
    // Typo.code — scaled up from the 7.5pt this was calibrated at when
    // the gutter still rendered at 11pt, so the column stays exactly as
    // wide as the numbers it holds now render at 13pt.
    static func numberColumnWidth(_ numberWidth: Int) -> CGFloat {
        CGFloat(numberWidth) * 8.9 + 4
    }

    /// The gutter's full width — both number columns, their gap and the
    /// horizontal padding — shared by the numbers, the fill drawn behind
    /// them, and the hunk-break cell, so the three can never drift apart.
    /// Static so the file box can start a comment card exactly where the
    /// gutter ends.
    static func gutterWidth(numberWidth: Int) -> CGFloat {
        numberColumnWidth(numberWidth) * 2 + 6 + 16
    }

    private var numberColumnWidth: CGFloat {
        Self.numberColumnWidth(numberWidth)
    }

    private var gutterWidth: CGFloat {
        Self.gutterWidth(numberWidth: numberWidth)
    }

    var body: some View {
        switch row.kind {
        case .hunkBreak:
            hunkBreakRow
        default:
            contentRow
        }
    }

    private var hunkBreakRow: some View {
        Text(row.text)
            .font(Typo.mono(size: GnatMetrics.xs))
            .ink(.tertiary)
            .lineLimit(1)
            .padding(.leading, gutterWidth + 25)
            .padding(.trailing, 16)
            .frame(minHeight: 21, alignment: .leading)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// What a row of a file's body is at least — one line of code, its
    /// vertical padding included. Named rather than inline for the reason the
    /// header's height is: `DiffSkeletonView` stands in for these rows.
    static let minimumRowHeight: CGFloat = 21

    // Top-aligned, not centred: a long line wraps, and everything that
    // belongs to the line as a whole — its numbers, its +/- — belongs on the
    // first of its rows, not floating in the middle of them. The 1.5pt
    // vertical padding is what keeps a single-line row at the same 19pt it
    // was when it was centred.
    private var contentRow: some View {
        HStack(alignment: .top, spacing: 0) {
            HStack(spacing: 6) {
                Text(row.oldNumber.map(String.init) ?? "")
                    .frame(width: numberColumnWidth, alignment: .trailing)
                Text(row.newNumber.map(String.init) ?? "")
                    .frame(width: numberColumnWidth, alignment: .trailing)
            }
            .font(Typo.mono(size: Typo.code, weight: .regular))
            .ink(.tertiary)
            .padding(.horizontal, 8)

            Text(glyph)
                .font(Typo.mono(size: Typo.code, weight: .regular))
                .foregroundStyle(glyphColor)
                .frame(width: 13)
                .padding(.leading, 12)

            Text(row.styledText)
                .font(Typo.mono(size: Typo.code, weight: .regular))
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.vertical, 1)
        .frame(minHeight: Self.minimumRowHeight)
        // Over the line rather than beside it, on a face of its own, so
        // showing it never rewraps the code under the pointer.
        .overlay(alignment: .topTrailing) {
            if showCommentButton {
                Button(action: { onComment(isSelectionEnd) }) {
                    Image(systemName: "plus.bubble")
                        .font(.system(size: 12, weight: .medium))
                        .ink(.accent)
                        .frame(width: 24, height: Self.minimumRowHeight - 2)
                        .control(radius: 4)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .padding(.top, 1)
                .padding(.trailing, 8)
                .help("Comment on this line")
            }
        }
        .background(rowFill)
        .background(isSelected ? DesignTokens.wash(.selection, tone: .accent, on: ground) : Color.clear)
        .contentShape(Rectangle())
        .onHover { hovering = $0 }
        .onTapGesture {
            onSelect(NSEvent.modifierFlags.contains(.shift))
        }
    }

    private var glyph: String {
        switch row.prefix {
        case "+": return "+"
        case "-": return "\u{2212}"
        default: return " "
        }
    }

    private var glyphColor: Color {
        switch row.kind {
        case .added: return DesignTokens.ink(.success, on: ground)
        case .removed: return DesignTokens.ink(.danger, on: ground)
        default: return .clear
        }
    }

    private var rowFill: Color {
        switch row.kind {
        case .added: return DesignTokens.diffAddedRowBg(on: ground)
        case .removed: return DesignTokens.diffRemovedRowBg(on: ground)
        default: return .clear
        }
    }

}

/// A pending comment, drawn as a card right under the last line it covers:
/// an avatar circle (the configured user's initials), their name, a yellow
/// "Pending" badge — every comment here is, since none of them are written
/// anywhere until they are sent — and the edit/delete icons the mock shows.
struct PendingCommentCardView: View {
    @Environment(\.ground) private var ground
    let comment: PendingComment
    let authorName: String
    let authorInitials: String
    let onEdit: () -> Void
    let onDelete: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 8) {
                Text("\(authorName.lowercased()) · pending").monoXS().ink(.secondary)
                Spacer(minLength: 0)
                Button(action: onEdit) {
                    Image(systemName: "pencil").font(.system(size: 11)).ink(.tertiary)
                        .frame(width: 18, height: 18)
                }
                .buttonStyle(GnatIconButtonStyle())
                .help("Edit this comment")
                Button(action: onDelete) {
                    Image(systemName: "trash").font(.system(size: 11)).ink(.tertiary)
                        .frame(width: 18, height: 18)
                }
                .buttonStyle(GnatIconButtonStyle())
                .help("Delete this comment")
            }
            Text(comment.text)
                .font(.system(size: 13.5))
                .lineSpacing(2)
                .ink(.primary)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 7)
        .surface(.chrome, radius: 4)
        .overlay {
            RoundedRectangle(cornerRadius: 4).strokeBorder(DesignTokens.rule(.border, on: .window), lineWidth: 1)
        }
    }
}

/// The inline editor a comment (new or reopened) is written in: a bordered
/// text box and Cancel/Comment buttons — "Comment" rather than "Save", since
/// what it does is leave one, and disabled on empty text the same way an
/// empty box is how the Go TUI takes a comment back rather than leaving one
/// at all.
struct CommentEditorView: View {
    @State private var text: String
    let onSave: (String) -> Void
    let onCancel: () -> Void

    init(initialText: String, onSave: @escaping (String) -> Void, onCancel: @escaping () -> Void) {
        _text = State(initialValue: initialText)
        self.onSave = onSave
        self.onCancel = onCancel
    }

    var body: some View {
        VStack(alignment: .trailing, spacing: 8) {
            TextEditor(text: $text)
                .font(Typo.mono(size: Typo.subhead))
                .scrollContentBackground(.hidden)
                .frame(minHeight: 60, idealHeight: 60, maxHeight: 140)
                .padding(6)
                .field(radius: 8)

            HStack(spacing: 8) {
                Button("Cancel", action: onCancel)
                    .buttonStyle(SecondaryButtonStyle())

                // Emptied and submitted is how a comment is taken back — the
                // Go TUI's own rule, and the reason this is never disabled on
                // blank text: clearing an existing comment and pressing this
                // is a second way to remove it, beside the card's trash icon.
                Button("Comment") { onSave(text) }
                    .buttonStyle(PrimaryButtonStyle())
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
