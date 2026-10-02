import AppKit
import SwiftUI
import NatKit

/// One entry of the conversation timeline, the mock's `PRComment` shape: an
/// avatar circle with the author's initials, their name, what they did in
/// saying it — coloured by its tone, since a review's verdict is the entry's
/// whole point and an avatar cannot carry it — when, and the markdown they
/// wrote, aligned under the name rather than the avatar.
struct PRConversationEntryView: View {
    let entry: ConvoEntry

    /// The byline's avatar and the gap after it. Internal rather than
    /// private, so `PRSkeletonView` stands its entries in the very column
    /// these put the text in.
    static let avatarSize: CGFloat = 22
    static let avatarGap: CGFloat = 10

    var body: some View {
        HStack(alignment: .top, spacing: Self.avatarGap) {
            Text(authorInitials(entry.author))
                .font(.system(size: 9, weight: .semibold))
                .ink(.accent)
                .frame(width: Self.avatarSize, height: Self.avatarSize)
                .wash(.avatar)
                .clipShape(Circle())
                .padding(.top, 2)

            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 8) {
                    Text(entry.author)
                        .font(.system(size: Typo.subhead, weight: .semibold))
                        .ink(.primary)

                    Text(entry.verb)
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .foregroundStyle(entry.tone.tint)

                    Text(ago(Date().timeIntervalSince(entry.at)))
                        .font(.system(size: Typo.caption, weight: .regular))
                        .ink(.tertiary)
                }

                if !entry.body.isEmpty {
                    Text(markdownAttributed(entry.body, size: Typo.subhead))
                        .font(.system(size: Typo.subhead, weight: .regular))
                        .ink(.secondary)
                        .lineSpacing(2)
                }
            }
        }
    }
}

/// The composer's own metrics. Named rather than inline for the reason
/// `ButtonMetrics` is, and for one more: `PRSkeletonView` reserves the band
/// the box lays out in, and a skeleton that guessed it would hand the
/// conversation rows the composer then took back.
enum PRComposerMetrics {
    /// What the editor is at least — one line — and at most. Inside the
    /// conversation's scroll the editor grows with its text between the two,
    /// and only scrolls itself once a comment is taller than the ceiling.
    static let editorMinHeight: CGFloat = 36
    static let editorMaxHeight: CGFloat = 60
    /// The inset over the editor.
    static let editorTopPadding: CGFloat = 7
    /// The send row under it: the button's own height and the insets around
    /// the row.
    static let sendRowHeight: CGFloat = 22
    static let sendRowTopPadding: CGFloat = 5
    static let sendRowBottomPadding: CGFloat = 7
    /// The field's corner.
    static let cornerRadius: CGFloat = 8

    /// What the box comes to empty, which is how a pull request opens: the
    /// editor at its floor, the send row under it, and the insets around both.
    static var height: CGFloat {
        editorTopPadding + editorMinHeight + sendRowTopPadding + sendRowHeight + sendRowBottomPadding
    }
}

/// The pull request's own comment box, matching the mock's `PRComposer`
/// chrome (a rounded field with a toolbar row under it, the send button in
/// accent) but functional: it posts through `pr-comment` rather than sitting
/// there as decoration. It sits at the end of the conversation, inside its
/// scroll; see the call site for why it is not a threaded reply.
struct PRComposerView: View {
    let placeholder: String
    @Binding var text: String
    let isSending: Bool
    let error: String?
    let onSend: () -> Void

    private var canSend: Bool {
        !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !isSending
    }

    /// What the hidden sizer lays out: the text as typed, with a space after
    /// a trailing newline (or in place of nothing) so the line the caret has
    /// just opened is counted too.
    private var sizingText: String {
        text.isEmpty || text.hasSuffix("\n") ? text + " " : text
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            VStack(alignment: .leading, spacing: 0) {
                // The editor is sized by a hidden copy of its own text rather
                // than by a frame of its own: inside a scroll a `TextEditor`
                // is offered no height to fill and would sit at whichever
                // bound its frame named, so the copy — wrapped at the text
                // view's own 5pt line fragment padding — measures the lines,
                // clamped to the editor's floor and ceiling, and the editor
                // is laid over exactly that.
                Text(sizingText)
                    .font(Typo.mono(size: Typo.subhead))
                    .padding(.horizontal, 5)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(
                        minHeight: PRComposerMetrics.editorMinHeight,
                        maxHeight: PRComposerMetrics.editorMaxHeight,
                        alignment: .top
                    )
                    .hidden()
                    .overlay(alignment: .topLeading) { editor }
                    .padding(.horizontal, 2)
                        .padding(.top, PRComposerMetrics.editorTopPadding)

                // The mock's toolbar row also draws textformat and paperclip
                // icons here; neither has anything real to do — gh has no API
                // for comment attachments — and a control that does nothing is
                // worse than the mock losing two glyphs, so only the send
                // button is drawn.
                HStack(spacing: 8) {
                    Spacer()

                    Button(action: onSend) {
                        Group {
                            if isSending {
                                // Sized down to the slot rather than laid out
                                // at the control's own size, which
                                // `scaleEffect` draws smaller without ever
                                // shrinking: an unframed spinner here made the
                                // send button — and the composer under it —
                                // grow while a comment was posting.
                                ProgressView()
                                    .controlSize(.small)
                                    .scaleEffect(0.55)
                                    .frame(width: 10, height: 10)
                            } else {
                                Image(systemName: "paperplane.fill")
                                    .font(.system(size: 12, weight: .medium))
                                    .ink(.onAccent)
                            }
                        }
                        .frame(width: 24, height: PRComposerMetrics.sendRowHeight)
                        .background(canSend ? DesignTokens.accent : DesignTokens.accentMuted(on: .field))
                        .clipShape(RoundedRectangle(cornerRadius: 6))
                    }
                    .buttonStyle(.plain)
                    .disabled(!canSend)
                    .help("Send")
                }
                .padding(.horizontal, 8)
                .padding(.top, PRComposerMetrics.sendRowTopPadding)
                .padding(.bottom, PRComposerMetrics.sendRowBottomPadding)
            }
            .field(radius: PRComposerMetrics.cornerRadius)

            if let error {
                Text(error)
                    .font(.system(size: Typo.caption, weight: .regular))
                    .ink(.danger)
                    .lineLimit(2)
            }
        }
    }

    /// The editor itself, with the placeholder under it while it is empty.
    private var editor: some View {
        ZStack(alignment: .topLeading) {
            if text.isEmpty {
                // Laid where the editor's own first line starts — the text
                // view's 5pt line fragment padding, and no top offset — so
                // the caret blinks exactly at the placeholder's first letter.
                Text(placeholder)
                    // The editor's own font, since the placeholder stands
                    // exactly where the first typed letter will: a
                    // proportional one would sit a hair off the caret it is
                    // drawn behind.
                    .font(Typo.mono(size: Typo.subhead))
                    .ink(.tertiary)
                    .padding(.leading, 5)
                    .allowsHitTesting(false)
            }
            TextEditor(text: $text)
                .font(Typo.mono(size: Typo.subhead))
                .scrollContentBackground(.hidden)
        }
    }
}

/// Capitalizes just the first letter of an already-lower-cased phrase, for a
/// sidebar line drawn as a short sentence ("Approved", "Review required")
/// rather than the merge box's own lower-case verdict word.
func sentenceCase(_ word: String) -> String {
    guard let first = word.first else { return word }
    return first.uppercased() + word.dropFirst()
}
