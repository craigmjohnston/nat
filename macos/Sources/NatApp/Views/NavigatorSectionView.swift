import SwiftUI
import NatKit

/// One of the navigator's stacked foldouts: a 32pt header — chevron, label,
/// and the section's own actions flush against its trailing edge — and its
/// rule, over a
/// body on the window's ground that takes an equal share of what is left
/// while open. A section with nothing to show yet is not drawn at all — the
/// navigator leaves it out.
///
/// The chevron only folds (`onFold`); the rest of the header is `onHead`,
/// which may also put the section's view up in the main pane. The body is
/// built once and kept while folded — collapsed to nothing rather than torn
/// down — so unfolding it again redraws nothing and reloads nothing: its
/// scroll position, its reads and its rendered markdown are all as left.
struct NavSectionView<Actions: View, Content: View>: View {
    let label: String
    let open: Bool
    var selected = false
    /// The design's quiet meta beside the label, drawn only while folded — a
    /// source section's count.
    var meta: String?
    /// A status badge just after the label, drawn open or folded — the PR
    /// section's Merged.
    var status: NavSectionStatus?
    /// Reworking, before the status: the agent is at this work again
    /// (`NavigatorModel.showsReworking`). The text is its tooltip; nil draws
    /// nothing.
    var reworking: String?
    /// Conflict, after the status: the section's branch conflicts with its
    /// base — a danger badge in the merge glyph the sidebar's conflict mark
    /// draws. The text is its tooltip; nil draws nothing.
    var conflict: String?
    /// A warning after the label and status, drawn open or folded as a small
    /// danger icon whose tooltip is its text — the PR section's failing
    /// checks. Nil draws nothing.
    var warning: String?
    /// A success mark in the warning's slot, drawn only where there is no
    /// warning — the PR section's passing checks. Its tooltip is its text.
    var passing: String?
    /// The running mark in the same slot, drawn only where there is neither a
    /// warning nor a passing mark — the PR section's checks still running. Its
    /// tooltip is its text.
    var running: String?
    let onHead: () -> Void
    var onFold: (() -> Void)?
    @ViewBuilder var actions: () -> Actions
    @ViewBuilder var content: () -> Content

    var body: some View {
        VStack(spacing: 0) {
            header
            // Under the band, not inside it — open or folded — so the band is
            // the full height the sidebar's and the main pane's headings are.
            DesignTokens.rule(.separator, on: .chrome).frame(height: 1)
            content()
                .frame(maxWidth: .infinity, maxHeight: open ? .infinity : 0, alignment: .top)
                .surface(.window)
                .clipped()
                .opacity(open ? 1 : 0)
                .allowsHitTesting(open)
                .accessibilityHidden(!open)
        }
        .frame(maxHeight: open ? .infinity : nil)
        .overlay(alignment: .bottom) { DesignTokens.rule(.separator, on: .chrome).frame(height: 1) }
    }

