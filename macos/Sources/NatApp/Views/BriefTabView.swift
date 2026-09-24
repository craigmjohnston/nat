import SwiftUI
import NatKit
import NatFixtures

/// The brief card's own header: its label and the Edit button. Its own view
/// rather than a row inside the card, because it is chrome rather than
/// content — identical before the slice's detail lands and after it — so
/// `BriefSkeletonView` draws this very thing from the first frame instead of
/// blocks that would be replaced by it.
///
/// Its defaults are the skeleton's state: nothing to edit yet, which is what
/// `canEditBrief` says of a slice with no detail read.
struct BriefCardHeader: View {
    var canEdit: Bool = false
    var help: String = ""
    var onEdit: () -> Void = {}

    var body: some View {
        HStack {
            Text("Brief")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .ink(.secondary)

            Spacer()

            Button(action: onEdit) {
                Text("Edit…")
                    .font(.system(size: Typo.subhead, weight: .regular))
            }
            .buttonStyle(GhostButtonStyle())
            .disabled(!canEdit)
            .help(help)
        }
    }
}

struct BriefTabView: View {
    /// What the brief is drawn on, for the one colour it computes as a
    /// value — a status dot's fill, chosen by a switch before it is applied.
    @Environment(\.ground) private var ground
    @Bindable var appModel: AppModel
    let slice: Slice
    var onTabChange: (WorkflowTab) -> Void = { _ in }

    /// The slice's own cached detail, read through the project's shared
    /// `SliceDetailStore` rather than a `NatClient` of this view's own — the
    /// store is what renders a cache hit instantly on re-selection and keeps
    /// it fresh with a background read that never blanks what is already
    /// showing.
    private var detailState: SliceDetailLoadState {
        guard let projectID = appModel.projectStore?.projectID else { return .idle }
        return appModel.sliceDetailStore(projectID: projectID).state(for: slice.id)
    }

    // Brief editing UI state — one editing state behind both Edit buttons.
    @State private var isEditingBrief = false
    @State private var editedBriefText = ""
    @State private var isSavingBrief = false
    @State private var briefSaveError: String?

    // Launch Agent UI state — empty is "leave it to Claude Code", matching
    // the config's own convention for an unset value.
    @State private var showLaunchPopover = false
    @State private var selectedModel: String = ""
    @State private var selectedEffort: String = ""
    @State private var launchWarning: String?

    /// Launching is held by the app's `SliceActionTracker`, not by this view:
    /// it advances the pane to the Agent stage at once, unmounting this tab,
    /// and a failure returns to a fresh one that must still find the error —
    /// and the one-shot rule — where the action left them.
    private var isLaunching: Bool { appModel.sliceActions.isRunning(.launch, sliceID: slice.id) }
    private var launchError: String? { appModel.sliceActions.error(.launch, sliceID: slice.id) }

    /// The models and effort levels the popover offers — see
    /// `SettingsView`'s own use of the same cache.
    @State private var agentOptions = AgentOptions.fallback

    /// The sidebar's width, draggable at its divider and remembered across
    /// launches — the same `PaneResizeHandle` bargain the PR tab's sidebar
    /// makes, kept under its own key since the two rails size independently.
    @AppStorage("briefSidebarWidth") private var sidebarWidth = 216.0
    @State private var liveSidebarWidth: Double?

    var body: some View {
        VStack(spacing: 0) {
            // The slice's first read, drawn as the brief it is about to be —
            // card, prose and properties rail — rather than as a spinner in
            // a card with no rail beside it, which would narrow the reading
            // column the moment the brief landed. A re-read never reaches
            // this: `SliceDetailLoadState` keeps what it has, and the
            // inspector's own pinned busy mark is what says a read is
            // running over it.
            if detailState.detail == nil, detailState.isLoading {
                BriefSkeletonView()
            } else {
                loadedBody
            }
        }
        .surface(.window)
        .task {
            await loadDetail()
        }
        .onChange(of: slice.id) { _, _ in
            Task {
                await loadDetail()
            }
            resetLaunchState()
            isEditingBrief = false
            briefSaveError = nil
            isSavingBrief = false
        }
        .task {
            resetLaunchState()
        }
        .task {
            agentOptions = await AgentOptionsCache.shared.resolve()
        }
    }

