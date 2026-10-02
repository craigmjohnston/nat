import AppKit
import SwiftUI
import NatKit

// The diff itself — headers, rows, gutter, the comment button — is drawn by
// `DiffCanvasView`. What stays SwiftUI is what is drawn under a row: the
// pending comments and the editor a comment is written in, hosted by
// `DiffCanvasRepresentable`.

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
