import SwiftUI
import NatKit

/// An Untitled tab's pane: the starter card, centred at 620pt, per `NPStarter`
/// in the design (`docs/design/nat-new-project/ui-newproject.jsx`). A
/// description to workshop into a plan, or an existing project to open.
///
/// Only From Notion is wired here — it is the app's add-project flow, and the
/// project it opens takes the tab over. The rest is drawn whole and disabled,
/// each with a `.help` naming the slice that wires it.
struct StarterView: View {
    /// Runs the add-project-from-Notion flow, which the window presents.
    let onFromNotion: () -> Void

    @State private var draft = ""

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                Text(StarterCard.title)
                    .font(.system(size: 22, weight: .semibold))
                    .ink(.primary)
                    .padding(.bottom, 6)

                Text(StarterCard.subtitle)
                    .font(.system(size: Typo.body, weight: .regular))
                    .ink(.secondary)
                    .padding(.bottom, 28)

                describeCard

                openDivider
                    .padding(.vertical, 22)

                HStack(spacing: 14) {
                    Button(action: onFromNotion) {
                        StarterTile(
                            title: StarterCard.notionTitle,
                            subtitle: StarterCard.notionSubtitle
                        ) { NotionMark(size: 26) }
                    }
                    .buttonStyle(StarterTileButtonStyle())

                    Button(action: {}) {
                        StarterTile(
                            title: StarterCard.filesystemTitle,
                            subtitle: StarterCard.filesystemSubtitle
                        ) {
                            Image(systemName: "folder")
                                .font(.system(size: 24, weight: .regular))
                                .ink(.info)
                        }
                    }
                    .buttonStyle(StarterTileButtonStyle())
                    .disabled(true)
                    .help(StarterCard.filesystemStaging)
                }
                // Both tiles as tall as the taller, as the mock's flex row has them.
                .fixedSize(horizontal: false, vertical: true)
            }
            .frame(width: 620)
            .padding(.top, 96)
            .padding(.bottom, 40)
            .frame(maxWidth: .infinity)
            .inelastic()
        }
        .surface(.window)
    }

    // MARK: - Describe a plan

    private var describeCard: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 8) {
                Image(systemName: "wand.and.stars")
                    .font(.system(size: 14, weight: .regular))
                    .ink(.accent)
                Text(StarterCard.describeHeading)
                    .font(.system(size: Typo.body, weight: .semibold))
                    .ink(.primary)
            }
            .padding(.bottom, 10)

            TextEditor(text: $draft)
                .font(Typo.mono(size: Typo.body))
                .ink(.primary)
                .scrollContentBackground(.hidden)
                .padding(.horizontal, 7)
                .padding(.vertical, 5)
                .frame(minHeight: 76)
                .overlay(alignment: .topLeading) {
                    if draft.isEmpty {
                        Text(StarterCard.describePlaceholder)
                            .font(Typo.mono(size: Typo.body))
                            .ink(.tertiary)
                            .padding(.horizontal, 12)
                            .padding(.vertical, 10)
                            .allowsHitTesting(false)
                    }
                }
                .field(radius: 6)

            HStack(spacing: 12) {
                Button(action: {}) {
                    HStack(spacing: 6) {
                        Image(systemName: "doc")
                            .font(.system(size: 12, weight: .regular))
                            .ink(.tertiary)
                        Text(StarterCard.openPlanLabel)
                            .font(.system(size: Typo.subhead, weight: .regular))
                            .ink(.secondary)
                    }
                }
                .buttonStyle(.plain)
                .disabled(true)
                .opacity(ButtonMetrics.disabledOpacity)
                .help(StarterCard.openPlanStaging)

                Spacer()

                Text(StarterCard.startHint)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.tertiary)

                Button(action: {}) {
                    Text(StarterCard.workshopLabel)
                }
                .buttonStyle(PrimaryButtonStyle())
                .keyboardShortcut(.return, modifiers: .command)
                .disabled(true)
                .help(StarterCard.workshopStaging)
            }
            .padding(.top, 12)
        }
        .padding(.horizontal, 18)
        .padding(.top, 16)
        .padding(.bottom, 14)
        .card(radius: 10)
    }

    // MARK: - The divider

    private var openDivider: some View {
        HStack(spacing: 12) {
            Rule(.separator)
            Text(StarterCard.openDivider)
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.tertiary)
                .fixedSize()
            Rule(.separator)
        }
    }
}

/// One of the two "open an existing project" tiles: a glyph over a title over
/// a line of what it does, on a card.
private struct StarterTile<Glyph: View>: View {
    let title: String
    let subtitle: String
    @ViewBuilder let glyph: () -> Glyph

    var body: some View {
        VStack(spacing: 4) {
            glyph()
                .frame(height: 34)
                .padding(.bottom, 6)

            Text(title)
                .font(.system(size: Typo.body, weight: .semibold))
                .ink(.primary)

            Text(subtitle)
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.tertiary)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(.top, 20)
        .padding(.bottom, 18)
        .padding(.horizontal, 20)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .card(radius: 10)
        .contentShape(RoundedRectangle(cornerRadius: 10))
    }
}

/// A tile's button: the card as it is, dimmed while pressed and while it has
/// nothing to do yet.
private struct StarterTileButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .opacity(buttonOpacity(isPressed: configuration.isPressed, isEnabled: isEnabled))
    }
}

#Preview {
    StarterView(onFromNotion: {})
        .frame(width: 1000, height: 780)
}
