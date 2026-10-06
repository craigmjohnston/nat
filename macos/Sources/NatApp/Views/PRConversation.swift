import AppKit
import SwiftUI
import NatKit

/// One entry of the conversation timeline, boxed as a Thread item is: a
/// byline — an avatar circle with the author's initials, their name, what
/// they did in saying it, coloured by its tone, since a review's verdict is
/// the entry's whole point and an avatar cannot carry it, and when — over
/// the markdown they wrote.
struct PRConversationEntryView: View {
    let entry: ConvoEntry
    @Environment(\.clock) private var clock

    static let avatarSize: CGFloat = 20

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .center, spacing: 8) {
                Text(authorInitials(entry.author))
                    .font(.system(size: 8, weight: .semibold))
                    .ink(.accent)
                    .frame(width: Self.avatarSize, height: Self.avatarSize)
                    .wash(.avatar)
                    .clipShape(Circle())

                Text(entry.author)
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .ink(.primary)

                Text(entry.verb)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(entry.tone.tint)

                Spacer(minLength: 0)

                Text(ago(clock().timeIntervalSince(entry.at)))
                    .font(.system(size: Typo.caption, weight: .regular))
                    .ink(.tertiary)
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 7)
            .frame(maxWidth: .infinity, alignment: .leading)
            .surface(.chrome)

            if !entry.body.isEmpty {
                MarkdownView(text: entry.body, size: PRConversationMetrics.textSize, ink: .primary)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 8)
                    .overlay(alignment: .top) { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .clipShape(RoundedRectangle(cornerRadius: 4))
        .overlay {
            RoundedRectangle(cornerRadius: 4).strokeBorder(DesignTokens.rule(.separator, on: .window), lineWidth: 1)
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
                        .hoverBrightens()
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
                    .font(Typo.mono(size: Typo.input))
                    .ink(.tertiary)
                    .padding(.leading, 5)
                    .allowsHitTesting(false)
            }
            TextEditor(text: $text)
                .font(Typo.mono(size: Typo.input))
                .scrollContentBackground(.hidden)
                // No scroller, and so no gutter where "Show scroll bars" is
                // Always (or a mouse is connected): the box still scrolls.
                .scrollIndicators(.never)
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

/// The PR section's reviewers: everyone asked and not yet answered, each
/// with a ✕ that withdraws the request, and Request Review — the
/// repository's collaborators, read when the section shows, and Other… for
/// a login (or `org/team`) typed out. All through `nat pr-reviewers`; a
/// refusal shows under the list and changes nothing.
struct ReviewersBlock: View {
    let pr: PRDetail
    let store: PRStore?

    @State private var candidates: [String] = []
    @State private var candidatesError: String?
    @State private var busy = false
    @State private var error: String?
    @State private var askingOther = false
    @State private var otherLogin = ""

    private var editable: Bool {
        store != nil && pr.state != PRLifecycleState.merged && pr.state != PRLifecycleState.closed
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            NavHeading(text: "Reviewers")
            if pr.reviewRequests.isEmpty {
                Text("No review requested.").ink(.secondary)
            } else {
                VStack(alignment: .leading, spacing: 3) {
                    ForEach(pr.reviewRequests, id: \.self) { login in
                        HStack(spacing: 6) {
                            Image(systemName: "circle.dashed")
                                .font(.system(size: GnatMetrics.treeGlyph, weight: .medium))
                                .ink(.secondary)
                                .frame(width: GnatMetrics.treeGlyphColumn)
                            Text(login).ink(.primary).lineLimit(1)
                            Text("· requested").ink(.secondary)
                            Spacer(minLength: 0)
                            if editable {
                                Button { edit(remove: [login]) } label: {
                                    Image(systemName: "xmark")
                                        .font(.system(size: 9, weight: .semibold))
                                        .ink(.tertiary)
                                        // The checks' cancel column, above it.
                                        .frame(width: CheckControlSlot<EmptyView>.side, height: CheckControlSlot<EmptyView>.side)
                                        .contentShape(Rectangle())
                                }
                                .buttonStyle(GnatIconButtonStyle())
                                .disabled(busy)
                                .help("Withdraw the request to \(login)")
                            }
                        }
                    }
                }
                .monoXS()
            }

            if editable {
                HStack(spacing: 8) {
                    Menu {
                        if let candidatesError {
                            Text("Collaborators could not be listed: \(candidatesError)")
                        } else if candidates.isEmpty {
                            Text("No one else to ask")
                        }
                        ForEach(candidates, id: \.self) { login in
                            Button(login) { edit(add: [login]) }
                        }
                        Divider()
                        Button("Other\u{2026}") {
                            otherLogin = ""
                            askingOther = true
                        }
                    } label: {
                        HeaderActionLabel(title: "Request review", systemImage: "person.badge.plus", isBusy: busy)
                    }
                    .menuStyle(.button)
                    .buttonStyle(GnatButtonStyle())
                    .menuIndicator(.hidden)
                    .fixedSize()
                    .disabled(busy)
                }
                .padding(.top, 2)
            }

            if let error {
                Text(error).ink(.danger).fixedSize(horizontal: false, vertical: true)
            }
        }
        .task(id: "\(pr.url)|\(pr.reviewRequests.joined(separator: ","))") { await loadCandidates() }
        .alert("Request a review", isPresented: $askingOther) {
            TextField("login or org/team", text: $otherLogin)
                .font(Typo.mono(size: Typo.input))
            Button("Request") {
                let login = otherLogin.trimmingCharacters(in: .whitespaces)
                if !login.isEmpty { edit(add: [login]) }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("GitHub asks them to review pull request #\(pr.number).")
        }
    }

    private func loadCandidates() async {
        guard editable, let store else { return }
        do {
            let answer = try await store.reviewers()
            candidates = answer?.candidates ?? []
            candidatesError = answer?.candidatesError
        } catch {
            candidatesError = SliceActionTracker.message(for: error)
        }
    }

    private func edit(add: [String] = [], remove: [String] = []) {
        guard let store else { return }
        busy = true
        error = nil
        Task {
            do {
                // nat answers an edit with what it did, not who could be asked
                // now: whoever was just asked leaves the candidates here, and
                // the settle read the edit asks for brings the rest.
                if let answer = try await store.editReviewers(add: add, remove: remove) {
                    candidates.removeAll { answer.added.contains($0) }
                }
            } catch {
                self.error = SliceActionTracker.message(for: error)
            }
            busy = false
        }
    }
}