    private var loadedBody: some View {
        VStack(spacing: 0) {
            // Scrollable content area
            HStack(spacing: 0) {
                ScrollView {
                    // The reading column: the brief itself, as prose.
                    VStack(alignment: .leading, spacing: 16) {
                        // The brief as a document card: real content gets a
                        // surface of its own, so the empty pane below reads
                        // as canvas rather than void. The Spacer that pushes
                        // this up stays outside — the card wraps only what
                        // it actually holds.
                        VStack(alignment: .leading, spacing: 16) {
                            // Brief section label — the same row
                            // `BriefSkeletonView` draws while the read is in
                            // flight, since it says the same thing either way.
                            BriefCardHeader(
                                canEdit: canEditBrief,
                                help: editBriefHelp,
                                onEdit: startEditingBrief
                            )

                            // Brief content — a cached detail (even a stale one
                            // still showing while a background read replaces it, or
                            // the last good one a failed read kept) always wins over
                            // "failed", so re-selecting a slice already read this
                            // session never blanks behind a skeleton. A first read
                            // never gets here at all: the pane draws
                            // `BriefSkeletonView` for that.
                            if isEditingBrief {
                                briefEditor
                            } else if let detail = detailState.detail {
                                // Render brief as markdown
                                VStack(alignment: .leading, spacing: 8) {
                                    if !detail.brief.isEmpty {
                                        Text(markdownAttributed(detail.brief, size: Typo.body))
                                            .font(.system(size: Typo.body, weight: .regular))
                                            .lineSpacing(2)
                                            .ink(.primary)
                                    } else {
                                        Text("No brief yet — what you write here becomes the agent's prompt.")
                                            .font(.system(size: Typo.body, weight: .regular))
                                            .ink(.tertiary)
                                    }
                                }
                            } else if let errorMsg = detailState.errorMessage {
                                VStack(spacing: 8) {
                                    Image(systemName: "exclamationmark.triangle")
                                        .font(.system(size: 24, weight: .regular))
                                        .ink(.danger)

                                    Text("Failed to load")
                                        .font(.system(size: Typo.body, weight: .regular))
                                        .ink(.primary)

                                    Text(errorMsg)
                                        .font(.system(size: Typo.subhead, weight: .regular))
                                        .ink(.secondary)
                                        .lineLimit(2)
                                }
                                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .center)
                            } else {
                                VStack(spacing: 8) {
                                    Image(systemName: "doc.text")
                                        .font(.system(size: 32, weight: .regular))
                                        .ink(.secondary)

                                    Text("No brief loaded")
                                        .font(.system(size: Typo.body, weight: .regular))
                                        .ink(.secondary)
                                }
                                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .center)
                            }
                        }
                        .padding(20)
                        .card(radius: 10, border: .hairline)

                        Spacer()
                    }
                    .padding(.horizontal, 22)
                    .padding(.vertical, 18)
                    .frame(maxWidth: 640)
                    .inelastic()
                }

                // The properties rail: status, milestone, branch and
                // dependencies read at a glance rather than threaded through
                // the prose — the same resizable, hairline-bordered sidebar
                // the PR tab draws beside its own main column. Only drawn
                // once there is a detail to read them off, the same gate the
                // reading column's own body uses.
                if let detail = detailState.detail {
                    briefSidebar(detail)
                }
            }
        }
    }

    // MARK: - Brief editing

    /// Only a Todo slice's brief is editable — `nat slice-edit` itself
    /// refuses one in progress or Done, since an agent already working from
    /// the brief it claimed with should not have it changed out from under
    /// it, and a Done slice has nothing left to brief.
    private var canEditBrief: Bool {
        slice.status == "Todo" && !isEditingBrief && detailState.detail != nil
    }

    private var editBriefHelp: String {
        guard slice.status != "Todo" else { return "" }
        return "Only a Todo slice's brief can be edited"
    }

    private func startEditingBrief() {
        editedBriefText = detailState.detail?.brief ?? ""
        briefSaveError = nil
        isEditingBrief = true
    }

    private func cancelEditingBrief() {
        isEditingBrief = false
        briefSaveError = nil
    }

    private var briefEditor: some View {
        VStack(alignment: .leading, spacing: 8) {
            TextEditor(text: $editedBriefText)
                .font(Typo.mono(size: Typo.body))
                .scrollContentBackground(.hidden)
                .frame(minHeight: 160, maxHeight: 320)
                .padding(6)
                .field(radius: 8)
                .disabled(isSavingBrief)

            if let briefSaveError {
                Text(briefSaveError)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.danger)
            }

            HStack(spacing: 8) {
                Spacer()

                Button("Cancel", action: cancelEditingBrief)
                    .buttonStyle(SecondaryButtonStyle())
                    .disabled(isSavingBrief)

                Button(action: { Task { await saveBrief() } }) {
                    AsyncActionLabel(isBusy: isSavingBrief) {
                        Text("Save")
                    }
                }
                .buttonStyle(PrimaryButtonStyle())
                .disabled(isSavingBrief)
            }
        }
    }

    private func saveBrief() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        isSavingBrief = true
        briefSaveError = nil
        do {
            _ = try await NatClient().sliceEdit(projectID: projectID, sliceRef: slice.id, description: editedBriefText)
            isEditingBrief = false
            // Reads the slice back through the shared store, so its cache
            // holds the edited brief rather than the one it replaced.
            await loadDetail()
        } catch let error as NatError {
            if case .commandFailed(let message) = error {
                briefSaveError = message
            } else {
                briefSaveError = error.localizedDescription
            }
        } catch {
            briefSaveError = error.localizedDescription
        }
        isSavingBrief = false
    }

    // MARK: - Helpers

    /// Whether a launch is on offer at all, before the one-shot rule — a
    /// detail to launch off, and a plan that says it can.
    private var launchIsAvailable: Bool {
        guard detailState.detail != nil else { return false }

        let hasLiveAgent = appModel.selectedSliceID.flatMap { sliceID in
            appModel.activityStore?.agents[sliceID] != nil
        } ?? false

        return LaunchPlan(for: slice, hasLiveAgent: hasLiveAgent).canLaunch
    }

    private func launchIsEnabled() -> Bool {
        appModel.sliceActions.isEnabled(.launch, sliceID: slice.id, available: launchIsAvailable)
    }

    private func resetLaunchState() {
        showLaunchPopover = false
        launchWarning = nil

        // Prefill from config
        if let agent = appModel.config?.sliceAgent {
            selectedModel = agent.model ?? ""
            selectedEffort = agent.effort ?? ""
        } else {
            selectedModel = ""
            selectedEffort = ""
        }
    }

    private func performLaunch() {
        guard let projectID = appModel.projectStore?.projectID else { return }
        let sliceRef = slice.id
        let appModel = appModel

        // Build model and effort (nil if left blank, which is
        // "leave it to Claude Code")
        let model = selectedModel.isEmpty ? nil : selectedModel
        let effort = selectedEffort.isEmpty ? nil : selectedEffort

        launchWarning = nil
        showLaunchPopover = false
        Task {
            // Advances to the Agent stage at once (a content swap, so
            // instant rather than animated), and back here if it fails.
            await appModel.sliceActions.run(.launch, sliceID: sliceRef, select: onTabChange) {
                let result = try await NatClient().sliceLaunch(
                    projectID: projectID,
                    sliceRef: sliceRef,
                    model: model,
                    effort: effort
                )
                launchWarning = result.warning

                // Refresh the project to pick up the new agent
                await appModel.refresh()
            }
        }
    }

    @ViewBuilder
    private func launchPopoverContent() -> some View {
        LaunchOptionsForm(model: $selectedModel, effort: $selectedEffort, agentOptions: agentOptions)
    }

    private func loadDetail() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        await appModel.sliceDetailStore(projectID: projectID).fetch(sliceRef: slice.id)
    }

    // MARK: - Properties sidebar

    /// The right-hand rail: the Launch Agent split button atop it — the
    /// standing inspector-top slot every pane with a rail opens with — then
    /// everything structured about the slice, read at a glance rather than
    /// threaded through the brief's prose, and a pinned foot for the launch's
    /// own busy mark and any error or warning it left behind. The same
    /// resizable, hairline-bordered shape `PRSidebarView` draws beside the PR
    /// tab's main column, right down to its own `@AppStorage` width.
    private func briefSidebar(_ detail: SliceDetail) -> some View {
        VStack(spacing: 0) {
            InspectorActionsBar {
                InspectorSplitButton(
                    title: "Launch Agent",
                    isBusy: isLaunching,
                    isEnabled: launchIsEnabled(),
                    onPrimary: performLaunch,
                    showMenu: $showLaunchPopover,
                    menu: launchPopoverContent
                )
                .onChange(of: launchIsAvailable, initial: true) { _, available in
                    appModel.sliceActions.observe(.launch, sliceID: slice.id, available: available)
                }
            }

            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    statusSection(detail)
                    if let milestoneName {
                        milestoneSection(milestoneName)
                    }
                    branchSection(detail)
                    dependsOnSection(detail)
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 18)
                .inelastic()
            }

            let isRefreshing = detailState.isLoading && detailState.detail != nil
            if isRefreshing || launchError != nil || launchWarning != nil {
                InspectorStatusFoot {
                    if isRefreshing {
                        HStack(spacing: 8) {
                            RefreshingMark(isRefreshing: true)
                            Spacer()
                        }
                    }

                    if let error = launchError {
                        InspectorNotice(text: error, systemImage: "exclamationmark.circle.fill", role: .danger)
                    } else if let warning = launchWarning {
                        InspectorNotice(text: warning, systemImage: "exclamationmark.triangle.fill", role: .warning)
                    }
                }
            }
        }
        .frame(width: liveSidebarWidth ?? sidebarWidth)
        .rule(.separator, edges: [.leading], width: 0.5)
        .overlay(alignment: .leading) {
            PaneResizeHandle(width: sidebarWidth, liveWidth: $liveSidebarWidth, onCommit: { sidebarWidth = $0 }, minWidth: 170, maxWidth: 400, edge: .leading)
                .offset(x: -4.5)
        }
    }

    private func statusSection(_ detail: SliceDetail) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("STATUS")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .ink(.tertiary)

            HStack(spacing: 6) {
                Circle()
                    .fill(DesignTokens.ink(statusDotColor(detail.status), on: ground))
                    .frame(width: 8, height: 8)
                Text(detail.status)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.primary)
            }
        }
    }

    // Omitted entirely when the milestone can't be named — a plan not yet
    // loaded says nothing false rather than a blank section.
    private func milestoneSection(_ name: String) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("MILESTONE")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .ink(.tertiary)

            Text(name)
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.primary)
        }
    }

    private func branchSection(_ detail: SliceDetail) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("BRANCH")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .ink(.tertiary)

            if let branch = detail.branch, !branch.isEmpty {
                Text(branch)
                    .font(Typo.mono(size: Typo.caption, weight: .regular))
                    .ink(.primary)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 3)
                    .field(radius: 4, border: .hairline)
                    .lineLimit(1)
                    .truncationMode(.middle)
            } else {
                Text("Assigned on launch")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.tertiary)
            }
        }
    }

    private func dependsOnSection(_ detail: SliceDetail) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("DEPENDS ON")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .ink(.tertiary)

            let entries = dependencyEntries(
                detail.dependsOn,
                plan: appModel.projectStore?.state.projectInfo?.slices ?? []
            )
            if entries.isEmpty {
                Text("None")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.tertiary)
            } else {
                VStack(alignment: .leading, spacing: 6) {
                    ForEach(Array(entries.enumerated()), id: \.offset) { _, entry in
                        HStack(spacing: 4) {
                            if entry.done {
                                Image(systemName: "checkmark")
                                    .font(.system(size: 8, weight: .bold))
                                    .ink(.success)
                            }
                            Text(entry.name)
                                .font(.system(size: Typo.caption, weight: .regular))
                                .ink(entry.done ? .tertiary : .primary)
                                .lineLimit(1)
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .surface(.control, radius: 10)
                    }

                    if detail.blocked {
                        Text("Blocked until these finish")
                            .font(.system(size: Typo.caption, weight: .regular))
                            .ink(.warning)
                    }
                }
            }
        }
    }

    /// The Status row's dot: green once done, orange while being worked, and
    /// otherwise the same quiet tertiary a Todo slice draws everywhere else.
    private func statusDotColor(_ status: String) -> InkRole {
        switch status {
        case "Done": return .success
        case "In progress": return .warning
        default: return .tertiary
        }
    }

    /// The slice's milestone name, read off the loaded plan rather than
    /// carried on the slice itself — `Slice.milestoneID` is a select option's
    /// name, and `Milestone.id` is that same name (see `domain.Milestone`),
    /// so this is a lookup rather than an ID resolution.
    private var milestoneName: String? {
        appModel.projectStore?.state.projectInfo?.milestones
            .first { $0.id == slice.milestoneID }?.name
    }
}

