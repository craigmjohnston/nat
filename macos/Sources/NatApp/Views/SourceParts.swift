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
    /// A container — the generic card mark everywhere one is named (the
    /// breadcrumb, pickers, the new-task sheet), and a sidebar card row with
    /// tasks under it: a card with another stacked behind.
    static let container = "rectangle.on.rectangle.angled"
    /// A sidebar card row with no tasks: the same card alone, the stack's
    /// front card, so the two read as one family at a glance.
    static let emptyContainer = "rectangle"
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
/// that will not parse draws as a quiet secondary chip. Every badge is one
/// width, `width`, its word centred, so a column of rows lines up; a word
/// longer than three characters shrinks to fit rather than widening it.
struct SourceBadgeView: View {
    @Environment(\.ground) private var ground
    let badge: SourceBadge
    /// The source's icon, leading the word — a card's badge drawn outside
    /// the source's own section; the capsule widens to hold it.
    var icon: SourceIcon?

    /// Every badge's one width — `BadgeCapsule.width`.
    static var width: CGFloat { BadgeCapsule.width }

    var body: some View {
        let colors = DesignTokens.wireBadge(badge.color, on: ground)
        BadgeCapsule(text: badge.text, ink: colors?.ink, wash: colors?.wash, icon: icon)
            .help(badge.title ?? badge.text)
    }
}

/// What names a source's task or container outside the source's own section,
/// in a project badge's place — a source project takes no badge: its card's
/// badge (a Shortcut card's project) led by the source's icon, else the icon
/// alone.
struct CardMarkView: View {
    let badge: SourceBadge?
    let icon: SourceIcon
    var ink: InkRole = .secondary

    var body: some View {
        if let badge {
            SourceBadgeView(badge: badge, icon: icon)
        } else {
            SourceIconView(icon: icon, size: 12)
                .ink(ink)
                .frame(height: BadgeCapsule.height)
        }
    }
}

/// A source container named as the titlebar names it — `ActiveIdentityLabel`'s
/// shape: its card mark and a slash, then the card glyph in the dot's place
/// and its title. No card mark where `cardIcon` is nil (the last crumb,
/// after a crumb already drawing it).
struct SourceIdentityLabel: View {
    let cardBadge: SourceBadge?
    let cardIcon: SourceIcon?
    let title: String
    var size: CGFloat = GnatMetrics.body
    /// The glyph's and the title's inks — the titlebar's quieter crumb passes
    /// `.tertiary` for both; the badge keeps its own.
    var iconInk: InkRole = .secondary
    var titleInk: InkRole = .primary

    var body: some View {
        HStack(spacing: 6) {
            if let cardIcon {
                CardMarkView(badge: cardBadge, icon: cardIcon, ink: iconInk)
                CrumbSlash()
            }
            Image(systemName: SourceGlyph.container)
                .font(.system(size: 10))
                .ink(iconInk)
                .frame(width: 13)
            Text(title)
                .ink(titleInk)
                .lineLimit(1)
        }
        .font(.system(size: size))
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
/// (`onConfirm`). A `filter` one is never an item — it is a button of its
/// own beside the menu — and an input kind this build does not know is left
/// out.
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
            case .filter, .unknown:
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
                .font(Typo.mono(size: Typo.input))
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

/// The filter editor a `filter` action opens, anchored to the row whose menu
/// it came from: one row per field — a single choice as a pop-up menu with
/// "Any" first (naming what it falls through to, where a wider filter sets
/// the field), several as a menu of checkmarks — then Cancel and Apply. A
/// field whose options are still loading says so and is the only one held:
/// the editor reads the tree once more (`onReread`) and draws what comes
/// back, keeping what was picked meanwhile. Apply hands the choices to
/// `onApply` as the action's input.
struct SourceFilterPopover: View {
    /// The action as the tree now has it — re-read while open, so a loading
    /// field fills in.
    let action: SourceAction
    let onCancel: () -> Void
    let onApply: (String) -> Void
    var onReread: () async -> Void = {}

    @State private var draft: SourceFilterDraft

    init(
        action: SourceAction, onCancel: @escaping () -> Void, onApply: @escaping (String) -> Void,
        onReread: @escaping () async -> Void = {}
    ) {
        self.action = action
        self.onCancel = onCancel
        self.onApply = onApply
        self.onReread = onReread
        _draft = State(initialValue: SourceFilterDraft(action: action))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text(action.label.trimmingCharacters(in: CharacterSet(charactersIn: "\u{2026}.")))
                .font(.system(size: Typo.headline, weight: .semibold))
                .ink(.primary)
            Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 10) {
                ForEach(action.fields) { field in
                    GridRow {
                        Text(field.label)
                            .font(.system(size: GnatMetrics.body))
                            .ink(.secondary)
                            .gridColumnAlignment(.trailing)
                        control(field)
                            .frame(width: 200, alignment: .leading)
                    }
                }
            }
            HStack {
                Spacer()
                Button("Cancel", action: onCancel)
                    .buttonStyle(SecondaryButtonStyle())
                    .keyboardShortcut(.cancelAction)
                Button("Apply") { onApply(draft.input(for: action)) }
                    .buttonStyle(PrimaryButtonStyle())
                    .keyboardShortcut(.defaultAction)
                    .disabled(!draft.differs(from: action))
            }
        }
        .padding(16)
        .fixedSize()
        .task {
            // Once, and only for a field still loading: the plugin is filling
            // it in the background, and a second look is all it needs.
            guard SourceFilterDraft.isLoading(action) else { return }
            try? await Task.sleep(for: .seconds(2))
            await onReread()
        }
    }

    @ViewBuilder
    private func control(_ field: SourceFilterField) -> some View {
        if field.loading && field.options.isEmpty {
            HStack(spacing: 6) {
                ProgressView().controlSize(.small)
                Text("Loading\u{2026}")
                    .font(.system(size: GnatMetrics.body))
                    .ink(.tertiary)
            }
            // A pop-up's height, so the rows stay put as it fills in.
            .frame(minHeight: 22)
        } else if field.multi {
            Menu {
                ForEach(field.options) { option in
                    Toggle(option.label, isOn: Binding(
                        get: { draft.isChosen(option.id, in: field) },
                        set: { _ in draft.toggle(option.id, in: field) }))
                }
            } label: {
                Text(draft.summary(field)).lineLimit(1)
            }
            .menuStyle(.button)
            .fixedSize(horizontal: false, vertical: true)
        } else {
            // A pull-down menu around an inline picker, not a pop-up picker:
            // a `.menu` picker builds every item before the popover can be
            // drawn — a fifth of a second for 400 epics, a second for 2,000
            // — where a menu's items are built only when it opens.
            Menu {
                Picker(field.label, selection: Binding(
                    get: { draft.choice(field) ?? "" },
                    set: { draft.choose($0.isEmpty ? nil : $0, in: field) }
                )) {
                    Text(SourceFilterDraft.anyLabel(field)).tag("")
                    Divider()
                    ForEach(field.options) { option in
                        Text(option.label).tag(option.id)
                    }
                }
                .labelsHidden()
                .pickerStyle(.inline)
            } label: {
                Text(draft.summary(field)).lineLimit(1)
            }
            .menuStyle(.button)
            .fixedSize(horizontal: false, vertical: true)
        }
    }
}
