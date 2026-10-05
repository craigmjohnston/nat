import SwiftUI
import NatKit

// MARK: - Cut-short text

/// A chunk of text cut to its first `maxWords` words until asked for the
/// rest — the brief's own Show more, for every Thread item that carries
/// prose, and the PR description at three times the length. Folds again
/// whenever the text itself changes.
struct Excerpt<Content: View>: View {
    let text: String
    var maxWords = briefExcerptWords
    @ViewBuilder let content: (String) -> Content

    @State private var expanded = false

    var body: some View {
        let excerpt = briefExcerpt(text, maxWords: maxWords)
        VStack(alignment: .leading, spacing: 4) {
            content(expanded ? text : excerpt ?? text)
            if excerpt != nil {
                Button(expanded ? "Show less" : "Show more") { expanded.toggle() }
                    .buttonStyle(GnatLinkButtonStyle())
            }
        }
        .onChange(of: text) { _, _ in expanded = false }
    }
}

// MARK: - Icons

extension ThreadEventKind {
    /// The glyph a Task log item is headed with — outlined, never filled,
    /// as the design's thin line icons are, so the set reads as one family.
    var symbol: String {
        switch self {
        case .launched: return "play.circle"
        case .agent: return "terminal"
        case .handedBack: return "arrow.uturn.backward.circle"
        case .sentBack: return "arrow.uturn.forward.circle"
        case .released: return "arrow.down.to.line.circle"
        case .relaunched: return "arrow.clockwise.circle"
        case .checksFailed: return "xmark.octagon"
        case .blocked: return "exclamationmark.octagon"
        case .followUps: return "lightbulb"
        case .followUp: return "arrow.right.circle"
        case .note: return "text.bubble"
        case .approved: return "checkmark.seal"
        // A stand-in name: `ThreadIcon` draws gnat's own `MergeIcon` for it.
        case .merged: return mergeSymbol
        case .closed: return "checkmark.circle"
        }
    }
}

/// The name a Task log item's symbol carries for a merge — drawn as gnat's
/// `MergeIcon`, the merge button's, rather than as an SF Symbol.
let mergeSymbol = "arrow.triangle.merge"

/// The brief's own glyph, heading it as a kind heads every other item.
let briefSymbol = "doc.text"

/// A Task log item's glyph, in the log's margin column: light, as the
/// design's line icons are, and quiet unless the item is live.
struct ThreadIcon: View {
    let symbol: String
    var role: InkRole = .tertiary

    static let size: CGFloat = 13

    var body: some View {
        Group {
            if symbol == mergeSymbol {
                MergeIcon(size: Self.size, lineWidth: 1.2)
            } else {
                Image(systemName: symbol)
                    .font(.system(size: Self.size, weight: .regular))
            }
        }
        .ink(role)
    }
}

// MARK: - Log items

/// How a Task log item's rule runs on down the margin column under its icon:
/// solid to the next item, none after the last, dashed after the last while
/// the log is live — the agent still at it.
enum LogConnector: Equatable {
    case none, solid, dashed
}

/// The Task log's measures — the design's `.log`/`.lg` rules.
enum LogMetrics {
    /// The margin column the icons and the rule sit in.
    static let margin: CGFloat = 24
    /// The header line the icon is centred on.
    static let headHeight: CGFloat = 19
    /// Between one item's content and the next's: the design's 16 between
    /// items plus each item's own 8 above it.
    static let spacing: CGFloat = 24
    /// Where the rule starts, under the icon, from the item's top.
    static let ruleTop: CGFloat = 23
    /// How far a solid rule runs into the gap under its item: to as far short
    /// of the next item's icon as it starts under its own.
    static let ruleOverrun: CGFloat = 20
    /// How far a live log's dashed tail runs past its last item.
    static let tailOverrun: CGFloat = 16
    /// The rule's x in the margin column: under the icon's centre.
    static let ruleX: CGFloat = 6
    /// The x of an open group's own rule, beside its items: in the margin
    /// column's gutter, between the log's rule and the items' text.
    static let groupRuleX: CGFloat = 18
}