    private var header: some View {
        HStack(spacing: 8) {
            DisclosureChevron(open: open)
                // Turned at once, as the section snaps.
                .transaction { $0.animation = nil }
                .frame(width: 24, height: GnatMetrics.sectionHeadHeight)
                .padding(.horizontal, -6)
                .contentShape(Rectangle())
                .onTapGesture { (onFold ?? onHead)() }
            HStack(spacing: 6) {
                Text(label)
                    .font(.system(size: GnatMetrics.body))
                    .ink(.primary)
                    // A label longer than the column ("Visual changes") takes
                    // the room it needs rather than truncating.
                    .fixedSize(horizontal: true, vertical: false)
                // Reworking, then the status — badges just after the label.
                if let reworking {
                    Chip(
                        NavigatorModel.reworkingLabel, tone: .warning, size: .small,
                        systemImage: NavigatorModel.reworkingSymbol
                    )
                    .fixedSize()
                    .help(reworking)
                    .accessibilityLabel(reworking)
                }
                if let status {
                    Chip(status.label, tone: status.tone, size: .small).fixedSize()
                }
                if let conflict {
                    Chip(NavigatorModel.conflictLabel, tone: .danger, size: .small) {
                        MergeIcon(size: 10, lineWidth: 1.3)
                    }
                    .fixedSize()
                    .help(conflict)
                    .accessibilityLabel(conflict)
                }
                if let warning {
                    Image(systemName: "xmark.octagon")
                        .font(.system(size: 12, weight: .medium))
                        .ink(.danger)
                        .help(warning)
                } else if let passing {
                    Image(systemName: "checkmark.circle")
                        .font(.system(size: 12, weight: .medium))
                        .ink(.success)
                        .help(passing)
                        .accessibilityLabel(passing)
                } else if let running {
                    Image(systemName: PRMarks.runningOutlineSymbol)
                        .font(.system(size: 12, weight: .medium))
                        .ink(.secondary)
                        .help(running)
                        .accessibilityLabel(running)
                }
            }
            .frame(minWidth: 58, alignment: .leading)
            if status == nil, reworking == nil, conflict == nil, let meta, !open {
                Text(meta).monoXS().ink(.tertiary).lineLimit(1)
            }
            Spacer(minLength: 0)
            HStack(spacing: 0) { actions() }
                .frame(maxHeight: .infinity)
        }
        .padding(.leading, 10)
        .frame(height: GnatMetrics.sectionHeadHeight)
        .background(selected ? DesignTokens.rowWash(selected: true, on: .chrome) : DesignTokens.fill(.chrome))
        .contentShape(Rectangle())
        .onTapGesture { onHead() }
    }
}

extension NavSectionView where Actions == EmptyView {
    init(
        label: String, open: Bool, selected: Bool = false, meta: String? = nil, status: NavSectionStatus? = nil,
        reworking: String? = nil, warning: String? = nil, passing: String? = nil, onHead: @escaping () -> Void,
        onFold: (() -> Void)? = nil, @ViewBuilder content: @escaping () -> Content
    ) {
        self.init(
            label: label, open: open, selected: selected, meta: meta, status: status, reworking: reworking,
            warning: warning,
            passing: passing, onHead: onHead, onFold: onFold,
            actions: { EmptyView() }, content: content)
    }
}

extension NavSectionStatus {
    var tone: Tone {
        switch self {
        // New wears Merged's own badge; Updated the accent, so the two read
        // apart at a glance.
        case .merged, .new: return .success
        case .updated: return .accent
        }
    }
}

/// An item's New or Updated badge — a Changes file's row, a Visual changes
/// image's row and header — in the section header's own chip.
struct SeenBadgeChip: View {
    let badge: SeenBadge

    var body: some View {
        let status = NavSectionStatus(badge) ?? .new
        Chip(status.label, tone: status.tone, size: .small).fixedSize()
    }
}

/// The navigator column: whatever sections the selection has, and a filler
/// taking the column's slack when every section is folded — then, pinned to
/// the column's foot under them all, its `footer` (a slice's action bar),
/// where it has one. Its title is the window titlebar's, drawn by the shell.
struct NavigatorColumn<Sections: View, Footer: View>: View {
    var anyOpen: Bool
    @ViewBuilder var sections: () -> Sections
    @ViewBuilder var footer: () -> Footer

    var body: some View {
        VStack(spacing: 0) {
            VStack(spacing: 0) {
                sections()
                if !anyOpen {
                    Spacer(minLength: 0)
                }
            }
            .frame(maxHeight: .infinity, alignment: .top)
            footer()
        }
        .frame(maxHeight: .infinity, alignment: .top)
        .rule(.separator, edges: [.trailing], width: 1)
        // The last section's bottom line sits on the status bar's own
        // top line rather than one point above it, so the two are one.
        .padding(.bottom, -1)
        .surface(.chrome)
    }
}

extension NavigatorColumn where Footer == EmptyView {
    init(anyOpen: Bool, @ViewBuilder sections: @escaping () -> Sections) {
        self.init(anyOpen: anyOpen, sections: sections, footer: { EmptyView() })
    }
}

/// The slice navigator's action bar: a row of header-band height on the
/// chrome, pinned to the column's foot — no title, no chevron, no fold, no
/// body — its buttons flush against its trailing edge (`content`), and a
/// rule over it. The rule is drawn a point up, over the last section's own
/// bottom rule where one sits right above, so the two are one line.
struct NavigatorActionBar<Content: View>: View {
    @ViewBuilder var content: () -> Content