/// The launch popover's own form: a model picker and an effort picker, the
/// same shape, over `AgentOptions`. A view of its own rather than a method on
/// `BriefTabView` so it can be drawn without the popover it normally opens
/// in — a gallery story has no way to capture a real `NSPopover`'s own
/// window, but the content itself is exactly this.
struct LaunchOptionsForm: View {
    @Binding var model: String
    @Binding var effort: String
    let agentOptions: AgentOptions

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            // Model — the effort field's own picker shape, plus Custom for a
            // full model ID: there is no API to enumerate every alias, so
            // `agentOptions.models` is a documented set rather than
            // everything this field allows.
            HStack(alignment: .top, spacing: 8) {
                Text("Model")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .frame(width: 50, alignment: .leading)

                ModelPicker(value: $model, options: agentOptions.models) { text in
                    TextField("claude-…", text: text)
                        .textFieldStyle(.roundedBorder)
                        .font(Typo.mono(size: Typo.code))
                }
                .frame(maxWidth: .infinity)
            }

            Divider()
                .padding(.vertical, 4)

            // Effort selector — a fixed set the CLI rejects anything outside
            // of, read from the same source the model field's placeholder is.
            HStack(spacing: 8) {
                Text("Effort")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .frame(width: 50, alignment: .leading)

                Picker("Effort", selection: $effort) {
                    Text("Default").tag("")
                    ForEach(agentOptions.efforts, id: \.self) { level in
                        Text(level).tag(level)
                    }
                }
                .pickerStyle(.menu)
                .frame(maxWidth: .infinity, alignment: .trailing)
            }

            Divider()
                .padding(.vertical, 4)

            // Footnote
            Text("Runs detached in tmux — closing nat won't stop it.")
                .font(.system(size: Typo.caption, weight: .regular))
                .ink(.tertiary)
        }
        .frame(width: 280)
    }
}

#Preview("Handed back") {
    @Previewable @State var appModel = Fixtures.appModel()
    let slice = Fixtures.slices.first { $0.id == Fixtures.mergeBoxSliceID }!

    BriefTabView(appModel: appModel, slice: slice)
        .frame(height: 400)
        .task { await Fixtures.start(appModel) }
}

#Preview("Blocked") {
    @Previewable @State var appModel = Fixtures.appModel()
    let slice = Fixtures.slices.first { $0.id == Fixtures.cacheSliceID }!

    BriefTabView(appModel: appModel, slice: slice)
        .frame(height: 400)
        .task { await Fixtures.start(appModel) }
}
