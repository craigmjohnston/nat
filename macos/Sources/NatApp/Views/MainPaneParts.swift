import AppKit
import SwiftUI
import NatKit

/// The one titlebar band over the navigator and the main pane — neither has
/// a heading band of its own, and no rule divides the band where the two
/// columns meet. The selection's identity starts at the navigator's leading
/// inset and may run on past its width; the live agent's readout or the
/// view's own actions stand at the band's trailing edge, the main pane's
/// tabs (`MainPaneTab`) just left of them. The tabs and those items live in
/// the main pane's part of the band alone (`TitlebarBandLayout`): a title
/// with no room left ellipsizes, and a main pane narrower than the run cuts
/// the run at its leading edge rather than letting it cross the split.
struct TitlebarBand<Identity: View, Trailing: View>: View {
    /// The navigator's width: the band's main-pane part is what is left.
    let navigatorWidth: Double
    var tabs: [MainPaneTab] = []
    var selected: MainPaneMode?
    var onTab: (MainPaneTab) -> Void = { _ in }
    @ViewBuilder var identity: () -> Identity
    @ViewBuilder var trailing: () -> Trailing

    var body: some View {
        GnatTitlebar(leading: 0, trailing: 0, rule: false) {
            TitlebarBandStack(navigatorWidth: navigatorWidth) {
                identity()
                    .padding(.horizontal, 10)
                HStack(spacing: 0) {
                    HStack(spacing: 0) {
                        ForEach(tabs, id: \.self) { tab in
                            MainPaneTabButton(title: tab.label, selected: tab.mode == selected) { onTab(tab) }
                        }
                    }
                    // Each tab's line is on its leading edge; this closes
                    // the run off from the readout beside it.
                    .overlay(alignment: .trailing) {
                        if !tabs.isEmpty {
                            DesignTokens.rule(.separator, on: .header).frame(width: 1)
                        }
                    }
                    TrailingItemsStack { trailing() }
                }
                .fixedSize(horizontal: true, vertical: false)
                // Exactly the room the band gives it, the run against its
                // trailing edge and anything past its leading edge cut.
                .frame(minWidth: 0, maxWidth: .infinity, alignment: .trailing)
                .clipped()
            }
            .frame(maxHeight: .infinity)
            // The band's line, behind the tabs: the picked one's own ground
            // covers it, so it stands open into the pane below.
            .background(alignment: .bottom) {
                DesignTokens.rule(.separator, on: .header).frame(height: 1).allowsHitTesting(false)
            }
        }
    }
}

extension TitlebarBand where Identity == EmptyView, Trailing == EmptyView {
    init(navigatorWidth: Double) {
        self.init(navigatorWidth: navigatorWidth, identity: { EmptyView() }, trailing: { EmptyView() })
    }
}

/// The band's trailing items in a row, 8pt apart and inset 12pt either
/// side — or nothing at all where none draws anything (a readout with no
/// reading yet), so the tabs then stand flush against the band's edge.
private struct TrailingItemsStack: Layout {
    static let spacing: CGFloat = 8
    static let inset: CGFloat = 12

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let sizes = shownSizes(subviews, height: proposal.height)
        guard !sizes.isEmpty else { return CGSize(width: 0, height: proposal.height ?? 0) }
        let width = sizes.reduce(0) { $0 + $1.width } + Self.spacing * CGFloat(sizes.count - 1) + Self.inset * 2
        return CGSize(width: width, height: proposal.height ?? sizes.map(\.height).max() ?? 0)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var x = bounds.minX + Self.inset
        for subview in subviews {
            let size = subview.sizeThatFits(ProposedViewSize(width: nil, height: bounds.height))
            guard size.width > 0 else { continue }
            subview.place(
                at: CGPoint(x: x, y: bounds.midY), anchor: .leading,
                proposal: ProposedViewSize(width: size.width, height: bounds.height))
            x += size.width + Self.spacing
        }
    }

    private func shownSizes(_ subviews: Subviews, height: CGFloat?) -> [CGSize] {
        subviews.map { $0.sizeThatFits(ProposedViewSize(width: nil, height: height)) }.filter { $0.width > 0 }
    }
}

/// The band's two parts laid out as `TitlebarBandLayout` places them: the
/// identity from the leading edge, offered the room up to the run; the run
/// offered what of the main pane's part it takes, against the trailing edge.
private struct TitlebarBandStack: Layout {
    let navigatorWidth: Double

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        CGSize(width: proposal.width ?? navigatorWidth, height: proposal.height ?? GnatMetrics.titlebarHeight)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        guard subviews.count == 2 else { return }
        let run = subviews[1].sizeThatFits(ProposedViewSize(width: nil, height: bounds.height)).width
        let layout = TitlebarBandLayout(bandWidth: bounds.width, navigatorWidth: navigatorWidth, runWidth: run)
        subviews[0].place(
            at: CGPoint(x: bounds.minX, y: bounds.midY), anchor: .leading,
            proposal: ProposedViewSize(width: layout.identityWidth, height: bounds.height))
        subviews[1].place(
            at: CGPoint(x: bounds.minX + layout.runX, y: bounds.minY), anchor: .topLeading,
            proposal: ProposedViewSize(width: layout.runShownWidth, height: bounds.height))
    }
}