    var body: some View {
        HStack(spacing: 0) {
            Spacer(minLength: 0)
            content()
        }
        .frame(height: GnatMetrics.sectionHeadHeight)
        .frame(maxWidth: .infinity)
        .background(DesignTokens.fill(.chrome))
        .overlay(alignment: .top) {
            DesignTokens.rule(.separator, on: .chrome).frame(height: 1).offset(y: -1)
        }
    }
}

/// A navigator body's prose block: the design's `.prose` padding and type.
struct NavProse<Content: View>: View {
    @ViewBuilder var content: () -> Content

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            content()
        }
        .font(.system(size: GnatMetrics.body))
        .lineSpacing(3)
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// The design's small uppercase heading inside a section body.
struct NavHeading: View {
    let text: String

    var body: some View {
        Text(text.uppercased())
            .font(.system(size: Typo.subhead))
            .tracking(0.7)
            .ink(.secondary)
            .padding(.top, 4)
    }
}

/// One recorded item of the Task log (`LogItem`): who, then what they did in
/// its tone, and at the header's end when where nat knows; then the body,
/// cut short as the brief is; then its labelled facts, on the same ground. A
/// live one (`ThreadEvent.isLive`) has its icon in its hue; the log's rule
/// runs on from it as `connector` says.
///
/// A `collapsible` one (a quiet kind, `ThreadEvent.isCollapsible`) starts
/// folded to its header — icon, title, time — and its header row opens and
/// folds it on a click, its icon a chevron while the pointer is over it.
struct ThreadEventCard: View {
    let event: ThreadEvent
    var connector: LogConnector = .none
    /// Whether the item folds to its header. One with nothing under its
    /// header to show never does: there would be nothing to open.
    var collapsible = false
    @Environment(\.clock) private var clock
    @Environment(\.threadFoldsOpen) private var foldsOpen
    @Environment(\.hoverForced) private var hoverForced
    /// Draws a fact that names a slice (`ThreadFact.sliceID`) as a task row
    /// — the slice navigator's, which has the plan to draw one from. Nil
    /// from it (or no closure at all, as a session's Thread has) and the
    /// fact is plain text.
    var taskRow: ((String) -> AnyView?)?
    /// Whether the user has opened (or folded) the item; nil until they
    /// have, when `threadFoldsOpen` says.
    @State private var expanded: Bool?
    @State private var hovering = false

    private var folds: Bool { collapsible && (event.body != nil || !event.facts.isEmpty) }
    private var isOpen: Bool { !folds || (expanded ?? foldsOpen) }

    var body: some View {
        LogItem(
            // An action reads as one plain sentence ("Agent handed back"); a
            // meta that is a separate fact (a comment's time) stays beside it.
            symbol: event.kind.symbol, iconRole: iconRole,
            glyph: folds && (hovering || hoverForced) ? AnyView(DisclosureChevron(open: isOpen)) : nil,
            who: event.metaIsAction ? event.title : event.who, meta: event.metaIsAction ? nil : event.meta,
            when: event.when.map { threadTimestamp($0, now: clock()) }, connector: connector
        ) {
            if isOpen { details }
        }
        .overlay(alignment: .top) {
            if folds {
                LogFoldButton(open: isOpen, hovering: $hovering) {
                    withAnimation(Motion.stateChange) { expanded = !isOpen }
                }
            }
        }
    }

    /// Everything under the header: the body, then the facts.
    private var details: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let body = event.body {
                Excerpt(text: body) { shown in
                    Text(markdownAttributed(shown, size: Typo.scaled(13.5)))
                        .font(.system(size: Typo.scaled(13.5)))
                        .lineSpacing(2)
                        .ink(.primary)
                        .textSelection(.enabled)
                }
            }
            if !event.facts.isEmpty {
                // Labelled values, as the brief's own facts are drawn.
                Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 3) {
                    ForEach(Array(event.facts.enumerated()), id: \.offset) { _, fact in
                        GridRow {
                            ThreadFactKey(fact.key)
                            if let id = fact.sliceID, let row = taskRow?(id) {
                                row
                            } else {
                                Text(fact.value)
                                    .ink(.primary)
                                    .lineLimit(1)
                                    .truncationMode(.middle)
                            }
                        }
                    }
                }
                .monoXS()
                .padding(.top, event.body == nil ? 2 : 8)
            }
        }
    }

    /// A failure in the danger ink; a live item's in its own hue; the rest
    /// quiet.
    private var iconRole: InkRole {
        if event.kind == .checksFailed { return .danger }
        if event.isLive { return tone }
        return .tertiary
    }

    private var tone: InkRole {
        switch event.tone {
        case .muted: return .secondary
        case .accent: return .accent
        case .hot: return .hot
        }
    }
}

