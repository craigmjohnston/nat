import AppKit
import SwiftUI
import NatKit

// A source container selected: its navigator (the story's facts, then its
// links and any further prose, then the card's action row) and its main pane
// (the description and its comment threads, or a links section's list), over
// `ContainerStore`'s reading of `nat container show`. What is open and what
// the pane shows are the shell's (`ContainerFocus`), as a slice's are.

/// The container's `container show` reading, as the views draw it.
@MainActor
private func containerState(_ appModel: AppModel, _ containerID: String) -> ContainerLoadState {
    appModel.containerStore(projectID: appModel.activeProjectID ?? "").state(for: containerID)
}

/// A selected container's navigator: the first section — titled by the
/// story's own title — holding its facts and how far its tasks are, with
/// Open in <source> (where the plugin gave the container a URL) in its
/// header; then each further section the plugin declared that the navigator
/// draws (links, any other prose — the comments are the main pane's); then,
/// pinned at the foot as a slice's action bar is, the card's actions.
struct ContainerNavigatorView: View {
    @Bindable var appModel: AppModel
    let containerID: String
    @Binding var focus: ContainerFocus
    let onNewTask: () -> Void
    /// Whether the action row's split menu is open — its popover's, or a
    /// story's.
    var actionMenuOpen = false

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
                    if let url = (show.container.externalURL ?? source?.container(withID: containerID)?.externalURL)
                        .flatMap(URL.init(string:)) {
                        HeaderLinkButton(
                            title: "Open in \(source?.title ?? "source")", systemImage: SourceGlyph.externalLink,
                            help: url.absoluteString
                        ) {
                            NSWorkspace.shared.open(url)
                        }
                    }
                } content: {
                    ScrollView {
                        VStack(alignment: .leading, spacing: 8) {
                            if let message = state.errorMessage {
                                Text("The refresh failed, so this is the last reading: \(message)").ink(.warning)
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
            } footer: {
                CardActionBar(
                    appModel: appModel, projectID: appModel.activeProjectID ?? "", containerID: containerID,
                    actions: model.actions, taskNoun: source?.taskNoun ?? "task",
                    onNewTask: onNewTask, menuOpen: actionMenuOpen)
            }
        } else {
            NavigatorColumn(anyOpen: true) {
                Group {
                    if let message = state.errorMessage {
                        NavNotice(text: "The \(source?.containerNoun ?? "container") could not be read: \(message)")
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
            case .links:
                SourceLinkList(links: section.links)
                    .padding(.vertical, 4)
            case .prose:
                NavProse {
                    MarkdownView(text: section.body ?? "", size: Typo.scaled(13.5))
                }
            case .comments, .unknown:
                EmptyView()
            }
        }
        .thinScrollers()
    }
}

/// A card's action row, pinned at its navigator's foot as a slice's action
/// bar is: **Launch agent** the main action — greyed, saying why, on a card
/// that already has tasks — with **Workshop** and **New task** in a split off
/// it. A launch that fails says so over the row.
struct CardActionBar: View {
    @Bindable var appModel: AppModel
    let projectID: String
    let containerID: String
    let actions: CardActions
    let taskNoun: String
    let onNewTask: () -> Void
    @State private var menuOpen: Bool
    @State private var launchError: String?

    init(
        appModel: AppModel, projectID: String, containerID: String, actions: CardActions, taskNoun: String,
        onNewTask: @escaping () -> Void, menuOpen: Bool = false
    ) {
        self.appModel = appModel
        self.projectID = projectID
        self.containerID = containerID
        self.actions = actions
        self.taskNoun = taskNoun
        self.onNewTask = onNewTask
        _menuOpen = State(initialValue: menuOpen)
    }

    private var launching: Bool { appModel.cardLaunches.contains(containerID) }

    var body: some View {
        VStack(spacing: 0) {
            if let launchError {
                NavNotice(text: launchError)
            }
            NavigatorActionBar {
                HeaderSplitButton(
                    primary: true, mainDisabled: !actions.canLaunchAgent || launching,
                    mainHelp: actions.launchRefusal ?? "File a \(taskNoun) named after this card and launch an agent on it",
                    chevronLabel: "More actions", menuOpen: $menuOpen, action: launch
                ) {
                    HeaderActionLabel(title: "Launch agent", systemImage: "arrow.right", isBusy: launching)
                } menu: {
                    CardActionMenu(taskNoun: taskNoun, onWorkshop: {
                        menuOpen = false
                        workshop()
                    }, onNewTask: {
                        menuOpen = false
                        onNewTask()
                    })
                }
            }
        }
        .onChange(of: containerID) { _, _ in launchError = nil }
    }

    private func launch() {
        launchError = nil
        let appModel = appModel, projectID = projectID, containerID = containerID, actions = actions
        Task {
            launchError = await appModel.launchCardAgent(projectID: projectID, containerID: containerID, card: actions)
        }
    }

    private func workshop() {
        let appModel = appModel, projectID = projectID, actions = actions
        Task { await appModel.workshopCard(projectID: projectID, card: actions) }
    }
}

/// The card action row's split menu, and the card row's right-click menu's
/// last two items: Workshop and New task.
struct CardActionMenu: View {
    let taskNoun: String
    let onWorkshop: () -> Void
    let onNewTask: () -> Void

    var body: some View {
        HeaderSplitMenuList(items: [
            HeaderSplitMenuList.Item(title: "Workshop", systemImage: workshopSymbol, action: onWorkshop),
            HeaderSplitMenuList.Item(title: "New \(taskNoun)", systemImage: "plus", action: onNewTask),
        ])
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

/// A selected container's main pane: its story — the description under its
/// heading, then under a rule its comment threads, each reply under the
/// comment it answers, a hover Reply on every comment opening a composer in
/// its thread, and the composer for a comment of its own at the foot — or a
/// links section's list. Images the plugin saved a copy of draw from it.
struct ContainerPane: View {
    @Bindable var appModel: AppModel
    let containerID: String
    let mode: ContainerPaneMode
    /// The thread whose reply composer is open — a story's, at first.
    var replyingTo: String?

    @State private var commentText = ""
    @State private var isSending = false
    @State private var commentError: String?
    @State private var replyThread: String?
    @State private var replyText = ""
    @State private var isReplying = false
    @State private var replyError: String?

    private static var textSize: CGFloat { Typo.headline }

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
                MainPaneNote(text: "The \(noun) could not be read: \(message)")
            } else {
                QuietLoadingView(label: "Reading the \(noun)")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .surface(.window)
        .onAppear { replyThread = replyingTo }
        .onChange(of: containerID) { _, _ in
            commentText = ""
            commentError = nil
            closeReply()
        }
    }

    private var noun: String { appModel.source(ofProject: appModel.activeProjectID ?? "")?.containerNoun ?? "container" }

    private func story(_ model: ContainerNavigatorModel) -> some View {
        let images = model.detail.imageFiles
        return ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                NavHeading(text: "Description")
                if let body = model.prose?.body, !body.isEmpty {
                    MarkdownView(text: body, size: Self.textSize, images: images)
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
                    ForEach(model.threads) { thread in
                        SourceCommentThreadView(
                            thread: thread, textSize: Self.textSize, images: images,
                            reply: comments.composer.map { composer in reply(to: thread, composer) })
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

    /// A thread's reply as its view holds it: open while it is the one
    /// replied to, one at a time — opening another moves the draft's box.
    private func reply(to thread: CommentThread, _ composer: SourceAction) -> PRReply {
        PRReply(
            isOpen: replyThread == thread.id, text: $replyText, isSending: isReplying,
            error: replyThread == thread.id ? replyError : nil,
            onOpen: {
                if replyThread != thread.id {
                    replyThread = thread.id
                    replyError = nil
                }
            },
            onCancel: closeReply,
            onSend: { Task { await sendReply(composer, to: thread) } })
    }

    private func closeReply() {
        replyThread = nil
        replyText = ""
        replyError = nil
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

    private func sendReply(_ composer: SourceAction, to thread: CommentThread) async {
        let text = replyText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, !isReplying, let projectID = appModel.activeProjectID else { return }
        isReplying = true
        replyError = nil
        replyError = await appModel.runSourceAction(
            projectID: projectID, action: composer, container: containerID, replyTo: thread.replyTarget, input: text)
        if replyError == nil { closeReply() }
        isReplying = false
    }
}

/// One comment thread of a container, boxed as the pull request's entries
/// are (`PRConversationEntryView`): the comment, then each reply under it,
/// stepped in past the avatar, then — while it is the one replied to — the
/// reply composer. Every comment's byline ends in a Reply under the pointer;
/// any of them answers the thread, which is one level deep.
struct SourceCommentThreadView: View {
    let thread: CommentThread
    var textSize: CGFloat = Typo.headline
    var images: [String: String] = [:]
    var reply: PRReply?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            SourceCommentView(comment: thread.comment, textSize: textSize, images: images, reply: reply)
            ForEach(thread.replies, id: \.id) { comment in
                SourceCommentView(comment: comment, textSize: textSize, images: images, reply: reply)
                    .padding(.leading, SourceCommentView.replyIndent)
                    .overlay(alignment: .top) { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
            }
            if let reply, reply.isOpen {
                PRComposerView(
                    placeholder: "Reply to \(thread.comment.by)\u{2026}",
                    text: reply.text, isSending: reply.isSending, error: reply.error,
                    onSend: reply.onSend, onCancel: reply.onCancel)
                    .padding(.leading, SourceCommentView.replyIndent)
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

/// One comment of a container's thread: an avatar of the author's initials,
/// their name and when — the plugin's own words for it — and, under the
/// pointer, Reply; then the markdown.
struct SourceCommentView: View {
    let comment: SourceComment
    var textSize: CGFloat = Typo.headline
    var images: [String: String] = [:]
    var reply: PRReply?
    @Environment(\.hoverForced) private var hoverForced
    @State private var hovering = false

    /// How far a reply steps in under the comment it answers.
    static let replyIndent: CGFloat = 28

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
                if let reply {
                    PRHoverIconButton(
                        systemImage: "arrowshape.turn.up.left", help: "Reply to \(comment.by)",
                        shown: hovering || hoverForced, action: reply.onOpen)
                }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 7)
            .frame(maxWidth: .infinity, alignment: .leading)
            .surface(.chrome)

            if !comment.text.isEmpty {
                MarkdownView(text: comment.text, size: textSize, ink: .primary, images: images)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 8)
                    .overlay(alignment: .top) { DesignTokens.rule(.separator, on: .window).frame(height: 1) }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .onHover { hovering = $0 }
    }
}