/// The selection as the titlebar band names it — its Active row's dot,
/// project tag and title, or the bare title where it has none — and the
/// chevron that says it opens the tree picker. As room runs out the title
/// alone gives way, ending in an ellipsis with the chevron still beside it.
struct TitlebarIdentityLabel: View {
    let identity: TitlebarIdentity?
    let title: String

    var body: some View {
        HStack(spacing: 5) {
            Group {
                if let identity, let icon = identity.icon {
                    SourceIdentityLabel(
                        icon: icon, tag: identity.tag, title: identity.title, size: GnatMetrics.titlebarText)
                } else if let identity {
                    ActiveIdentityLabel(
                        tag: identity.tag, state: identity.state, live: identity.live, title: identity.title,
                        size: GnatMetrics.titlebarText, titleInk: .primary)
                } else {
                    Text(title).ink(.primary)
                }
            }
            .layoutPriority(-1)
            Image(systemName: "chevron.down")
                .font(.system(size: 9, weight: .semibold))
                .ink(.tertiary)
                .fixedSize()
        }
        .font(.system(size: GnatMetrics.titlebarText))
        .lineLimit(1)
        .truncationMode(.tail)
    }
}

/// The agent readout, at the titlebar band's trailing edge beside the tabs,
/// kept small: the live agent's model, its effort quieter, then its context
/// use as a bare percent as its own statusline reports it — in the warning
/// tint once it runs high — or nothing. The long form is its tooltip.
struct AgentModelHeading: View {
    let agent: AgentStatus?

    var body: some View {
        if let readout = buildAgentReadout(from: agent) {
            HStack(spacing: 6) {
                if let model = readout.model { Text(model).ink(.secondary) }
                if let effort = readout.effort { Text(effort).ink(.tertiary) }
                if let context = readout.context {
                    Text(context.text).ink(context.warning ? .hot : .tertiary)
                }
            }
            .font(Typo.mono(size: 11))
            .monospacedDigit()
            .lineLimit(1)
            .fixedSize()
            .help(readout.detail)
        }
    }
}

/// The PR view's action, at the titlebar band's trailing edge: Open in GitHub, once
/// the right pull request is read (`expectedNumber`, as
/// `PRConversationPane` checks it).
struct PROpenInGitHubButton: View {
    let store: PRStore
    let expectedNumber: Int?

    var body: some View {
        if let pr = store.loadState.pr, expectedNumber == nil || pr.number == expectedNumber {
            Button {
                if let url = URL(string: pr.url) { NSWorkspace.shared.open(url) }
            } label: {
                HeaderActionLabel(title: "Open in GitHub", systemImage: "arrow.up.right.square")
            }
            .buttonStyle(GnatHeaderButtonStyle())
            .help("Open the pull request on GitHub")
            // Flush with the band's edge, as a navigator header's actions are.
            .padding(.trailing, -12)
        }
    }
}

/// The size the PR view sets its prose in — the description and every
/// comment alike.
enum PRConversationMetrics {
    static let textSize: CGFloat = 15
}

/// A pull request's description and conversation, with the comment box at
/// the end — the main pane's half of the PR; its checks and verdict are the
/// navigator's. `expectedNumber` keeps a reading of some other pull request
/// (the store is shared across the project) from showing while the right
/// one is read.
struct PRConversationPane: View {
    let store: PRStore
    let expectedNumber: Int?

    @State private var commentText = ""
    @State private var isSending = false
    @State private var commentError: String?