/// A folding log item's header row as one button, drawn over the row and
/// invisible: a click anywhere on it opens or folds the item, and the pointer
/// over it (`hovering`) is what turns the item's icon into its chevron.
struct LogFoldButton: View {
    let open: Bool
    @Binding var hovering: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Color.clear
                .frame(maxWidth: .infinity)
                .frame(height: LogMetrics.headHeight)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .onHover { hovering = $0 }
        .accessibilityLabel(open ? "Fold" : "Open")
    }
}

/// A run of quiet log items folded into one (`ThreadLogItem.group`): a
/// stacked icon, how many items it holds, in italic, and when the first and
/// the last of them happened. Opened, its items sit in one quiet recessed well
/// under the header — a tinted, rounded fill inset from the log's rule, which
/// runs on past it unbroken — each a step in from the header and folded as it
/// would be on its own, and the well's last line is the control that folds
/// the group again, "Hide 5 items", so it shuts from below. Folding from there
/// keeps the group's header in view: the log scrolls back to it where it had
/// gone off the top.
struct ThreadGroupCard: View {
    let events: [ThreadEvent]
    var connector: LogConnector = .none
    var taskRow: ((String) -> AnyView?)?
    @Environment(\.clock) private var clock
    @Environment(\.threadFoldsOpen) private var foldsOpen
    @Environment(\.hoverForced) private var hoverForced
    /// Whether the user has opened (or folded) the group; nil until they
    /// have, when `threadFoldsOpen` says.
    @State private var expanded: Bool?
    @State private var hoveringHead = false
    @State private var hoveringFoot = false
    /// Whether the header is on screen, for the foot's fold to know whether
    /// to bring it back.
    @State private var headVisible = true
    /// The header's scroll anchor.
    @State private var headID = UUID()

    private var isOpen: Bool { expanded ?? foldsOpen }

    var body: some View {
        ScrollViewReader { proxy in
            if isOpen {
                VStack(alignment: .leading, spacing: 6) {
                    head(connector: .none)
                    well { foldFromFoot(proxy) }
                }
                // The log's rule from the header's icon down past the well to
                // the next item, unbroken.
                .overlay(alignment: .topLeading) { LogConnectorRule(connector: connector) }
            } else {
                head(connector: connector)
            }
        }
    }

    private func toggle() {
        withAnimation(Motion.stateChange) { expanded = !isOpen }
    }

    /// Folds the group from its foot, bringing its header back on screen
    /// where it had scrolled off — so the log does not leave the reader past
    /// a group that has just shrunk to one line.
    private func foldFromFoot(_ proxy: ScrollViewProxy) {
        let bringBack = !headVisible
        withAnimation(Motion.stateChange) {
            expanded = false
            if bringBack { proxy.scrollTo(headID, anchor: .top) }
        }
    }

    private func head(connector: LogConnector) -> some View {
        LogItem(
            symbol: "",
            glyph: hoveringHead || hoverForced ? AnyView(DisclosureChevron(open: isOpen)) : AnyView(stackedIcon),
            who: threadGroupTitle(count: events.count), whoRole: .secondary, whoItalic: true,
            when: threadTimestampRange(events.compactMap(\.when), now: clock()), connector: connector
        ) {
            EmptyView()
        }
        .overlay(alignment: .top) {
            LogFoldButton(open: isOpen, hovering: $hoveringHead, action: toggle)
        }
        .id(headID)
        .onScrollVisibilityChange(threshold: 0.5) { headVisible = $0 }
    }

