import AppKit
import SwiftUI
import NatKit

// The small pieces a task source is drawn with, wherever it shows: its icon,
// its coloured badges, its identity in the titlebar, a container's facts.
// Everything they draw is data the plugin declared (`SourceModels.swift`);
// nothing here decides anything.

/// The glyphs a source's rows are drawn with, whatever the plugin: a
/// container is the design's card, a link a branch (a pull request) or an
/// arrow out (anything else).
enum SourceGlyph {
    static let container = "rectangle.on.rectangle.angled"
    static let pullRequestLink = "arrow.triangle.branch"
    static let externalLink = "arrow.up.right.square"
}

/// A source's icon: the plugin's own SVG drawn as a template in the ink
/// around it, where it gave one that reads as an image, else its SF Symbol.
struct SourceIconView: View {
    let icon: SourceIcon
    var size: CGFloat = 13

    var body: some View {
        if let image = Self.image(icon.svg) {
            Image(nsImage: image)
                .renderingMode(.template)
                .resizable()
                .aspectRatio(contentMode: .fit)
                .frame(width: size, height: size)
        } else {
            Image(systemName: icon.symbol)
                .font(.system(size: size - 1))
                .frame(width: size, height: size)
        }
    }

    private static func image(_ svg: String?) -> NSImage? {
        guard let svg, let image = NSImage(data: Data(svg.utf8)) else { return nil }
        image.isTemplate = true
        return image
    }
}

/// A container's badge: its word in the plugin's colour, on a capsule of
/// the same hue washed into the ground (`DesignTokens.wireBadge`); a colour
/// that will not parse draws as a quiet secondary chip.
struct SourceBadgeView: View {
    @Environment(\.ground) private var ground
    let badge: SourceBadge

    var body: some View {
        let colors = DesignTokens.wireBadge(badge.color, on: ground)
        Text(badge.text)
            .font(Typo.mono(size: 10.5, weight: .medium))
            .tracking(0.3)
            .lineLimit(1)
            .foregroundStyle(colors?.ink ?? DesignTokens.chipInk(.labelSecondary, on: ground))
            .padding(.horizontal, 4)
            .frame(minWidth: 24, minHeight: 16)
            .background(colors?.wash ?? DesignTokens.chipWash(.labelSecondary, on: ground))
            .clipShape(RoundedRectangle(cornerRadius: 4))
            .fixedSize()
            .help(badge.title ?? badge.text)
    }
}

/// A source container named as the titlebar names it: its source's icon in
/// the dot's place, the source's tag, then its title — `ActiveIdentityLabel`'s
/// shape.
struct SourceIdentityLabel: View {
    @Environment(\.ground) private var ground
    let icon: SourceIcon
    let tag: String
    let title: String
    var size: CGFloat = GnatMetrics.body

    var body: some View {
        HStack(spacing: 6) {
            SourceIconView(icon: icon, size: 12)
                .ink(.secondary)
                .frame(width: 12)
            (identityTag(tag, on: ground) + Text(title))
                .font(.system(size: size))
                .ink(.primary)
                .lineLimit(1)
        }
    }
}

/// One fact's value: its words, led by a small dot in the plugin's colour
/// where the fact carries one.
struct SourceFactValue: View {
    @Environment(\.ground) private var ground
    let fact: SourceFact

    var body: some View {
        HStack(spacing: 5) {
            if let color = fact.color.flatMap({ DesignTokens.wireTint($0, on: ground) }) {
                Circle().fill(color).frame(width: 7, height: 7)
            }
            Text(fact.value).ink(.primary).lineLimit(1)
        }
    }
}

/// A container's facts as rows of a facts grid — label and value, as the
/// brief card's facts are drawn — for the `Grid` around them to lay out.
struct SourceFactRows: View {
    let facts: [SourceFact]

    var body: some View {
        ForEach(Array(facts.enumerated()), id: \.offset) { _, fact in
            GridRow {
                Text(fact.label).ink(.tertiary)
                SourceFactValue(fact: fact)
            }
        }
    }
}

/// A source's own actions as menu items: a plain action runs on the click, a
/// `text` one asks for its line first (`onText`), a `choice` one is a
/// submenu of its options, a destructive one is drawn as one and confirmed
/// (`onConfirm`). An input kind this build does not know is left out.
struct SourceActionItems: View {
    let actions: [SourceAction]
    let onRun: (SourceAction, String?) -> Void
    let onText: (SourceAction) -> Void
    let onConfirm: (SourceAction) -> Void

    var body: some View {
        ForEach(actions) { action in
            switch action.input {
            case .none:
                if action.destructive {
                    Button(action.label, role: .destructive) { onConfirm(action) }
                } else {
                    Button(action.label) { onRun(action, nil) }
                }
            case .text:
                Button(action.label.hasSuffix("\u{2026}") ? action.label : action.label + "\u{2026}") { onText(action) }
            case .choice:
                Menu(action.label) {
                    ForEach(action.options, id: \.self) { option in
                        Button(option) { onRun(action, option) }
                    }
                }
            case .unknown:
                EmptyView()
            }
        }
    }
}

/// The small sheet a `text` action asks for its line in.
struct SourceActionTextSheet: View {
    let action: SourceAction
    let onCancel: () -> Void
    let onRun: (String) -> Void

    @State private var text = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text(action.label.trimmingCharacters(in: CharacterSet(charactersIn: "\u{2026}.")))
                .font(.system(size: Typo.headline, weight: .semibold))
                .ink(.primary)
            TextField("", text: $text)
                .textFieldStyle(.roundedBorder)
                .font(Typo.mono(size: Typo.code))
                .onSubmit(submit)
            HStack {
                Spacer()
                Button("Cancel", action: onCancel)
                    .buttonStyle(SecondaryButtonStyle())
                    .keyboardShortcut(.cancelAction)
                Button("OK", action: submit)
                    .buttonStyle(PrimaryButtonStyle())
                    .keyboardShortcut(.defaultAction)
                    .disabled(text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }
        .padding(20)
        .frame(width: 360)
    }

    private func submit() {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        onRun(trimmed)
    }
}
