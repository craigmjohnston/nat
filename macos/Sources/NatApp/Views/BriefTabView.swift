import SwiftUI
import NatKit

struct BriefTabView: View {
    @Bindable var appModel: AppModel
    let slice: Slice
    @State private var sliceDetail: SliceDetail?
    @State private var isLoading = false
    @State private var error: String?

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

                        // Edit button (disabled)
                        Button(action: {}) {
                            Text("Edit…")
                                .font(.system(size: 11, weight: .regular))
                        }
                        .buttonStyle(.borderless)
                        .disabled(true)
                    }

                    // Brief content
                    if isLoading {
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

            HStack(spacing: 8) {
                Button(action: {}) {
                    Text("Edit Brief…")
                        .font(.system(size: 11, weight: .regular))
                }
                .buttonStyle(.borderless)
                .disabled(true)

                Spacer()

                // Split Launch Agent button
                HStack(spacing: 0) {
                    Button(action: {}) {
                        HStack(spacing: 0) {
                            Text("Launch Agent")
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundStyle(DesignTokens.accentText)
                                .padding(.horizontal, 10)
                                .frame(height: 22)
                        }
                        .background(DesignTokens.accent)
                    }
                    .buttonStyle(.plain)
                    .disabled(true)

                    Divider()
                        .frame(maxHeight: 22)
                        .opacity(0.25)

                    Button(action: {}) {
                        Image(systemName: "chevron.down")
                            .font(.system(size: 9, weight: .bold))
                            .foregroundStyle(DesignTokens.accentText)
                            .frame(width: 20, height: 22)
                    }
                    .buttonStyle(.plain)
                    .disabled(true)
                }
                .background(DesignTokens.accent)
                .cornerRadius(4)
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .background(DesignTokens.controlBg)
        }
        .background(DesignTokens.windowBg)
        .task {
            await loadDetail()
        }
        .onChange(of: slice.id) { _, _ in
            Task {
                await loadDetail()
            }
        }
    }

    // MARK: - Helpers

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
