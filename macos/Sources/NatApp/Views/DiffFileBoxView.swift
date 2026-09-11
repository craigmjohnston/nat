import AppKit
import SwiftUI
import NatKit

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
            header

            if !isCollapsed {
                Divider().frame(height: 0.5)

                LazyVStack(alignment: .leading, spacing: 0) {
                    ForEach(file.rows) { row in
                        DiffRowView(
                            row: row,
                            numberWidth: numberWidth,
                            isSelected: selection?.rowIDs.contains(row.id) ?? false,
                            showCommentButton: commentsEnabled && draft == nil && row.kind != .hunkBreak
                                && selection?.rowIDs.last == row.id,
                            onSelect: { shift in onRowClick(row, shift) },
                            onComment: onOpenCommentEditor
                        )

                        if let draft, draft.anchorRowIDs.last == row.id {
                            CommentEditorView(
                                initialText: draft.text,
                                onSave: onSaveDraft,
                                onCancel: onCancelDraft
                            )
                            .padding(.leading, commentLeadingInset)
                            .padding(.trailing, 12)
                            .padding(.vertical, 10)
                            .background(DesignTokens.rowAltBg)
                        }

                        ForEach(comments.filter { $0.anchorRowIDs.last == row.id }) { comment in
                            PendingCommentCardView(
                                comment: comment,
                                authorName: authorName,
                                authorInitials: authorInitials,
                                onEdit: { onEditComment(comment) },
                                onDelete: { onDeleteComment(comment) }
                            )
                            .padding(.leading, commentLeadingInset)
                            .padding(.trailing, 12)
                            .padding(.vertical, 10)
                            .background(DesignTokens.rowAltBg)
                        }
                    }
                }
            }
        }
        .background(DesignTokens.controlBg)
        .clipShape(RoundedRectangle(cornerRadius: 10))
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(DesignTokens.controlBorder, lineWidth: 0.5)
        )
    }

    private var header: some View {
        HStack(spacing: 10) {
            Image(systemName: isCollapsed ? "chevron.right" : "chevron.down")
                .font(.system(size: 12, weight: .medium))
                .foregroundStyle(DesignTokens.labelTertiary)

            if isViewed {
                Image(systemName: "checkmark")
                    .font(.system(size: 12, weight: .medium))
                    .foregroundStyle(DesignTokens.systemGreen)
            }

            Text(file.path)
                .font(.system(size: Typo.code, weight: .regular, design: .monospaced))
                .foregroundStyle(DesignTokens.label)
                .lineLimit(1)
                .truncationMode(.head)

            if file.isRenamed {
                Text("was \(file.oldPath)")
                    .font(.system(size: Typo.code, weight: .regular, design: .monospaced))
                    .foregroundStyle(DesignTokens.labelTertiary)
                    .lineLimit(1)
            }

            if !comments.isEmpty {
                HStack(spacing: 3) {
                    Image(systemName: "text.bubble")
                        .font(.system(size: 12, weight: .medium))
                    Text("\(comments.count)")
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .monospacedDigit()
                }
                .foregroundStyle(DesignTokens.accent)
            }

            Spacer()

            if file.adds > 0 {
                Text("+\(file.adds)")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .monospacedDigit()
                    .foregroundStyle(DesignTokens.systemGreen)
            }
            if file.dels > 0 {
                Text("\u{2212}\(file.dels)")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .monospacedDigit()
                    .foregroundStyle(DesignTokens.systemRed)
            }

            Button(action: onToggleViewed) {
                Text("Viewed")
            }
            .buttonStyle(GhostButtonStyle())
        }
        .padding(.horizontal, 12)
        .frame(height: 32)
        .background(DesignTokens.rowAltBg)
        .contentShape(Rectangle())
        .onTapGesture {
            onToggleCollapsed()
        }
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
    let row: DiffRow
    let numberWidth: Int
    let isSelected: Bool
    let showCommentButton: Bool
    let onSelect: (Bool) -> Void
    let onComment: () -> Void

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

    /// The gutter's paint, drawn as the row's own background rather than the
    /// numbers': a wrapped line makes the row taller than its numbers, and a
    /// fill on the numbers alone floats in the middle of it as a block with
    /// bare row above and below. The hairline is the mock's border between
    /// the gutter and the code.
    private func gutterCell(_ fill: Color) -> some View {
        HStack(spacing: 0) {
            fill
            DesignTokens.separator.frame(width: 0.5)
        }
        .frame(width: gutterWidth + 0.5)
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
        HStack(spacing: 0) {
            Text("···")
                .font(.system(size: Typo.code, weight: .regular, design: .monospaced))
                .foregroundStyle(DesignTokens.accent)
                .frame(width: gutterWidth)

            Text(row.text)
                .font(.system(size: Typo.code, weight: .regular, design: .monospaced))
                .foregroundStyle(DesignTokens.labelTertiary)
                .lineLimit(1)
                .padding(.leading, 12)

            Spacer(minLength: 0)
        }
        .frame(minHeight: 24)
        .background(alignment: .leading) {
            gutterCell(DesignTokens.accent.opacity(0.1))
        }
    }

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
            .font(.system(size: Typo.code, weight: .regular, design: .monospaced))
            .foregroundStyle(DesignTokens.labelTertiary)
            .padding(.horizontal, 8)

            Text(glyph)
                .font(.system(size: Typo.code, weight: .regular, design: .monospaced))
                .foregroundStyle(glyphColor)
                .frame(width: 13)
                .padding(.leading, 12)

            Text(rowText)
                .font(.system(size: Typo.code, weight: .regular, design: .monospaced))
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)

            if showCommentButton {
                Button(action: onComment) {
                    Image(systemName: "plus.bubble")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundStyle(DesignTokens.accent)
                }
                .buttonStyle(.plain)
                .padding(.trailing, 8)
                .help("Comment on this line")
            }

            Spacer(minLength: 0)
        }
        .padding(.vertical, 1.5)
        .frame(minHeight: 19)
        .background(alignment: .leading) {
            gutterCell(gutterFill)
        }
        .background(rowFill)
        .background(isSelected ? DesignTokens.accent.opacity(0.16) : Color.clear)
        .contentShape(Rectangle())
        .onTapGesture {
            onSelect(NSEvent.modifierFlags.contains(.shift))
        }
    }

    /// The row's text, coloured per its syntax runs where the file has any
    /// (`DiffRow.tokens`) — always drawn on top of the row's own +/- wash,
    /// never instead of it. A row with no tokens (no matched language, or the
    /// blank placeholder for an empty line) renders exactly as it always did:
    /// one plain colour, the dimmer label for a described file's message.
    private var rowText: AttributedString {
        let defaultColor: Color = row.kind == .described ? DesignTokens.labelSecondary : DesignTokens.label
        guard !row.text.isEmpty else {
            var s = AttributedString(" ")
            s.foregroundColor = defaultColor
            return s
        }
        return DiffSyntax.attributedLine(row.text, tokens: row.tokens, defaultColor: defaultColor)
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
        case .added: return DesignTokens.systemGreen
        case .removed: return DesignTokens.systemRed
        default: return .clear
        }
    }

    private var rowFill: Color {
        switch row.kind {
        case .added: return DesignTokens.systemGreen.opacity(0.2)
        case .removed: return DesignTokens.systemRed.opacity(0.2)
        default: return .clear
        }
    }

    private var gutterFill: Color {
        switch row.kind {
        case .added: return DesignTokens.systemGreen.opacity(0.32)
        case .removed: return DesignTokens.systemRed.opacity(0.32)
        default: return DesignTokens.rowAltBg
        }
    }
}

