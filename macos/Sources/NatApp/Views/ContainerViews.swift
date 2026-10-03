import AppKit
import SwiftUI
import NatKit

// A source container selected: its navigator (the story's facts, then one
// section per section the plugin declared) and its main pane (the story and
// its comments, or a links section's list), over `ContainerStore`'s reading
// of `nat container-show`. What is open and what the pane shows are the
// shell's (`ContainerFocus`), as a slice's are.

/// The container's `container-show` reading, as the views draw it.
@MainActor
private func containerState(_ appModel: AppModel, _ containerID: String) -> ContainerLoadState {
    appModel.containerStore(projectID: appModel.activeProjectID ?? "").state(for: containerID)
}

/// A selected container's navigator: the first section — titled by the
/// story's own title — holding its facts and how far its tasks are, with
/// **New task** in its header; then each section the plugin declared, of a
/// kind this build draws.
struct ContainerNavigatorView: View {
    @Bindable var appModel: AppModel
    let containerID: String
    @Binding var focus: ContainerFocus
    let onNewTask: () -> Void

    private var source: SidebarSource? { appModel.source(ofProject: appModel.activeProjectID ?? "") }

    var body: some View {
        let state = containerState(appModel, containerID)
        if let show = state.show {
            let model = ContainerNavigatorModel(show: show)
            NavigatorColumn(anyOpen: !focus.open.isEmpty) {
                NavSectionView(
                    label: model.storyTitle, open: focus.open.contains(model.storyID),
                    selected: focus.main == .story,
                    onHead: { click(model.storyID, shows: .story) }, onFold: { fold(model.storyID) }
                ) {
                    Button(action: onNewTask) {
                        HeaderActionLabel(title: "New \(source?.taskNoun ?? "task")", systemImage: "plus")
                    }
                    .buttonStyle(GnatHeaderButtonStyle(primary: true))
                } content: {
                    ScrollView {
                        VStack(alignment: .leading, spacing: 8) {
                            if let message = state.errorMessage {
                                Text("Showing the last reading — \(message)").ink(.warning)
                            }
                            Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 3) {
                                SourceFactRows(facts: show.container.facts)
                                GridRow {
                                    Text("\(source?.taskNoun ?? "task")s").ink(.tertiary)
                                    Text(model.tasksFact).ink(model.tasks.isEmpty ? .tertiary : .primary)
                                }
                            }
                            .monoXS()
                        }
                        .font(.system(size: GnatMetrics.body))
                        .padding(12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .thinScrollers()
                }
                ForEach(model.sections) { section in
                    let shows = model.paneMode(for: section.id)
                    NavSectionView(
                        label: section.title, open: focus.open.contains(section.id), selected: focus.main == shows
                            && shows != .story,
                        meta: model.meta(for: section),
                        onHead: { click(section.id, shows: shows) }, onFold: { fold(section.id) }
                    ) {
                        sectionBody(section)
                    }
                }
            }
        } else {
            NavigatorColumn(anyOpen: true) {
                Group {
                    if let message = state.errorMessage {
                        NavNotice(text: "The \(source?.containerNoun ?? "container") could not be read — \(message)")
                    } else {
                        QuietLoadingView(label: "Reading the \(source?.containerNoun ?? "container")")
                            .frame(maxWidth: .infinity, minHeight: 80)
                    }
                }
                .frame(maxHeight: .infinity, alignment: .top)
                .surface(.window)
            }
        }
    }

    private func click(_ section: String, shows: ContainerPaneMode) {
        focus = focus.clickingHead(section, shows: shows)
    }

    private func fold(_ section: String) {
        focus = focus.togglingFold(section)
    }

    @ViewBuilder
    private func sectionBody(_ section: SourceSection) -> some View {
        ScrollView {
            switch section.kind {
            case .comments:
                VStack(spacing: 6) {
                    if section.comments.isEmpty {
                        NavProse { Text("No comments yet.").ink(.secondary) }
                    }
                    ForEach(Array(section.comments.enumerated()), id: \.offset) { _, comment in
                        ThreadEventCard(event: ThreadEvent(.agent, who: comment.by, meta: comment.when, body: comment.text))
                    }
                }
                .padding(6)
            case .links:
                SourceLinkList(links: section.links)
                    .padding(.vertical, 4)
            case .prose:
                NavProse {
                    MarkdownView(text: section.body ?? "", size: 13.5)
                }
            case .unknown:
                EmptyView()
            }
        }
        .thinScrollers()
    }
}

/// A links section's rows: a branch glyph for a pull request, an arrow out
/// for anything else; its label, its text, its state — each opening its URL.
struct SourceLinkList: View {
    let links: [SourceLink]
    var textSize: CGFloat = GnatMetrics.body

    var body: some View {
        VStack(spacing: 0) {
            if links.isEmpty {
                Text("No links.")
                    .font(.system(size: textSize))
                    .ink(.tertiary)
                    .padding(.horizontal, 12)
                    .frame(height: 28)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            ForEach(Array(links.enumerated()), id: \.offset) { _, link in
                HStack(spacing: 10) {
                    Image(systemName: link.state == nil ? SourceGlyph.externalLink : SourceGlyph.pullRequestLink)
                        .font(.system(size: 11))
                        .ink(.tertiary)
                        .frame(width: 14)
                    Text(link.label).monoXS().ink(.secondary).lineLimit(1).fixedSize()
                    Text(link.text)
                        .font(.system(size: textSize))
                        .ink(.primary)
                        .lineLimit(1)
                        .truncationMode(.tail)
                    Spacer(minLength: 0)
                    if let state = link.state {
                        Text(state).monoXS().ink(.tertiary)
                    }
                }
                .padding(.horizontal, 12)
                .frame(height: 28)
                .gnatRow()
                .contentShape(Rectangle())
                .onTapGesture {
                    if let url = link.url.flatMap(URL.init(string:)) { NSWorkspace.shared.open(url) }
                }
                .help(link.url ?? "")
            }
        }
    }
}

/// A selected container's main pane: its story — the prose, then under a
/// rule its comments thread with the composer at its foot — or a links
/// section's list.
struct ContainerPane: View {
    @Bindable var appModel: AppModel
    let containerID: String
    let mode: ContainerPaneMode

