import SwiftUI
import NatKit

struct BriefTabView: View {
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

    // Launch Agent UI state
    @State private var showLaunchPopover = false
    @State private var selectedModel: String = "Default"
    @State private var selectedEffort: String = "Default"
    @State private var isLaunching = false
    @State private var launchError: String?
    @State private var launchWarning: String?

    /// The sidebar's width, draggable at its divider and remembered across
    /// launches — the same `PaneResizeHandle` bargain the PR tab's sidebar
    /// makes, kept under its own key since the two rails size independently.
    @AppStorage("briefSidebarWidth") private var sidebarWidth = 216.0

    var body: some View {
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
                            // Brief section label
                            HStack {
                                Text("Brief")
                                    .font(.system(size: Typo.subhead, weight: .semibold))
                                    .foregroundStyle(DesignTokens.labelSecondary)

                                Spacer()

                                Button(action: startEditingBrief) {
                                    Text("Edit…")
                                        .font(.system(size: Typo.subhead, weight: .regular))
                                }
                                .buttonStyle(GhostButtonStyle())
                                .disabled(!canEditBrief)
                                .help(editBriefHelp)
                            }

                            // Brief content — a cached detail (even a stale one
                            // still showing while a background read replaces it, or
                            // the last good one a failed read kept) always wins over
                            // "loading"/"failed", so re-selecting a slice already
                            // read this session never blanks behind a spinner.
                            if isEditingBrief {
                                briefEditor
                            } else if let detail = detailState.detail {
                                // Render brief as markdown
                                VStack(alignment: .leading, spacing: 8) {
                                    if !detail.brief.isEmpty {
                                        Text(markdownAttributed(detail.brief, size: Typo.body))
                                            .font(.system(size: Typo.body, weight: .regular))
                                            .lineSpacing(2)
                                            .foregroundStyle(DesignTokens.label)
                                    } else {
                                        Text("No brief yet — what you write here becomes the agent's prompt.")
                                            .font(.system(size: Typo.body, weight: .regular))
                                            .foregroundStyle(DesignTokens.labelTertiary)
                                    }
                                }
                            } else if detailState.isLoading {
                                QuietLoadingView(label: "Loading the brief…")
                                    .frame(minHeight: 100)
                            } else if let errorMsg = detailState.errorMessage {
                                VStack(spacing: 8) {
                                    Image(systemName: "exclamationmark.triangle")
                                        .font(.system(size: 24, weight: .regular))
                                        .foregroundStyle(DesignTokens.systemRed)

                                    Text("Failed to load")
                                        .font(.system(size: Typo.body, weight: .regular))
                                        .foregroundStyle(DesignTokens.label)

                                    Text(errorMsg)
                                        .font(.system(size: Typo.subhead, weight: .regular))
                                        .foregroundStyle(DesignTokens.labelSecondary)
                                        .lineLimit(2)
                                }
                                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .center)
                            } else {
                                VStack(spacing: 8) {
                                    Image(systemName: "doc.text")
                                        .font(.system(size: 32, weight: .regular))
                                        .foregroundStyle(DesignTokens.labelSecondary)

                                    Text("No brief loaded")
                                        .font(.system(size: Typo.body, weight: .regular))
                                        .foregroundStyle(DesignTokens.labelSecondary)
                                }
                                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .center)
                            }
                        }
                        .padding(20)
                        .background(
                            RoundedRectangle(cornerRadius: 10)
                                .fill(DesignTokens.controlBg)
                        )
                        .overlay(
                            RoundedRectangle(cornerRadius: 10)
                                .stroke(DesignTokens.hairline, lineWidth: 1)
                        )

                        Spacer()
                    }
                    .padding(.horizontal, 22)
                    .padding(.vertical, 18)
                    .frame(maxWidth: 640)
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

            // Footer bar: the Launch Agent split control alone now — the
            // card's own "Edit…" is the one edit affordance, so the footer
            // isn't offering a second. A top hairline (rather than a
            // Divider) plus a faint fill mark it as its own action-bar
            // surface, distinct from the content above it.
            VStack(spacing: 0) {
                HStack(spacing: 8) {
                    Spacer()

                    // Split Launch Agent button — the one gradient action on
                    // this screen, per the button grammar. Dimmed as a whole
                    // rather than through each button's own disabled state,
                    // since a split control half-dimmed would read as only
                    // one half of it being unavailable.
                    ZStack {
                        HStack(spacing: 0) {
                            Button(action: performLaunch) {
                                // The label keeps its footprint while a
                                // launch is in flight — drawn invisible with
                                // the spinner overlaid — so the button never
                                // collapses to spinner width and the split
                                // control doesn't jump (the same reserve-and-
                                // overlay trick the project tabs use for
                                // their bolding labels).
                                Text("Launch Agent")
                                    .font(.system(size: Typo.subhead, weight: .semibold))
                                    .foregroundStyle(DesignTokens.accentText)
                                    .padding(.horizontal, 10)
                                    .opacity(isLaunching ? 0 : 1)
                                    .overlay {
                                        if isLaunching {
                                            ProgressView()
                                                .scaleEffect(0.5, anchor: .center)
                                        }
                                    }
                            }
                            .frame(height: 22)
                            .buttonStyle(.plain)
                            .disabled(!launchIsEnabled() || isLaunching)

                            Divider()
                                .frame(maxHeight: 22)
                                .opacity(0.25)

                            Button(action: { showLaunchPopover.toggle() }) {
                                Image(systemName: "chevron.down")
                                    .font(.system(size: 12, weight: .medium))
                                    .foregroundStyle(DesignTokens.accentText)
                                    .frame(width: 20, height: 22)
                            }
                            .buttonStyle(.plain)
                            .disabled(!launchIsEnabled())
                        }
                        // Flat accent rather than the brand gradient: the
                        // icon and the progress bar already carry the
                        // gradient, and a button shouting it too was one
                        // gradient too many.
                        .background(DesignTokens.accent)
                        .clipShape(RoundedRectangle(cornerRadius: 6))
                    }
                    .opacity(launchIsEnabled() ? 1 : 0.55)
                    .popover(isPresented: $showLaunchPopover, arrowEdge: .bottom) {
                        launchPopoverContent()
                            .padding(10)
                    }
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 8)
                .overlay(alignment: .top) {
                    DesignTokens.hairline
                        .frame(height: 1)
                }
                .background(DesignTokens.controlBg.opacity(0.5))

                // Error or warning message
                if let error = launchError {
                    HStack(spacing: 8) {
                        Image(systemName: "exclamationmark.circle.fill")
                            .foregroundStyle(DesignTokens.systemRed)
                            .font(.system(size: 12, weight: .medium))
                        Text(error)
                            .font(.system(size: Typo.subhead, weight: .regular))
                            .foregroundStyle(DesignTokens.systemRed)
                            .lineLimit(2)
                        Spacer()
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 6)
                    .background(Color(nsColor: .controlBackgroundColor).opacity(0.5))
                } else if let warning = launchWarning {
                    HStack(spacing: 8) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .foregroundStyle(DesignTokens.systemYellow)
                            .font(.system(size: 12, weight: .medium))
                        Text(warning)
                            .font(.system(size: Typo.subhead, weight: .regular))
                            .foregroundStyle(DesignTokens.systemYellow)
                            .lineLimit(2)
                        Spacer()
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 6)
                    .background(Color(nsColor: .controlBackgroundColor).opacity(0.5))
                }
            }
        }
        .background(DesignTokens.windowBg)
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
                .font(.system(size: Typo.body, weight: .regular))
                .scrollContentBackground(.hidden)
                .frame(minHeight: 160, maxHeight: 320)
                .padding(6)
                .background(DesignTokens.fieldBg)
                .clipShape(RoundedRectangle(cornerRadius: 8))
                .overlay(
                    RoundedRectangle(cornerRadius: 8)
                        .stroke(DesignTokens.controlBorder, lineWidth: 0.5)
                )
                .disabled(isSavingBrief)

            if let briefSaveError {
                Text(briefSaveError)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.systemRed)
            }

            HStack(spacing: 8) {
                Spacer()

                Button("Cancel", action: cancelEditingBrief)
                    .buttonStyle(.bordered)
                    .disabled(isSavingBrief)

                Button(action: { Task { await saveBrief() } }) {
                    if isSavingBrief {
                        ProgressView()
                            .scaleEffect(0.7, anchor: .center)
                    } else {
                        Text("Save")
                    }
                }
                .buttonStyle(.borderedProminent)
                .tint(DesignTokens.accent)
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

    private func launchIsEnabled() -> Bool {
        if isLaunching { return false }
        guard detailState.detail != nil else { return false }

        let hasLiveAgent = appModel.selectedSliceID.flatMap { sliceID in
            appModel.activityStore?.agents[sliceID] != nil
        } ?? false

        let plan = LaunchPlan(for: slice, hasLiveAgent: hasLiveAgent)
        return plan.canLaunch
    }

    private func resetLaunchState() {
        showLaunchPopover = false
        launchError = nil
        launchWarning = nil
        isLaunching = false

        // Prefill from config
        if let agent = appModel.config?.sliceAgent {
            selectedModel = agent.model ?? "Default"
            selectedEffort = agent.effort ?? "Default"
        } else {
            selectedModel = "Default"
            selectedEffort = "Default"
        }
    }

    private func performLaunch() {
        Task {
            isLaunching = true
            launchError = nil
            launchWarning = nil

            do {
                guard let projectID = appModel.projectStore?.projectID else {
                    launchError = "No project loaded"
                    isLaunching = false
                    return
                }

                // Build model and effort (nil if "Default")
                let model = selectedModel == "Default" ? nil : selectedModel
                let effort = selectedEffort == "Default" ? nil : selectedEffort

                let result = try await NatClient().sliceLaunch(
                    projectID: projectID,
                    sliceRef: slice.id,
                    model: model,
                    effort: effort
                )

                // Store warning if present
                if let warning = result.warning {
                    launchWarning = warning
                }

                // Refresh the project to pick up the new agent
                await appModel.refresh()

                // Switch to Agent tab — a content swap, not a state change,
                // so it happens instantly rather than animating.
                onTabChange(.agent)

                // Close the popover
                showLaunchPopover = false
            } catch let error as NatError {
                if case .commandFailed(let message) = error {
                    launchError = message
                } else {
                    launchError = error.localizedDescription
                }
            } catch {
                launchError = error.localizedDescription
            }

            isLaunching = false
        }
    }

    @ViewBuilder
    private func launchPopoverContent() -> some View {
        VStack(alignment: .leading, spacing: 10) {
            // Model selector
            HStack(spacing: 8) {
                Text("Model")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .frame(width: 50, alignment: .leading)

                Picker("Model", selection: $selectedModel) {
                    Text("Default").tag("Default")
                    Text("sonnet").tag("sonnet")
                    Text("opus").tag("opus")
                    Text("haiku").tag("haiku")
                }
                .pickerStyle(.menu)
                .frame(maxWidth: .infinity, alignment: .trailing)
            }

            Divider()
                .padding(.vertical, 4)

            // Effort selector
            HStack(spacing: 8) {
                Text("Effort")
                    .font(.system(size: Typo.subhead, weight: .semibold))
                    .frame(width: 50, alignment: .leading)

                Picker("Effort", selection: $selectedEffort) {
                    Text("Default").tag("Default")
                    Text("low").tag("low")
                    Text("med").tag("med")
                    Text("high").tag("high")
                }
                .pickerStyle(.segmented)
                .frame(maxWidth: .infinity)
            }

            Divider()
                .padding(.vertical, 4)

            // Footnote
            Text("Runs detached in tmux — closing nat won't stop it.")
                .font(.system(size: Typo.caption, weight: .regular))
                .foregroundStyle(DesignTokens.labelTertiary)
        }
        .frame(width: 280)
    }

    private func loadDetail() async {
        guard let projectID = appModel.projectStore?.projectID else { return }
        await appModel.sliceDetailStore(projectID: projectID).fetch(sliceRef: slice.id)
    }

    // MARK: - Properties sidebar

    /// The right-hand rail: everything structured about the slice, read at a
    /// glance rather than threaded through the brief's prose — the same
    /// resizable, hairline-bordered shape `PRSidebarView` draws beside the PR
    /// tab's main column, right down to its own `@AppStorage` width.
    private func briefSidebar(_ detail: SliceDetail) -> some View {
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
        }
        .frame(width: sidebarWidth)
        .rectBorder(width: 0.5, edges: [.leading], color: DesignTokens.separator)
        .overlay(alignment: .leading) {
            PaneResizeHandle(width: $sidebarWidth, minWidth: 170, maxWidth: 400, edge: .leading)
                .offset(x: -4.5)
        }
    }

    private func statusSection(_ detail: SliceDetail) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("STATUS")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(DesignTokens.labelTertiary)

            HStack(spacing: 6) {
                Circle()
                    .fill(statusDotColor(detail.status))
                    .frame(width: 8, height: 8)
                Text(detail.status)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.label)
            }
        }
    }

    // Omitted entirely when the milestone can't be named — a plan not yet
    // loaded says nothing false rather than a blank section.
    private func milestoneSection(_ name: String) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("MILESTONE")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(DesignTokens.labelTertiary)

            Text(name)
                .font(.system(size: Typo.subhead, weight: .regular))
                .foregroundStyle(DesignTokens.label)
        }
    }

    private func branchSection(_ detail: SliceDetail) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("BRANCH")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(DesignTokens.labelTertiary)

            if let branch = detail.branch, !branch.isEmpty {
                Text(branch)
                    .font(.system(size: Typo.caption, weight: .regular, design: .monospaced))
                    .foregroundStyle(DesignTokens.label)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 3)
                    .background(DesignTokens.fieldBg)
                    .clipShape(RoundedRectangle(cornerRadius: 4))
                    .overlay(
                        RoundedRectangle(cornerRadius: 4)
                            .stroke(DesignTokens.hairline, lineWidth: 1)
                    )
                    .lineLimit(1)
                    .truncationMode(.middle)
            } else {
                Text("Assigned on launch")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.labelTertiary)
            }
        }
    }

    private func dependsOnSection(_ detail: SliceDetail) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("DEPENDS ON")
                .font(.system(size: Typo.subhead, weight: .semibold))
                .foregroundStyle(DesignTokens.labelTertiary)

            let entries = dependencyEntries(
                detail.dependsOn,
                plan: appModel.projectStore?.state.projectInfo?.slices ?? []
            )
            if entries.isEmpty {
                Text("None")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .foregroundStyle(DesignTokens.labelTertiary)
            } else {
                VStack(alignment: .leading, spacing: 6) {
                    ForEach(Array(entries.enumerated()), id: \.offset) { _, entry in
                        HStack(spacing: 4) {
                            if entry.done {
                                Image(systemName: "checkmark")
                                    .font(.system(size: 8, weight: .bold))
                                    .foregroundStyle(DesignTokens.systemGreen)
                            }
                            Text(entry.name)
                                .font(.system(size: Typo.caption, weight: .regular))
                                .foregroundStyle(entry.done ? DesignTokens.labelTertiary : DesignTokens.label)
                                .lineLimit(1)
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(DesignTokens.controlFace)
                        .clipShape(RoundedRectangle(cornerRadius: 10))
                    }

                    if detail.blocked {
                        Text("Blocked until these finish")
                            .font(.system(size: Typo.caption, weight: .regular))
                            .foregroundStyle(DesignTokens.systemYellow)
                    }
                }
            }
        }
    }

    /// The Status row's dot: green once done, orange while being worked, and
    /// otherwise the same quiet tertiary a Todo slice draws everywhere else.
    private func statusDotColor(_ status: String) -> Color {
        switch status {
        case "Done": return DesignTokens.systemGreen
        case "In progress": return DesignTokens.systemOrange
        default: return DesignTokens.labelTertiary
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

#Preview {
    let appModel = AppModel()
    let slice = Slice(
        id: "test-id",
        name: "Test Slice",
        status: "In progress",
        milestoneID: "m1",
        assignee: "Craig",
        pr: "",
        url: "https://example.com",
        branch: "feature/test",
        repo: "/path/to/repo",
        dependsOn: nil,
        blocked: false,
        handedBack: false
    )

    BriefTabView(appModel: appModel, slice: slice)
        .frame(height: 400)
}
