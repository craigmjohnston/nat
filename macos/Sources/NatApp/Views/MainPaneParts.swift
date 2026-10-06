import AppKit
import SwiftUI
import NatKit

/// The one titlebar band over the navigator and the main pane — neither has
/// a heading band of its own, and no rule divides the band where the two
/// columns meet. It holds two things: the selection's breadcrumb
/// (`TitlebarBreadcrumb`), from the navigator's leading inset and free to run
/// on past its width, and the main pane's tabs (`TitlebarTab` — a slice's
/// or session's `MainPaneTab`, or the workshop's `WorkshopTab`) filling from
/// the band's trailing edge leftwards, the first rightmost — with only the
/// `trailing` item, a handed-back slice's run button, to their right, a line
/// between it and the rightmost tab. Both live in the main pane's part of
/// the band alone (`TitlebarBandLayout`): a breadcrumb with no room left
/// ellipsizes, never closer than `GnatMetrics.breadcrumbGap` to what follows
/// it, and a main pane narrower than the
/// two cuts them at their leading edge rather than letting them cross the
/// split. The agent's readout is the status bar's; a slice's actions are its
/// navigator's action bar.
struct TitlebarBand<Identity: View>: View {
    /// The navigator's width: the band's main-pane part is what is left.
    let navigatorWidth: Double
    var tabs: [TitlebarTab] = []
    /// The picked tab's `id`.
    var selected: String?
    var onTab: (TitlebarTab) -> Void = { _ in }
    /// A tab's `id` to draw under the pointer — a story's, since a render
    /// has no pointer; nil, `.onHover` alone decides.
    var hoveredTab: String?
    /// The band's rightmost item, after the tabs — the slice's run button.
    var trailing: AnyView?
    @ViewBuilder var identity: () -> Identity