    @State private var commentText = ""
    @State private var isSending = false
    @State private var commentError: String?

    private static let textSize: CGFloat = 15

    var body: some View {
        let state = containerState(appModel, containerID)
        VStack(spacing: 0) {
            if let show = state.show {
                let model = ContainerNavigatorModel(show: show)
                switch mode {
                case .story:
                    story(model)
                case .section(let id):
                    ScrollView {
                        SourceLinkList(
                            links: model.sections.first { $0.id == id }?.links ?? [], textSize: Self.textSize)
                            .padding(.vertical, 12)
                            .frame(maxWidth: 820, alignment: .leading)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .thinScrollers()
                }
            } else if let message = state.errorMessage {
                MainPaneNote(text: "The \(noun) could not be read — \(message)")
            } else {
                QuietLoadingView(label: "Reading the \(noun)")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .surface(.window)
        .onChange(of: containerID) { _, _ in
            commentText = ""
            commentError = nil
        }
    }

    private var noun: String { appModel.source(ofProject: appModel.activeProjectID ?? "")?.containerNoun ?? "container" }

    private func story(_ model: ContainerNavigatorModel) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                if let body = model.prose?.body, !body.isEmpty {
                    MarkdownView(text: body, size: Self.textSize)
                } else {
                    Text("No description.").font(.system(size: Self.textSize)).ink(.secondary)
                }
                if let comments = model.comments {
                    DesignTokens.rule(.separator, on: .window).frame(height: 1).padding(.top, 14)
                    NavHeading(text: comments.comments.isEmpty
                               ? comments.title : "\(comments.title) \u{00B7} \(comments.comments.count)")
                    if comments.comments.isEmpty {
                        Text("No comments yet.").font(.system(size: Self.textSize)).ink(.secondary)
                    }
                    ForEach(Array(comments.comments.enumerated()), id: \.offset) { _, comment in
                        SourceCommentView(comment: comment, textSize: Self.textSize)
                    }
                    if let composer = comments.composer {
                        PRComposerView(
                            placeholder: "\(composer.label) on the \(noun)\u{2026}",
                            text: $commentText, isSending: isSending, error: commentError,
                            onSend: { Task { await send(composer) } })
                            .background {
                                // ⌘↩ sends, as the design's composer says.
                                Button("") { Task { await send(composer) } }
                                    .keyboardShortcut(.return, modifiers: .command)
                                    .hidden()
                            }
                    }
                }
            }
            .padding(.horizontal, 28)
            .padding(.vertical, 22)
            .frame(maxWidth: 820, alignment: .leading)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .thinScrollers()
    }

    private func send(_ composer: SourceAction) async {
        let text = commentText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, !isSending, let projectID = appModel.activeProjectID else { return }
        isSending = true
        commentError = nil
        commentError = await appModel.runSourceAction(
            projectID: projectID, action: composer, container: containerID, input: text)
        if commentError == nil { commentText = "" }
        isSending = false
    }
}

/// One comment of a container's thread, boxed as the pull request's are
/// (`PRConversationEntryView`): an avatar of the author's initials, their
/// name and when — the plugin's own words for it — over the markdown.
struct SourceCommentView: View {
    let comment: SourceComment
    var textSize: CGFloat = 15

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .center, spacing: 8) {
                Text(authorInitials(comment.by))
                    .font(.system(size: 8, weight: .semibold))
                    .ink(.accent)
                    .frame(width: PRConversationEntryView.avatarSize, height: PRConversationEntryView.avatarSize)
                    .wash(.avatar)
                    .clipShape(Circle())
                Text(comment.by)
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .ink(.primary)
                Spacer(minLength: 0)
                Text(comment.when)
                    .font(.system(size: Typo.caption))
                    .ink(.tertiary)
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 7)
            .frame(maxWidth: .infinity, alignment: .leading)
            .surface(.chrome)

            if !comment.text.isEmpty {
                MarkdownView(text: comment.text, size: textSize, ink: .primary)
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

/// What a container's main pane stands at the titlebar band's trailing
/// edge: Open in <source>, where the plugin gave the container a URL.
struct ContainerTitlebarTrailing: View {
    @Bindable var appModel: AppModel
    let containerID: String

    var body: some View {
        let source = appModel.source(ofProject: appModel.activeProjectID ?? "")
        let url = containerState(appModel, containerID).show?.container.externalURL
            ?? source?.container(withID: containerID)?.externalURL
        if let url = url.flatMap(URL.init(string:)) {
            Button {
                NSWorkspace.shared.open(url)
            } label: {
                HeaderActionLabel(title: "Open in \(source?.title ?? "source")", systemImage: SourceGlyph.externalLink)
            }
            .buttonStyle(GnatHeaderButtonStyle())
            .help(url.absoluteString)
            .padding(.trailing, -12)
        }
    }
}