    var body: some View {
        if let pr = store.loadState.pr, expectedNumber == nil || pr.number == expectedNumber {
            content(pr)
                .onChange(of: pr.number) { _, _ in
                    commentText = ""
                    commentError = nil
                }
        } else if let message = store.loadState.errorMessage {
            MainPaneNote(text: "The pull request could not be read — \(message)")
        } else {
            QuietLoadingView(label: "Reading the pull request")
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private func content(_ pr: PRDetail) -> some View {
        let entries = conversation(comments: pr.comments, reviews: pr.reviews)
        let described = pr.body.trimmingCharacters(in: .whitespacesAndNewlines)
        return ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Text(pr.title).ink(.primary)
                    Text("#\(pr.number)").ink(.tertiary).fixedSize()
                }
                .font(.system(size: 20, weight: .semibold))
                .textSelection(.enabled)
                NavHeading(text: "Description")
                if described.isEmpty {
                    Text("No description.").font(.system(size: PRConversationMetrics.textSize)).ink(.secondary)
                } else {
                    Excerpt(text: described, maxWords: briefExcerptWords * 3) { shown in
                        MarkdownView(text: shown, size: PRConversationMetrics.textSize)
                    }
                }

                NavHeading(text: entries.isEmpty ? "Conversation" : "Conversation · \(entries.count)")
                if entries.isEmpty {
                    Text("No comments yet.").font(.system(size: PRConversationMetrics.textSize)).ink(.secondary)
                }
                ForEach(Array(entries.enumerated()), id: \.offset) { _, entry in
                    PRConversationEntryView(entry: entry)
                }
                if pr.state != PRLifecycleState.merged && pr.state != PRLifecycleState.closed {
                    PRComposerView(
                        placeholder: "Comment on the pull request\u{2026}",
                        text: $commentText, isSending: isSending, error: commentError,
                        onSend: { Task { await send() } })
                }
            }
            .padding(20)
            .frame(maxWidth: 820, alignment: .leading)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .thinScrollers()
    }

    private func send() async {
        isSending = true
        commentError = nil
        do {
            try await store.comment(text: commentText)
            commentText = ""
        } catch {
            commentError = SliceActionTracker.message(for: error)
        }
        isSending = false
    }
}

// MARK: - Markdown with tables

/// Markdown as `markdownAttributed` draws it, with its tables drawn as
/// tables — `Text` has no way to lay one out.
struct MarkdownView: View {
    let text: String
    let size: CGFloat
    var ink: InkRole = .primary

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(Array(markdownBlocks(text).enumerated()), id: \.offset) { _, block in
                switch block {
                case .text(let prose):
                    // The blank lines around a table are the stack's spacing.
                    let prose = prose.trimmingCharacters(in: .newlines)
                    if !prose.isEmpty {
                        Text(markdownAttributed(prose, size: size))
                        .font(.system(size: size))
                        .lineSpacing(2)
                        .ink(ink)
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                case .table(let table):
                    MarkdownTableView(table: table, size: size)
                case .details(let details):
                    MarkdownDetailsView(details: details, size: size, ink: ink)
                }
            }
        }
    }
}

/// A `<details>` fold as GitHub draws it: a disclosure triangle and the
/// summary, the folded markdown under it, indented, once opened.
struct MarkdownDetailsView: View {
    let details: MarkdownDetails
    let size: CGFloat
    let ink: InkRole

    @State private var isOpen: Bool