    var body: some View {
        GnatTitlebar(leading: 0, trailing: 0, rule: false) {
            TitlebarBandStack(navigatorWidth: navigatorWidth) {
                identity()
                    .padding(.leading, GnatMetrics.breadcrumbInset)
                    .padding(.trailing, GnatMetrics.breadcrumbGap)
                HStack(spacing: 0) {
                    // The first tab is the rightmost; it closes its trailing
                    // edge only where the run button stands beside it.
                    ForEach(TitlebarBandLayout.leftToRight(tabs), id: \.self) { tab in
                        MainPaneTabButton(
                            title: tab.label, selected: tab.id == selected,
                            closed: trailing != nil && tab == tabs.first
                        ) { onTab(tab) }
                            .transformEnvironment(\.hoverForced) { if tab.id == hoveredTab { $0 = true } }
                    }
                }
                .fixedSize(horizontal: true, vertical: false)
                // Exactly the room the band gives it, the run against its
                // trailing edge and anything past its leading edge cut.
                .frame(minWidth: 0, maxWidth: .infinity, alignment: .trailing)
                .clipped()
                (trailing ?? AnyView(EmptyView()))
                    .fixedSize(horizontal: true, vertical: false)
                    .frame(minWidth: 0, maxWidth: .infinity, maxHeight: .infinity, alignment: .trailing)
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

extension TitlebarBand where Identity == EmptyView {
    init(navigatorWidth: Double) {
        self.init(navigatorWidth: navigatorWidth, identity: { EmptyView() })
    }
}

/// The band's three parts laid out as `TitlebarBandLayout` places them: the
/// breadcrumb from the leading edge, offered the room up to the tabs; the
/// trailing item offered what of the main pane's part it takes, against the
/// trailing edge; the tabs what of the rest they take, just before it.
private struct TitlebarBandStack: Layout {
    let navigatorWidth: Double

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        CGSize(width: proposal.width ?? navigatorWidth, height: proposal.height ?? GnatMetrics.titlebarHeight)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        // No trailing item is no subview at all — an empty view lays out as
        // nothing — so the band has two parts or three.
        guard subviews.count >= 2 else { return }
        let trailingItem = subviews.count > 2 ? subviews[2] : nil
        let run = subviews[1].sizeThatFits(ProposedViewSize(width: nil, height: bounds.height)).width
        let trailing = trailingItem?.sizeThatFits(ProposedViewSize(width: nil, height: bounds.height)).width ?? 0
        let layout = TitlebarBandLayout(
            bandWidth: bounds.width, navigatorWidth: navigatorWidth, runWidth: run, trailingWidth: trailing)
        subviews[0].place(
            at: CGPoint(x: bounds.minX, y: bounds.midY), anchor: .leading,
            proposal: ProposedViewSize(width: layout.identityWidth, height: bounds.height))
        subviews[1].place(
            at: CGPoint(x: bounds.minX + layout.runX, y: bounds.minY), anchor: .topLeading,
            proposal: ProposedViewSize(width: layout.runShownWidth, height: bounds.height))
        trailingItem?.place(
            at: CGPoint(x: bounds.minX + layout.trailingX, y: bounds.minY), anchor: .topLeading,
            proposal: ProposedViewSize(width: layout.trailingShownWidth, height: bounds.height))
    }
}

/// Which crumb of the breadcrumb a tree picker is open on.
enum CrumbPickerOrigin { case project, milestone, title }

/// The words of the titlebar's breadcrumb (`TitlebarBreadcrumb`).
struct TitlebarCrumbs: Equatable {
    /// The project crumb, which opens the picker on the project.
    var project: String?
    /// The crumb between the project and the selection: a milestone, a
    /// source task's container, or — for a workshop or a session — the
    /// project's own name.
    var parent: String?
    var parentKind: ParentKind = .milestone
    /// The selection's own name; empty with nothing selected, and then
    /// there is no breadcrumb at all.
    var title: String

    enum ParentKind: Equatable {
        /// A milestone, which opens the picker on itself.
        case milestone
        /// A source task's container, which opens the picker on itself.
        case container
        /// The project's name, standing where a milestone would — a
        /// workshop's, a session's: no picker.
        case project
    }

    /// Whether a crumb before the last one names the project.
    var namesProject: Bool { project != nil || parentKind == .project }

    static let none = TitlebarCrumbs(title: "")
}

/// Where the selection sits, as the titlebar band reads it left to right: a
/// project crumb, a milestone or container crumb (or what stands for one),
/// each followed by a quiet slash, then the selection itself — the last
/// crumb, drawn as `TitlebarIdentityLabel` draws it, with the project's tag
/// dropped where a crumb before it names the project already.
///
/// Every crumb that opens the tree picker (`CrumbTreePicker`, `picker`)
/// opens it on itself; `openPicker` is which one is open. Moving between
/// selections slides the crumbs rather than snapping them: each part keeps
/// its place in the row, so a name that changes width pushes its neighbours
/// along while the words cross-fade, and a part that comes or goes fades.
///
/// As room runs out the selection's name is kept longest: it ellipsizes to
/// 80% of itself, then the project crumb turns into the project's tag, then
/// the milestone ellipsizes to half of itself, and past that the breadcrumb
/// gives way to the selection's Active row line alone — dot, tag, name
/// (`BreadcrumbFit`, from each crumb's width as measured here).
struct TitlebarBreadcrumb<Picker: View>: View {
    let crumbs: TitlebarCrumbs
    let identity: TitlebarIdentity?
    /// The project's colour: its puck, at the far left in the band's inset
    /// (`GnatMetrics.breadcrumbInset`) — never in the row, so `measurements`
    /// do not count it and it stays at every `BreadcrumbFit` stage.
    var projectColor: ProjectColor?
    @Binding var openPicker: CrumbPickerOrigin?
    @ViewBuilder var picker: (CrumbPickerOrigin) -> Picker

    @State private var widths: [CrumbMeasure: CGFloat] = [:]

    private static var spacing: CGFloat { 10 }

    /// The project's tag, for the crumb naming the project to turn into.
    private var tag: String { identity?.tag ?? "" }

    /// The crumb naming the project: the project crumb, or a workshop's or
    /// session's project name standing where a milestone would.
    private var projectName: String? {
        crumbs.project ?? (crumbs.parentKind == .project ? crumbs.parent : nil)
    }

    /// The milestone or container crumb — the one that shortens.
    private var shortenable: String? { crumbs.parentKind == .project ? nil : crumbs.parent }

    private var fit: BreadcrumbFit {
        func width(_ group: CrumbMeasure, _ text: CrumbMeasure) -> CrumbWidth {
            CrumbWidth(group: Double(widths[group] ?? 0), text: Double(widths[text] ?? 0))
        }
        return BreadcrumbFit(
            available: Double(widths[.available] ?? .infinity), spacing: Double(Self.spacing),
            project: projectName.map { _ in width(.projectGroup, .projectText) },
            projectTag: tag.isEmpty ? nil : width(.tagGroup, .tagText),
            parent: shortenable.map { _ in width(.parentGroup, .parentText) },
            title: width(.titleGroup, .titleText))
    }

    var body: some View {
        let fit = fit
        Group {
            if fit.stage == .minimal {
                if !crumbs.title.isEmpty {
                    // The Active row's line: dot, the project's tag, the name.
                    crumbButton(.title) { TitlebarIdentityLabel(identity: identity, title: crumbs.title) }
                }
            } else {
                row(fit)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { widths[.available] = $0 }
        .background(alignment: .leading) { measurements.hidden() }
        // Left of the project crumb's folder glyph, with the PROJECTS row's
        // gap, in the inset before the row; none with no breadcrumb.
        .overlay(alignment: .leading) {
            if let projectColor, !crumbs.title.isEmpty {
                ProjectPuck(color: projectColor, ground: .header)
                    .offset(x: -(ProjectPuck.gap + ProjectPuck.width))
                    .allowsHitTesting(false)
            }
        }
        .contentTransition(.interpolate)
        .font(.system(size: GnatMetrics.titlebarText))
        .lineLimit(1)
        .truncationMode(.tail)
        .animation(Motion.breadcrumb, value: crumbs)
    }

    private func row(_ fit: BreadcrumbFit) -> some View {
        HStack(spacing: Self.spacing) {
            if let project = crumbs.project {
                projectGroup(project, asTag: fit.projectAsTag, picks: true)
                    .transition(.opacity)
            }
            if let parent = crumbs.parent {
                Group {
                    switch crumbs.parentKind {
                    case .milestone, .container:
                        parentGroup(parent, picks: true)
                            .frame(maxWidth: fit.parentWidth.map { CGFloat($0) }, alignment: .leading)
                    case .project:
                        projectGroup(parent, asTag: fit.projectAsTag, picks: false)
                    }
                }
                .transition(.opacity)
            }
            if !crumbs.title.isEmpty {
                crumbButton(.title) { titleLabel }
                    .frame(maxWidth: fit.titleWidth.map { CGFloat($0) }, alignment: .leading)
                    .layoutPriority(-1)
            }
        }
    }

    private var titleLabel: TitlebarIdentityLabel {
        TitlebarIdentityLabel(identity: identity?.lastCrumb(afterProjectCrumb: crumbs.namesProject), title: crumbs.title)
    }

    /// The crumb naming the project, then its slash: the project's name, or
    /// its tag. `picks`: whether it opens the tree picker (the project
    /// crumb does; a workshop's or session's project name does not).
    @ViewBuilder
    private func projectGroup(_ name: String, asTag: Bool, picks: Bool) -> some View {
        HStack(spacing: Self.spacing) {
            if picks {
                crumbButton(.project) { projectLabel(name, asTag: asTag) }
            } else {
                projectLabel(name, asTag: asTag)
            }
            CrumbSlash()
        }
        .fixedSize()
    }

    @ViewBuilder
    private func projectLabel(_ name: String, asTag: Bool) -> some View {
        if asTag {
            CrumbTagLabel(tag: tag)
        } else {
            ProjectCrumbLabel(name: name)
        }
    }

    /// The milestone or container crumb, then its slash; the name alone
    /// ellipsizes.
    private func parentGroup(_ parent: String, picks: Bool) -> some View {
        HStack(spacing: Self.spacing) {
            if picks {
                crumbButton(.milestone) { parentLabel(parent) }
            } else {
                parentLabel(parent)
            }
            CrumbSlash().fixedSize()
        }
    }

    private func parentLabel(_ parent: String) -> some View {
        HStack(spacing: 7) {
            Group {
                if crumbs.parentKind == .container {
                    // A source task's container: the sidebar's card mark.
                    Image(systemName: SourceGlyph.container)
                        .font(.system(size: 10))
                        .ink(.tertiary)
                        .frame(width: 13, height: CrumbLine.height)
                } else {
                    // The sidebar's own milestone mark, open.
                    FolderGlyph(open: true, color: DesignTokens.ink(.tertiary, on: .header))
                        .frame(height: CrumbLine.height)
                }
            }
            .fixedSize()
            Text(parent).ink(.tertiary)
        }
    }

    /// Every crumb at its whole width, and its words alone, unseen — what
    /// `fit` weighs the room against.
    private var measurements: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let projectName {
                measured(.projectGroup) { projectGroup(projectName, asTag: false, picks: false) }
                measured(.projectText) { Text(projectName) }
            }
            if !tag.isEmpty {
                measured(.tagGroup) { projectGroup(tag, asTag: true, picks: false) }
                measured(.tagText) { CrumbTagLabel(tag: tag) }
            }
            if let shortenable {
                measured(.parentGroup) { parentGroup(shortenable, picks: false) }
                measured(.parentText) { Text(shortenable) }
            }
            measured(.titleGroup) { titleLabel }
            measured(.titleText) { Text(identity?.title ?? crumbs.title) }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }

    private func measured<Content: View>(_ key: CrumbMeasure, @ViewBuilder _ content: () -> Content) -> some View {
        content()
            .fixedSize()
            .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { widths[key] = $0 }
    }

    /// A crumb that opens the tree picker on itself.
    private func crumbButton<Label: View>(_ origin: CrumbPickerOrigin, @ViewBuilder label: () -> Label) -> some View {
        Button { openPicker = origin } label: {
            label()
                .padding(.horizontal, 5)
                .padding(.vertical, 2)
                .hoverWash(cornerRadius: 5)
                .padding(.horizontal, -5)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .popover(isPresented: Binding(
            get: { openPicker == origin }, set: { if !$0 { openPicker = nil } }
        ), arrowEdge: .top) {
            picker(origin)
        }
    }
}

/// What the breadcrumb measures to fit itself (`BreadcrumbFit`).
private enum CrumbMeasure: Hashable {
    case available
    case projectGroup, projectText, tagGroup, tagText, parentGroup, parentText, titleGroup, titleText
}

/// The project crumb as the project's tag, set as the Active row sets it.
private struct CrumbTagLabel: View {
    let tag: String

    var body: some View {
        Text(tag)
            .font(Typo.mono(size: Typo.scaled(10), weight: .medium))
            .tracking(1)
            .ink(.secondary)
            .frame(height: CrumbLine.height)
    }
}

/// The breadcrumb's one line: every crumb's glyph is framed to the crumb
/// text's line height, so the row centres them all on the text's middle
/// rather than each on its own bounds.
private enum CrumbLine {
    static let height: CGFloat = 16
}

/// A crumb naming the project: the sidebar's own project mark, open, then
/// the name — the glyph framed to the crumb line as the milestone's is.
private struct ProjectCrumbLabel: View {
    let name: String

    var body: some View {
        HStack(spacing: 7) {
            StackedFolderGlyph(
                open: true,
                color: DesignTokens.ink(.tertiary, on: .header),
                backColor: DesignTokens.ink(.tertiary, on: .header))
                .frame(height: CrumbLine.height)
            Text(name).ink(.tertiary)
        }
    }
}

/// The quiet slash after a crumb. A slash descends below the baseline, so
/// its glyph's middle sits a point under the text's; it is lifted that
/// point, without moving its frame, onto the line the rest share.
private struct CrumbSlash: View {
    var body: some View {
        Text("/")
            .ink(.quaternary)
            .frame(height: CrumbLine.height)
            .offset(y: -1)
    }
}

/// The selection as the titlebar band's last crumb names it — its Active
/// row's dot, project tag and title, or the bare title where it has none —
/// and the chevron that says it opens the tree picker. As room runs out the
/// title alone gives way, ending in an ellipsis with the chevron still
/// beside it.
struct TitlebarIdentityLabel: View {
    let identity: TitlebarIdentity?
    let title: String

    var body: some View {
        HStack(spacing: 5) {
            Group {
                if let identity, let icon = identity.icon {
                    SourceIdentityLabel(
                        icon: icon, tag: identity.tag, title: identity.title, size: GnatMetrics.titlebarText,
                        iconInk: .tertiary, titleInk: .tertiary)
                } else if let identity {
                    ActiveIdentityLabel(
                        tag: identity.tag, state: identity.state, live: identity.live, title: identity.title,
                        symbol: identity.symbol, size: GnatMetrics.titlebarText, titleInk: .tertiary)
                } else {
                    Text(title).ink(.tertiary)
                }
            }
            .layoutPriority(-1)
            Image(systemName: "chevron.down")
                .font(.system(size: 9, weight: .semibold))
                .ink(.tertiary)
                .fixedSize()
                .frame(height: CrumbLine.height)
        }
        .font(.system(size: GnatMetrics.titlebarText))
        .lineLimit(1)
        .truncationMode(.tail)
    }
}

/// The agent readout, at the status bar's trailing edge in the bar's own
/// sans and its one quiet ink: the selection's live agent's model and effort, then — past
/// a divider like the leading edge's — its context clause as its own
/// statusline reports it, in the warning tint once it runs high; or nothing.
/// The long form is its tooltip.
struct AgentModelHeading: View {
    let agent: AgentStatus?

    var body: some View {
        if let readout = buildAgentReadout(from: agent) {
            HStack(spacing: 10) {
                if readout.model != nil || readout.effort != nil {
                    HStack(spacing: 6) {
                        if let model = readout.model { Text(model).ink(.tertiary) }
                        if let effort = readout.effort { Text(effort).ink(.tertiary) }
                    }
                }
                if let context = readout.context {
                    if readout.model != nil || readout.effort != nil { StatusBarDivider() }
                    Text(context.text).ink(context.warning ? .hot : .tertiary)
                }
            }
            .font(.system(size: GnatMetrics.xs))
            .monospacedDigit()
            .lineLimit(1)
            .fixedSize()
            .help(readout.detail)
        }
    }
}

/// The PR section head's one action: Open in GitHub,
/// once the right pull request is read (`expectedNumber`, as
/// `PRConversationPane` checks it).
struct PROpenInGitHubButton: View {
    let store: PRStore
    let expectedNumber: Int?

    var body: some View {
        if let pr = store.loadState.pr, expectedNumber == nil || pr.number == expectedNumber {
            HeaderLinkButton(
                title: "Open in GitHub", systemImage: "arrow.up.right.square", help: "Open the pull request on GitHub"
            ) {
                if let url = URL(string: pr.url) { NSWorkspace.shared.open(url) }
            }
        }
    }
}

/// The size the PR view sets its prose in — the description and every
/// comment alike — and the insets of a conversation entry's box.
enum PRConversationMetrics {
    static var textSize: CGFloat { Typo.headline }
    /// Byline and body alike, either side.
    static let entryHorizontalPadding: CGFloat = 14
    static let bylineVerticalPadding: CGFloat = 10
    /// The body's, and an open reply composer's under it.
    static let bodyVerticalPadding: CGFloat = 12
    /// How tall the description editor grows before it scrolls itself.
    static let descriptionEditorMaxHeight: CGFloat = 320
}

/// A pull request's description and conversation, with the comment box at
/// the end — the main pane's half of the PR; its checks and verdict are the
/// navigator's. `expectedNumber` keeps a reading of some other pull request
/// (the store is shared across the project) from showing while the right
/// one is read.
///
/// Every entry takes a Reply (one composer open at a time, each entry's
/// draft kept until sent or cancelled) and the description an Edit, whatever
/// the pull request's state: GitHub takes comments and edits on a merged or
/// closed one too.
struct PRConversationPane: View {
    let store: PRStore
    let expectedNumber: Int?

    @State private var commentText = ""
    @State private var isSending = false
    @State private var commentError: String?

    /// The entry whose reply composer is open (`ConvoEntry.replyKey`).
    @State private var replyingTo: String?
    /// Each entry's reply draft, kept while another's composer is open.
    @State private var replyDrafts: [String: String] = [:]
    @State private var isSendingReply = false
    @State private var replyError: String?

    /// The description being edited, nil while it is drawn.
    @State private var descriptionDraft: String?
    @State private var isSavingDescription = false
    @State private var descriptionError: String?
    @State private var hoveringDescription = false
    @Environment(\.hoverForced) private var hoverForced

    /// - Parameters:
    ///   - reply: an entry's reply composer open with this draft — a story's,
    ///     since a render has no pointer to press Reply with.
    ///   - editingDescription: the description editor open with this text,
    ///     for the same reason.
    init(
        store: PRStore, expectedNumber: Int?,
        reply: (key: String, text: String)? = nil, editingDescription: String? = nil
    ) {
        self.store = store
        self.expectedNumber = expectedNumber
        _replyingTo = State(initialValue: reply?.key)
        _replyDrafts = State(initialValue: reply.map { [$0.key: $0.text] } ?? [:])
        _descriptionDraft = State(initialValue: editingDescription)
    }

    var body: some View {
        if let pr = store.loadState.pr, expectedNumber == nil || pr.number == expectedNumber {
            content(pr)
                .onChange(of: pr.number) { _, _ in
                    commentText = ""
                    commentError = nil
                    replyingTo = nil
                    replyDrafts = [:]
                    replyError = nil
                    descriptionDraft = nil
                    descriptionError = nil
                }
        } else if let message = store.loadState.errorMessage {
            MainPaneNote(text: "The pull request could not be read: \(message)")
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
                .font(.system(size: Typo.scaled(20), weight: .semibold))
                .textSelection(.enabled)
                description(described)

                NavHeading(text: entries.isEmpty ? "Conversation" : "Conversation · \(entries.count)")
                if entries.isEmpty {
                    Text("No comments yet.").font(.system(size: PRConversationMetrics.textSize)).ink(.secondary)
                }
                ForEach(Array(entries.enumerated()), id: \.offset) { _, entry in
                    PRConversationEntryView(entry: entry, reply: reply(to: entry))
                }
                PRComposerView(
                    placeholder: "Comment on the pull request\u{2026}",
                    text: $commentText, isSending: isSending, error: commentError,
                    onSend: { Task { await send() } })
            }
            .padding(20)
            .frame(maxWidth: 820, alignment: .leading)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .thinScrollers()
    }

    /// The Description heading with its hover-only Edit, over the rendered
    /// markdown — or, while it is edited, the editor with Save and Cancel.
    @ViewBuilder
    private func description(_ described: String) -> some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(spacing: 6) {
                NavHeading(text: "Description")
                editButton(described)
                Spacer(minLength: 0)
            }
            if let draft = descriptionDraft {
                PRComposerView(
                    placeholder: "Describe the pull request\u{2026}",
                    text: Binding(get: { draft }, set: { descriptionDraft = $0 }),
                    isSending: isSavingDescription, error: descriptionError,
                    onSend: { Task { await saveDescription() } },
                    onCancel: {
                        descriptionDraft = nil
                        descriptionError = nil
                    },
                    sendTitle: "Save",
                    editorMaxHeight: PRConversationMetrics.descriptionEditorMaxHeight)
            } else if described.isEmpty {
                Text("No description.").font(.system(size: PRConversationMetrics.textSize)).ink(.secondary)
            } else {
                Excerpt(text: described, maxWords: briefExcerptWords * 3) { shown in
                    MarkdownView(text: shown, size: PRConversationMetrics.textSize)
                }
            }
        }
        .contentShape(Rectangle())
        .onHover { hoveringDescription = $0 }
    }

    /// The Description heading's Edit, under the pointer.
    private func editButton(_ described: String) -> some View {
        PRHoverIconButton(
            systemImage: "pencil", help: "Edit the description",
            shown: descriptionDraft == nil && (hoveringDescription || hoverForced)
        ) {
            descriptionError = nil
            descriptionDraft = described
        }
    }

    /// An entry's reply: open while it is `replyingTo`, its draft its own.
    private func reply(to entry: ConvoEntry) -> PRReply {
        let key = entry.replyKey
        let isOpen = replyingTo == key
        return PRReply(
            isOpen: isOpen,
            text: Binding(get: { replyDrafts[key] ?? "" }, set: { replyDrafts[key] = $0 }),
            isSending: isOpen && isSendingReply,
            error: isOpen ? replyError : nil,
            onOpen: {
                guard !isSendingReply else { return }
                replyError = nil
                replyingTo = key
            },
            onCancel: {
                replyDrafts[key] = nil
                replyError = nil
                if replyingTo == key { replyingTo = nil }
            },
            onSend: { Task { await sendReply(to: entry) } })
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

    private func sendReply(to entry: ConvoEntry) async {
        let key = entry.replyKey
        let text = replyDrafts[key] ?? ""
        guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        isSendingReply = true
        replyError = nil
        do {
            try await store.comment(text: replyBody(to: entry, text: text))
            replyDrafts[key] = nil
            if replyingTo == key { replyingTo = nil }
        } catch {
            replyError = SliceActionTracker.message(for: error)
        }
        isSendingReply = false
    }

    private func saveDescription() async {
        guard let draft = descriptionDraft else { return }
        isSavingDescription = true
        descriptionError = nil
        do {
            try await store.editDescription(draft)
            descriptionDraft = nil
        } catch {
            descriptionError = SliceActionTracker.message(for: error)
        }
        isSavingDescription = false
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
