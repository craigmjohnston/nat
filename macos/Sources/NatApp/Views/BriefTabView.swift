import SwiftUI
import NatKit

struct BriefTabView: View {
    @Bindable var appModel: AppModel
    let slice: Slice
    var onTabChange: (WorkflowTab) -> Void = { _ in }

    @State private var sliceDetail: SliceDetail?
    @State private var isLoading = false
    @State private var error: String?

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

    private func briefAttributedString(_ brief: String) -> AttributedString {
        do {
            return try AttributedString(markdown: brief, options: .init(interpretedSyntax: .full))
        } catch {
            // Fallback to plain text if markdown parsing fails
            return AttributedString(brief)
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            // Scrollable content area
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    // Brief section label
                    HStack {
                        Text("BRIEF — BECOMES THE AGENT'S PROMPT")
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundStyle(DesignTokens.labelTertiary)

                        Spacer()

                        Button(action: startEditingBrief) {
                            Text("Edit…")
                                .font(.system(size: 11, weight: .regular))
                        }
                        .buttonStyle(.borderless)
                        .disabled(!canEditBrief)
                        .help(editBriefHelp)
                    }

                    // Brief content
                    if isEditingBrief {
                        briefEditor
                    } else if isLoading {
                        VStack(spacing: 8) {
                            ProgressView()
                                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .center)
                        }
                        .frame(minHeight: 100)
                    } else if let detail = sliceDetail {
                        // Render brief as markdown
                        VStack(alignment: .leading, spacing: 8) {
                            if !detail.brief.isEmpty {
                                Text(briefAttributedString(detail.brief))
                                    .font(.system(size: 13, weight: .regular))
                                    .lineSpacing(2)
                                    .foregroundStyle(DesignTokens.label)
                                    .textSelection(.enabled)
                            } else {
                                Text("No brief yet")
                                    .font(.system(size: 13, weight: .regular))
                                    .foregroundStyle(DesignTokens.labelSecondary)
                            }
                        }

                        // Info line: dependencies
                        VStack(alignment: .leading, spacing: 4) {
                            Divider()
                                .padding(.vertical, 6)

                            Text(dependencyText(detail))
                                .font(.system(size: 12, weight: .regular))
                                .foregroundStyle(DesignTokens.labelTertiary)
                        }

                        // Info line: branch
                        if let branch = detail.branch, !branch.isEmpty {
                            HStack(spacing: 4) {
                                Text("branch")
                                    .font(.system(size: 12, weight: .regular))
                                Text(branch)
                                    .font(.system(size: 12, weight: .regular, design: .monospaced))
                                    .foregroundStyle(DesignTokens.label)
                            }
                            .foregroundStyle(DesignTokens.labelTertiary)
                        } else {
                            Text("branch assigned on launch")
                                .font(.system(size: 12, weight: .regular))
                                .foregroundStyle(DesignTokens.labelTertiary)
                        }
                    } else if let errorMsg = error {
                        VStack(spacing: 8) {
                            Image(systemName: "exclamationmark.triangle")
                                .font(.system(size: 24, weight: .regular))
                                .foregroundStyle(DesignTokens.systemRed)

                            Text("Failed to load")
                                .font(.system(size: 13, weight: .regular))
                                .foregroundStyle(DesignTokens.label)

                            Text(errorMsg)
                                .font(.system(size: 11, weight: .regular))
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
                                .font(.system(size: 13, weight: .regular))
                                .foregroundStyle(DesignTokens.labelSecondary)
                        }
                        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .center)
                    }

                    Spacer()
                }
                .padding(.horizontal, 22)
                .padding(.vertical, 18)
                .frame(maxWidth: 640)
            }

            // Footer bar with buttons
            Divider()
                .frame(height: 0.5)

            VStack(spacing: 0) {
                HStack(spacing: 8) {
                    Button(action: startEditingBrief) {
                        Text("Edit Brief…")
                            .font(.system(size: 11, weight: .regular))
                    }
                    .buttonStyle(.borderless)
                    .disabled(!canEditBrief)
                    .help(editBriefHelp)

                    Spacer()

                    // Split Launch Agent button
                    ZStack {
                        HStack(spacing: 0) {
                            Button(action: performLaunch) {
                                if isLaunching {
                                    ProgressView()
                                        .scaleEffect(0.7, anchor: .center)
                                        .frame(width: 22, height: 22)
                                } else {
                                    Text("Launch Agent")
                                        .font(.system(size: 12, weight: .semibold))
                                        .foregroundStyle(DesignTokens.accentText)
                                        .padding(.horizontal, 10)
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
                                    .font(.system(size: 9, weight: .bold))
                                    .foregroundStyle(DesignTokens.accentText)
                                    .frame(width: 20, height: 22)
                            }
                            .buttonStyle(.plain)
                            .disabled(!launchIsEnabled())
                        }
                        .background(DesignTokens.accent)
                        .cornerRadius(4)
                    }
                    .popover(isPresented: $showLaunchPopover, arrowEdge: .bottom) {
                        launchPopoverContent()
                            .padding(10)
                    }
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 8)
                .background(DesignTokens.controlBg)

                // Error or warning message
                if let error = launchError {
                    HStack(spacing: 8) {
                        Image(systemName: "exclamationmark.circle.fill")
                            .foregroundStyle(DesignTokens.systemRed)
                            .font(.system(size: 12))
                        Text(error)
                            .font(.system(size: 11, weight: .regular))
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
                            .font(.system(size: 12))
                        Text(warning)
                            .font(.system(size: 11, weight: .regular))
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
        slice.status == "Todo" && !isEditingBrief && !isLoading
    }

    private var editBriefHelp: String {
        guard slice.status != "Todo" else { return "" }
        return "Only a Todo slice's brief can be edited"
    }

    private func startEditingBrief() {
        editedBriefText = sliceDetail?.brief ?? ""
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
                .font(.system(size: 13, weight: .regular))
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
                    .font(.system(size: 11, weight: .regular))
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
            // Refreshes the slice detail — this view holds no cache of its
            // own beside `sliceDetail`, so re-loading it is what "invalidate
            // and refresh" means here.
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
        guard sliceDetail != nil else { return false }

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

                // Switch to Agent tab
                withAnimation {
                    onTabChange(.agent)
                }

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
                    .font(.system(size: 12, weight: .semibold))
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
                    .font(.system(size: 12, weight: .semibold))
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
                .font(.system(size: 10, weight: .regular))
                .foregroundStyle(DesignTokens.labelTertiary)
        }
        .frame(width: 280)
    }

    private func loadDetail() async {
        isLoading = true
        error = nil
        sliceDetail = nil

        do {
            if let projectID = appModel.projectStore?.projectID {
                let detail = try await NatClient().sliceShow(projectID: projectID, sliceRef: slice.id)
                sliceDetail = detail
            }
        } catch {
            self.error = error.localizedDescription
        }

        isLoading = false
    }

    private func dependencyText(_ detail: SliceDetail) -> String {
        if detail.blocked {
            if let deps = detail.dependsOn, !deps.isEmpty {
                return "Waits on \(deps.count) slice\(deps.count == 1 ? "" : "s")"
            } else {
                return "Blocked"
            }
        } else if let deps = detail.dependsOn, !deps.isEmpty {
            return "Waits on \(deps.count) slice\(deps.count == 1 ? "" : "s")"
        } else {
            return "Nothing depends on this slice"
        }
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