    init(details: MarkdownDetails, size: CGFloat, ink: InkRole) {
        self.details = details
        self.size = size
        self.ink = ink
        _isOpen = State(initialValue: details.open)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Button {
                withAnimation(Motion.stateChange) { isOpen.toggle() }
            } label: {
                HStack(alignment: .firstTextBaseline, spacing: 6) {
                    Image(systemName: "chevron.right")
                        .font(.system(size: size - 4, weight: .semibold))
                        .ink(.tertiary)
                        .rotationEffect(.degrees(isOpen ? 90 : 0))
                    Text(markdownAttributed(details.summary, size: size))
                        .font(.system(size: size, weight: .medium))
                        .ink(ink)
                        .multilineTextAlignment(.leading)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .help(isOpen ? "Fold" : "Show the details")

            if isOpen && !details.body.isEmpty {
                MarkdownView(text: details.body, size: size, ink: ink)
                    .padding(.leading, size + 2)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// A table, scrolled sideways when it is wider than its room. A column
/// wider than its fair share and a third of the room is cut short
/// (`tableColumnWidths`), its heading carrying an expand mark that gives it
/// its whole width back — and, expanded, a mark to cut it again.
struct MarkdownTableView: View {
    let table: MarkdownTable
    let size: CGFloat

    @State private var available: CGFloat = 0
    @State private var expanded: Set<Int>
    /// Each column's widest cell as SwiftUI actually lays it out, read off a
    /// hidden copy of the column — code spans, bold and emoji included,
    /// which a plain-font measurement misses and then cuts with no mark.
    @State private var measured: [Int: CGFloat] = [:]

    /// `initiallyExpanded` is the gallery's seam: a story seeds the columns
    /// it is a story about.
    init(table: MarkdownTable, size: CGFloat, initiallyExpanded: Set<Int> = []) {
        self.table = table
        self.size = size
        _expanded = State(initialValue: initiallyExpanded)
    }

    /// What each column would take with nothing cut: its widest cell as
    /// drawn (`measured`), plus the cell's own padding — until that is read,
    /// the plain text set in the system face.
    private var naturalWidths: [Double] {
        let body = NSFont.systemFont(ofSize: size)
        let heading = NSFont.systemFont(ofSize: size, weight: .semibold)
        return table.header.indices.map { column in
            if let width = measured[column] { return Double(ceil(width)) + Self.cellPadding * 2 }
            let cells = [(table.header[column], heading)] + table.rows.map { ($0[column], body) }
            let widest = cells.map { cell, font in
                (String(markdownAttributed(cell, size: size).characters) as NSString)
                    .size(withAttributes: [.font: font]).width
            }.max() ?? 0
            return Double(ceil(widest)) + Self.cellPadding * 2
        }
    }

    /// Every column laid out at its ideal width, unseen, so `measured` reads
    /// what each cell really takes.
    private var measurer: some View {
        HStack(alignment: .top, spacing: 0) {
            ForEach(table.header.indices, id: \.self) { column in
                VStack(alignment: .leading, spacing: 0) {
                    Text(markdownAttributed(table.header[column], size: size))
                        .font(.system(size: size, weight: .semibold))
                    ForEach(table.rows.indices, id: \.self) { row in
                        Text(markdownAttributed(table.rows[row][column], size: size))
                            .font(.system(size: size))
                    }
                }
                .lineLimit(1)
                .fixedSize()
                .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { measured[column] = $0 }
            }
        }
        .fixedSize()
        .hidden()
        .accessibilityHidden(true)
    }

    static let cellPadding: Double = 8
    static let markWidth: Double = 18

    var body: some View {
        let widths = tableColumnWidths(natural: naturalWidths, available: Double(available), expanded: expanded)
        ScrollView(.horizontal) {
            Grid(alignment: .leading, horizontalSpacing: 0, verticalSpacing: 0) {
                GridRow {
                    ForEach(table.header.indices, id: \.self) { column in
                        headingCell(column, width: widths[column])
                    }
                }
                .background(DesignTokens.fill(.chrome))
                ForEach(table.rows.indices, id: \.self) { row in
                    // Outside any GridRow, so it spans every column.
                    DesignTokens.rule(.separator, on: .window).frame(height: 1)
                    GridRow {
                        ForEach(table.header.indices, id: \.self) { column in
                            cell(table.rows[row][column], column: column, width: widths[column])
                        }
                    }
                }
            }
            .overlay {
                RoundedRectangle(cornerRadius: 4).strokeBorder(DesignTokens.rule(.separator, on: .window), lineWidth: 1)
            }
            .clipShape(RoundedRectangle(cornerRadius: 4))
        }
        .thinScrollers(.horizontal)
        .scrollBounceBehavior(.basedOnSize, axes: .horizontal)
        .background(alignment: .topLeading) { measurer.frame(width: 0, height: 0, alignment: .topLeading).clipped() }
        .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { available = $0 }
    }

    private func width(_ column: TableColumnWidth, index: Int) -> CGFloat {
        CGFloat(column.width + (expanded.contains(index) ? Self.markWidth : 0))
    }

    private func headingCell(_ column: Int, width: TableColumnWidth) -> some View {
        let isExpanded = expanded.contains(column)
        return HStack(spacing: 4) {
            Text(markdownAttributed(table.header[column], size: size))
                .font(.system(size: size, weight: .semibold))
                .ink(.primary)
                .lineLimit(1)
                .truncationMode(.tail)
                .frame(maxWidth: .infinity, alignment: frameAlignment(column))
            if width.abbreviated || isExpanded {
                Button {
                    withAnimation(Motion.stateChange) {
                        if isExpanded { expanded.remove(column) } else { expanded.insert(column) }
                    }
                } label: {
                    Image(systemName: isExpanded ? "arrow.right.and.line.vertical.and.arrow.left" : "arrow.left.and.right")
                        .font(.system(size: 10, weight: .semibold))
                        .ink(.tertiary)
                        .frame(width: 14, height: 14)
                        .contentShape(Rectangle())
                }
                .buttonStyle(GnatIconButtonStyle())
                .help(isExpanded ? "Abbreviate the column" : "Show the column at full width")
            }
        }
        .padding(.horizontal, Self.cellPadding)
        .padding(.vertical, 5)
        .frame(width: self.width(width, index: column), alignment: .leading)
    }

    private func cell(_ text: String, column: Int, width: TableColumnWidth) -> some View {
        Text(markdownAttributed(text, size: size))
            .font(.system(size: size))
            .ink(.primary)
            .lineLimit(1)
            .truncationMode(.tail)
            .textSelection(.enabled)
            .padding(.horizontal, Self.cellPadding)
            .padding(.vertical, 5)
            .frame(width: self.width(width, index: column), alignment: frameAlignment(column))
            .help(width.abbreviated ? text : "")
    }

    private func frameAlignment(_ column: Int) -> Alignment {
        switch table.alignments[column] {
        case .leading: return .leading
        case .center: return .center
        case .trailing: return .trailing
        }
    }
}
