import AppKit
import SwiftUI
import NatKit

/// The main pane's segment of the window titlebar — the pane has no heading
/// band of its own. Its tabs (`MainPaneTab`) start flush at the pane's
/// leading edge; at its trailing edge, the live agent's readout and the
/// view's own actions, or nothing.
struct MainPaneTitlebar<Trailing: View>: View {
    var tabs: [MainPaneTab] = []
    var selected: MainPaneMode?
    var onTab: (MainPaneTab) -> Void = { _ in }
    @ViewBuilder var trailing: () -> Trailing

    var body: some View {
        GnatTitlebar(leading: 0, trailing: 0, rule: false) {
            HStack(spacing: 0) {
                ForEach(tabs, id: \.self) { tab in
                    MainPaneTabButton(title: tab.label, selected: tab.mode == selected) { onTab(tab) }
                }
                HStack(spacing: 8) {
                    Spacer(minLength: 8)
                    trailing()
                }
                .padding(.trailing, 12)
                .frame(maxHeight: .infinity)
                // The band's line, where no tab stands over it.
                .overlay(alignment: .bottom) {
                    DesignTokens.rule(.separator, on: .header).frame(height: 1).allowsHitTesting(false)
                }
            }
        }
    }
}

extension MainPaneTitlebar where Trailing == EmptyView {
    init(tabs: [MainPaneTab] = [], selected: MainPaneMode? = nil, onTab: @escaping (MainPaneTab) -> Void = { _ in }) {
        self.init(tabs: tabs, selected: selected, onTab: onTab, trailing: { EmptyView() })
    }
}

/// The agent readout, at the main pane's titlebar's trailing edge: the live
/// agent's model / effort, then its context use as its own statusline
/// reports them — the context in the warning tint once it runs high — or
/// nothing.
struct AgentModelHeading: View {
    let agent: AgentStatus?

    var body: some View {
        if let readout = buildAgentReadout(from: agent) {
            HStack(spacing: 4) {
                if let label = readout.label {
                    Text(label).ink(.secondary)
                }
                if let context = readout.context {
                    if readout.label != nil { Text("·").ink(.secondary) }
                    Text(context.text).ink(context.warning ? .hot : .secondary)
                }
            }
            .monoXS()
            .monospacedDigit()
            .lineLimit(1)
        }
    }
}

/// The PR view's action, in the main pane's titlebar: Open in GitHub, once
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
            // Flush with the pane's edge, as a navigator header's actions are.
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