    /// The open group's items, each folded as it would be on its own, then
    /// its foot, on the recessed ground.
    private func well(fold: @escaping () -> Void) -> some View {
        VStack(alignment: .leading, spacing: LogMetrics.wellSpacing) {
            ForEach(Array(events.enumerated()), id: \.offset) { _, event in
                ThreadEventCard(event: event, collapsible: true, taskRow: taskRow)
            }
            foot(fold: fold)
        }
        // Its items start folded, whatever opened the group.
        .environment(\.threadFoldsOpen, false)
        .padding(LogMetrics.wellPadding)
        .frame(maxWidth: .infinity, alignment: .leading)
        .surface(.rowAlt, radius: LogMetrics.wellRadius)
        .padding(.leading, LogMetrics.wellInset)
    }

    /// The well's last line: what folding does, in the header's own italic
    /// secondary, lit under the pointer as the log's other fold controls are,
    /// under the items' titles.
    private func foot(fold: @escaping () -> Void) -> some View {
        Button(action: fold) {
            Text(threadGroupFoldTitle(count: events.count))
                .font(.system(size: Typo.scaled(13.5)))
                .italic()
                .ink(hoveringFoot || hoverForced ? .primary : .secondary)
                .frame(minHeight: LogMetrics.headHeight)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .onHover { hoveringFoot = $0 }
        .padding(.leading, LogMetrics.margin)
        .accessibilityLabel("Fold \(threadGroupTitle(count: events.count))")
    }

    /// The first two kinds of item the group holds, one behind the other:
    /// the back one up and to the right, a step quieter, the front one on a
    /// disc of the log's own ground so only the back one's edge shows.
    private var stackedIcon: some View {
        let symbols = events.map(\.kind.symbol).reduce(into: [String]()) { seen, symbol in
            if !seen.contains(symbol) { seen.append(symbol) }
        }
        return ZStack {
            if symbols.count > 1 {
                ThreadIcon(symbol: symbols[1], role: .quaternary).offset(x: 3, y: -3)
            }
            ThreadIcon(symbol: symbols.first ?? "", role: .tertiary)
                .background(Circle().fill(DesignTokens.fill(.window)).padding(-1.5))
        }
    }
}

/// A launch choice the user can change, as a chip — the design's `.chip`, a
/// bordered pill — holding the value and a small caret, opening a menu of the
/// choices: the launch item's model and effort. An empty value reads
/// `placeholder`, a step quieter than a chosen one. Disabled, it is the same
/// chip with the value in the quaternary ink and no caret.
struct NavFactMenu<Items: View>: View {
    @Environment(\.isEnabled) private var isEnabled
    let value: String
    var placeholder = "default"
    @ViewBuilder var items: () -> Items

    var body: some View {
        if isEnabled {
            Menu {
                items()
            } label: {
                HStack(spacing: 4) {
                    text
                    Image(systemName: "chevron.down")
                        .font(.system(size: 8, weight: .semibold))
                        .ink(.tertiary)
                }
                .chip()
                .hoverWash(cornerRadius: 5)
                .contentShape(Rectangle())
            }
            .menuStyle(.button)
            .buttonStyle(.plain)
            .menuIndicator(.hidden)
            .fixedSize()
        } else {
            // Disabled, a plain chip: a disabled Menu fades its label on top
            // of any ink, which would leave it fainter than meant.
            text.chip().fixedSize()
        }
    }

    private var text: some View {
        Text(value.isEmpty ? placeholder : value)
            .ink(isEnabled ? (value.isEmpty ? .tertiary : .secondary) : .quaternary)
            .lineLimit(1)
    }
}

private extension View {
    /// The design's `.chip`: mono xs in a pill bordered by the stronger rule.
    func chip() -> some View {
        monoXS()
            .padding(.horizontal, 8)
            .padding(.vertical, 2)
            .overlay {
                RoundedRectangle(cornerRadius: 5).strokeBorder(DesignTokens.rule(.border, on: .window), lineWidth: 1)
            }
    }
}

/// A one-line notice in a section body: a refusal, a warning, a stale read.
struct NavNotice: View {
    let text: String
    var role: InkRole = .danger

    var body: some View {
        Text(text)
            .font(.system(size: Typo.scaled(13)))
            .ink(role)
            .fixedSize(horizontal: false, vertical: true)
            .textSelection(.enabled)
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}