/// A pending comment, drawn as a card right under the last line it covers:
/// an avatar circle (the configured user's initials), their name, a yellow
/// "Pending" badge — every comment here is, since none of them are written
/// anywhere until they are sent — and the edit/delete icons the mock shows.
struct PendingCommentCardView: View {
    let comment: PendingComment
    let authorName: String
    let authorInitials: String
    let onEdit: () -> Void
    let onDelete: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 8) {
                Text(authorInitials)
                    .font(.system(size: Typo.caption, weight: .semibold))
                    .foregroundStyle(DesignTokens.accent)
                    .frame(width: 20, height: 20)
                    .background(DesignTokens.accent.opacity(0.3))
                    .clipShape(Circle())

                Text(authorName)
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .foregroundStyle(DesignTokens.label)

                Text("Pending")
                    .font(.system(size: Typo.caption, weight: .semibold))
                    .foregroundStyle(DesignTokens.systemYellow)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(DesignTokens.systemYellow.opacity(0.18))
                    .clipShape(Capsule())

                Spacer()

                Button(action: onEdit) {
                    Image(systemName: "pencil")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundStyle(DesignTokens.labelTertiary)
                }
                .buttonStyle(.plain)
                .help("Edit this comment")

                Button(action: onDelete) {
                    Image(systemName: "trash")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundStyle(DesignTokens.labelTertiary)
                }
                .buttonStyle(.plain)
                .help("Delete this comment")
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 7)
            .background(DesignTokens.controlFace)

            Divider().frame(height: 0.5)

            Text(comment.text)
                .font(.system(size: Typo.subhead, weight: .regular))
                .foregroundStyle(DesignTokens.label)
                .padding(12)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .background(DesignTokens.controlBg)
        .frame(maxWidth: .infinity, alignment: .leading)
        .clipShape(RoundedRectangle(cornerRadius: 8))
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .stroke(DesignTokens.controlBorder, lineWidth: 0.5)
        )
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
                .font(.system(size: Typo.subhead, weight: .regular))
                .scrollContentBackground(.hidden)
                .frame(minHeight: 60, idealHeight: 60, maxHeight: 140)
                .padding(6)
                .background(DesignTokens.fieldBg)
                .clipShape(RoundedRectangle(cornerRadius: 8))
                .overlay(
                    RoundedRectangle(cornerRadius: 8)
                        .stroke(DesignTokens.controlBorder, lineWidth: 0.5)
                )

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
