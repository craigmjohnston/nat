import AppKit
import SwiftUI
import NatKit

/// The main pane's heading: a band the height of a navigator section's
/// header, on the same ground, with what describes the view under it — the
/// diff's commit switcher, the agent's model and effort — or nothing.
struct MainPaneHeader<Content: View>: View {
    @ViewBuilder var content: () -> Content

    var body: some View {
        HStack(spacing: 8) {
            content()
        }
        .padding(.horizontal, 12)
        .frame(height: GnatMetrics.sectionHeadHeight)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(DesignTokens.fill(.chrome))
        .environment(\.ground, .chrome)
        .overlay(alignment: .bottom) { DesignTokens.rule(.separator, on: .chrome).frame(height: 1) }
    }
}

extension MainPaneHeader where Content == EmptyView {
    init() { self.init(content: { EmptyView() }) }
}

/// The agent heading's words: the live agent's model and effort as its own
/// statusline reports them, or nothing with no reading.
struct AgentModelHeading: View {
    let agent: AgentStatus?

    var body: some View {
        if let label = buildAgentReadout(from: agent)?.label {
            Text(label).monoXS().ink(.secondary).lineLimit(1)
        }
    }
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
                Text("\(pr.title) #\(pr.number)")
                    .font(.system(size: 18, weight: .semibold))
                    .ink(.primary)
                    .textSelection(.enabled)

                NavHeading(text: "Description")
                if described.isEmpty {
                    Text("No description.").font(.system(size: 13.5)).ink(.secondary)
                } else {
                    MarkdownView(text: described, size: 13.5)
                }

                NavHeading(text: entries.isEmpty ? "Conversation" : "Conversation · \(entries.count)")
                if entries.isEmpty {
                    Text("No comments yet.").font(.system(size: 13.5)).ink(.secondary)
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
        .inelastic()
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
                }
            }
        }
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

    /// `initiallyExpanded` is the gallery's seam: a story seeds the columns
    /// it is a story about.
    init(table: MarkdownTable, size: CGFloat, initiallyExpanded: Set<Int> = []) {
        self.table = table
        self.size = size
        _expanded = State(initialValue: initiallyExpanded)
    }

    /// What each column would take with nothing cut: its widest cell, set in
    /// the face it is drawn in, plus the cell's own padding.
    private var naturalWidths: [Double] {
        let body = NSFont.systemFont(ofSize: size)
        let heading = NSFont.systemFont(ofSize: size, weight: .semibold)
        return table.header.indices.map { column in
            let cells = [(table.header[column], heading)] + table.rows.map { ($0[column], body) }
            let widest = cells.map { cell, font in
                (String(markdownAttributed(cell, size: size).characters) as NSString)
                    .size(withAttributes: [.font: font]).width
            }.max() ?? 0
            return Double(ceil(widest)) + Self.cellPadding * 2
        }
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
        .scrollBounceBehavior(.basedOnSize, axes: .horizontal)
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
                .buttonStyle(.plain)
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