extension View {
    /// The Task log's padding around its items: the design's `.log`, with the
    /// first item's own 8 above it, and more room at the foot while the log
    /// is live, for its dashed tail.
    func taskLogPadding(live: Bool = false) -> some View {
        padding(.top, 14)
            .padding(.leading, 12)
            .padding(.trailing, 14)
            .padding(.bottom, live ? 22 : 12)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// One item of the Task log, boxed by nothing: its icon in the margin column,
/// then beside it the header — who, what (its meta, in its tone), and at the
/// end when and any action — over its body. Its rule (`connector`) runs down
/// the margin column from under the icon towards the next item's.
struct LogItem<Action: View, Content: View>: View {
    let symbol: String
    var iconRole: InkRole = .tertiary
    /// Drawn in the icon's slot in place of `symbol`'s glyph, at the same
    /// size so nothing shifts — a folded item's chevron under the pointer, a
    /// group's stacked icon. Nil draws the glyph.
    var glyph: AnyView?
    let who: String
    var whoRole: InkRole = .primary
    /// Sets who in italic — a group's count, which names no one.
    var whoItalic = false
    var meta: String?
    var metaRole: InkRole = .secondary
    var when: String?
    var connector: LogConnector = .none
    @ViewBuilder var action: () -> Action
    @ViewBuilder var content: () -> Content

    var body: some View {
        HStack(alignment: .top, spacing: 0) {
            Group {
                if let glyph { glyph } else { ThreadIcon(symbol: symbol, role: iconRole) }
            }
            .frame(width: ThreadIcon.size, height: LogMetrics.headHeight)
            .frame(width: LogMetrics.margin, alignment: .leading)
            VStack(alignment: .leading, spacing: 0) {
                header
                if Content.self != EmptyView.self {
                    content()
                        .padding(.top, 2)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .overlay(alignment: .topLeading) { rule }
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text(who)
                .font(.system(size: Typo.scaled(13.5)))
                .italic(whoItalic)
                .ink(whoRole)
                .fixedSize(horizontal: false, vertical: true)
            if let meta {
                Text(meta).monoXS().ink(metaRole).lineLimit(1).layoutPriority(1)
            }
            Spacer(minLength: 0)
            if let when {
                Text(when).monoXS().ink(.secondary).lineLimit(1).fixedSize()
            }
            action()
        }
        .frame(minHeight: LogMetrics.headHeight)
    }

    @ViewBuilder
    private var rule: some View {
        if connector != .none {
            LogRule()
                .stroke(
                    DesignTokens.rule(.border, on: .window),
                    style: StrokeStyle(lineWidth: 1, dash: connector == .dashed ? [3, 3] : []))
                .frame(width: 1)
                .padding(.top, LogMetrics.ruleTop)
                .padding(.bottom, -(connector == .dashed ? LogMetrics.tailOverrun : LogMetrics.ruleOverrun))
                .offset(x: LogMetrics.ruleX)
                .allowsHitTesting(false)
                .accessibilityHidden(true)
        }
    }
}

extension LogItem where Action == EmptyView {
    init(
        symbol: String, iconRole: InkRole = .tertiary, glyph: AnyView? = nil, who: String,
        whoRole: InkRole = .primary, whoItalic: Bool = false,
        meta: String? = nil, metaRole: InkRole = .secondary, when: String? = nil,
        connector: LogConnector = .none, @ViewBuilder content: @escaping () -> Content
    ) {
        self.init(
            symbol: symbol, iconRole: iconRole, glyph: glyph, who: who, whoRole: whoRole, whoItalic: whoItalic,
            meta: meta, metaRole: metaRole, when: when, connector: connector, action: { EmptyView() },
            content: content)
    }
}

/// The log's rule: one vertical line down the middle of its frame.
private struct LogRule: Shape {
    func path(in rect: CGRect) -> Path {
        Path { path in
            path.move(to: CGPoint(x: rect.midX, y: rect.minY))
            path.addLine(to: CGPoint(x: rect.midX, y: rect.maxY))
        }
    }
}

/// A Thread fact's key: every card's key column takes the width of the
/// widest key any card can show (`widestThreadFactKey`), so every value in
/// the Thread starts at the same x. A longer key — a source's own label —
/// still draws whole, widening its own card's column.
struct ThreadFactKey: View {
    let text: String

    init(_ text: String) {
        self.text = text
    }

    var body: some View {
        ZStack(alignment: .leading) {
            Text(widestThreadFactKey).hidden()
            Text(text).ink(.tertiary)
        }
    }
}

// MARK: - Launch

/// The Task log's last item while the slice can be launched — not yet
/// anything that happened: what Launch (the section header's) will do, then
/// the model and effort it will run with, each a chip opening a menu, and
/// the base the worktree is cut from. Blocked, it is the same item quietened:
/// the chips disabled, and what it waits on said.
struct LaunchCard: View {
    enum Mode: Equatable {
        case launch
        /// A slice nat recorded a launch of, whose agent is gone
        /// (`launchIsRelaunch`).
        case relaunch
        /// An approved slice at its pull request: a fix agent, sent at the
        /// review on the same branch.
        case fix
        /// The dependencies still unfinished, by name.
        case blocked(waitingOn: [String])

        /// The Thread header's launch button, as this mode launches.
        var actionTitle: String {
            switch self {
            case .relaunch: return "Relaunch"
            case .fix: return "Launch fix agent"
            case .launch, .blocked: return "Launch"
            }
        }
    }

    let mode: Mode
    @Binding var model: String
    @Binding var effort: String
    let options: AgentOptions
    /// The branch the worktree is cut from, as nat resolves it; nil before
    /// the slice's detail is read, or with no repo to read it in.
    let base: String?

    private var blocked: Bool {
        if case .blocked = mode { return true }
        return false
    }

    var body: some View {
        LogItem(
            symbol: blocked ? "lock" : ThreadEventKind.launched.symbol,
            who: blocked ? "Blocked" : mode == .relaunch ? "Relaunch" : mode == .fix ? "Fix" : "Launch",
            whoRole: blocked ? .tertiary : .primary
        ) {
            VStack(alignment: .leading, spacing: 0) {
                explanation
                    .font(.system(size: Typo.scaled(13.5)))
                    .lineSpacing(2)
                    .fixedSize(horizontal: false, vertical: true)
                HStack(spacing: 6) {
                    NavFactMenu(value: model, placeholder: "default model") {
                        Button("Default") { model = "" }
                        ForEach(options.models, id: \.self) { option in Button(option) { model = option } }
                    }
                    .help("Model")
                    NavFactMenu(value: effort, placeholder: "default effort") {
                        Button("Default") { effort = "" }
                        ForEach(options.efforts, id: \.self) { option in Button(option) { effort = option } }
                    }
                    .help("Effort")
                }
                .disabled(blocked)
                .padding(.top, 8)
                if let base {
                    Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 3) {
                        GridRow {
                            ThreadFactKey("base")
                            Text(base)
                                .ink(blocked ? .tertiary : .primary)
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }
                    }
                    .monoXS()
                    .padding(.top, 8)
                }
            }
        }
    }

    @ViewBuilder
    private var explanation: some View {
        switch mode {
        case .launch:
            Text("Start an agent with this brief, in a worktree on a new branch. Its log appears here and its terminal opens on the right.")
                .ink(.secondary)
        case .relaunch:
            Text("No agent is running on this task. Relaunch to start a new agent on its branch that carries on from the work so far.")
                .ink(.secondary)
        case .fix:
            Text("The pull request is open. Launch a fix agent on its branch to answer the review and get the checks green; it hands back when the fix is pushed.")
                .ink(.secondary)
        case .blocked(let names):
            let waiting = names.isEmpty ? Text("its dependencies") : names.enumerated().reduce(Text("")) { text, entry in
                text + Text(entry.offset == 0 ? "" : ", ")
                    + Text(entry.element).foregroundStyle(DesignTokens.ink(.primary, on: .window))
            }
            (Text("Waits on ") + waiting
                + Text(names.count > 1 ? ". Launch unlocks when they are done." : ". Launch unlocks when that task is done."))
                .ink(.secondary)
        }
    }
}

// MARK: - Dependencies

/// One dependency in the brief's depends list: its dot and name, a click
/// selecting it, and its detail up the moment the pointer is over it.
struct DependencyRow: View {
    let slice: Slice
    let state: SliceDisplayState
    let live: Bool
    let milestone: String
    let onSelect: () -> Void

    @State private var hovering = false

    var body: some View {
        Button(action: {
            hovering = false
            onSelect()
        }) {
            HStack(spacing: 6) {
                StateDot(state: state, live: live, size: 6).frame(width: 10)
                Text(slice.name)
                    .ink(state == .done ? .secondary : .primary)
                    .underline(hovering)
                    .lineLimit(1)
                    .truncationMode(.tail)
                Spacer(minLength: 0)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .onHover { hovering = $0 }
        .quietPopover(isPresented: $hovering, arrowEdge: .maxX) {
            DependencyDetailView(slice: slice, state: state, live: live, milestone: milestone)
        }
        .accessibilityHint("Selects the task")
    }
}

/// A dependency's hover detail: its whole name, where it stands, its
/// milestone and pull request, and that a click goes to it.
struct DependencyDetailView: View {
    let slice: Slice
    let state: SliceDisplayState
    let live: Bool
    let milestone: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(slice.name)
                .font(.system(size: Typo.scaled(13), weight: .semibold))
                .ink(.primary)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 6) {
                StateDot(state: state, live: live, size: 6).frame(width: 10)
                Text(state.word).ink(.secondary)
            }
            Grid(alignment: .leading, horizontalSpacing: 10, verticalSpacing: 3) {
                GridRow {
                    Text("milestone").ink(.tertiary)
                    Text(milestone.isEmpty ? "none" : milestone).ink(.primary).lineLimit(1)
                }
                if let number = pullRequestNumber(slice.pr) {
                    GridRow {
                        Text("pr").ink(.tertiary)
                        Text("#\(number)").ink(.primary)
                    }
                }
                if !slice.assignee.isEmpty {
                    GridRow {
                        Text("assignee").ink(.tertiary)
                        Text(slice.assignee).ink(.primary).lineLimit(1)
                    }
                }
            }
            .monoXS()
            Text("Click to open the task.")
                .font(.system(size: Typo.subhead))
                .ink(.tertiary)
        }
        .font(.system(size: Typo.scaled(13)))
        .padding(12)
        .frame(width: 260, alignment: .leading)
    }
}
