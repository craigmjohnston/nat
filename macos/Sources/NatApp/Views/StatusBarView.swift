import SwiftUI
import NatKit

/// The window's status bar: the gnat mark and how many agents are running,
/// then each Claude usage window, set apart by faint dividers at the leading
/// edge, all in the system sans at `xs` and the one quiet `.tertiary` ink
/// (bar the warning tint where a reading runs high); then, only while nat
/// throttles or has paused polling, GitHub's budget (`GitHubBudgetReadout`,
/// off the last reading); then, only where a newer Claude Code exists, the
/// one update notice (`ClaudeUpdateNotice`), which opens the update window
/// (`ClaudeUpdateSheet`) that runs it; at the trailing edge
/// (`trailing`), the selection's live agent's model, effort and context in
/// the same (`AgentModelHeading`) — a slice's, a session's, the planning
/// agent's — or nothing. Where the selection sits is the titlebar band's
/// breadcrumb.
struct StatusBarView<Trailing: View>: View {
    @Bindable var appModel: AppModel
    @ViewBuilder var trailing: () -> Trailing
    @Environment(\.clock) private var clock

    static var height: CGFloat { GnatMetrics.statusBarHeight }

    private var agentCount: Int {
        appModel.activityStore?.agents.count ?? 0
    }

    var body: some View {
        let usage = buildUsageDisplay(from: appModel.usageStore?.reading, now: clock())
        HStack(spacing: 10) {
            HStack(spacing: 12) {
                GnatMark(color: DesignTokens.mark)
                    .frame(width: 14, height: 14)
                Text("\(agentCount) agent\(agentCount == 1 ? "" : "s")")
                    .ink(.tertiary)
            }
            ForEach(Array(usage.windows.enumerated()), id: \.offset) { _, window in
                StatusBarDivider()
                UsageWindowText(window: window)
            }
            let budget = GitHubBudgetReadout(appModel.githubReadingStore?.rateLimit)
            if let text = budget.text(now: clock()) {
                StatusBarDivider()
                GitHubBudgetText(text: text, tooltip: budget.tooltip(now: clock()), warning: budget.stateWord == "paused")
            }
            if let versions = appModel.claudeVersionStore, let notice = versions.notice {
                StatusBarDivider()
                ClaudeUpdateNotice(text: notice) { versions.confirmUpdate() }
            }
            Spacer(minLength: 16)
            trailing()
        }
        .font(.system(size: GnatMetrics.xs))
        .monospacedDigit()
        .lineLimit(1)
        .padding(.horizontal, 10)
        // Centred in the band under the top rule, which draws over the bar.
        .frame(height: Self.height - 1)
        .padding(.top, 1)
        .surface(.chrome)
        .rule(.separator, edges: [.top], width: 1)
        .sheet(isPresented: Binding(
            get: { appModel.claudeVersionStore?.update != nil },
            set: { if !$0 { appModel.claudeVersionStore?.dismissUpdate() } }
        )) {
            if let versions = appModel.claudeVersionStore, let state = versions.update {
                ClaudeUpdateSheet(
                    state: state, version: versions.version,
                    onUpdate: { Task { await versions.runUpdate() } },
                    onDone: { versions.dismissUpdate() })
            }
        }
    }
}

extension StatusBarView where Trailing == EmptyView {
    init(appModel: AppModel) {
        self.init(appModel: appModel, trailing: { EmptyView() })
    }
}

/// The faint upright line between the status bar's clauses — the leading
/// edge's and `AgentModelHeading`'s alike.
struct StatusBarDivider: View {
    var body: some View {
        DesignTokens.rule(.separator, on: .chrome)
            .frame(width: 1, height: 12)
    }
}

/// One Claude usage window's clause, in the warning tint once it runs high.
private struct UsageWindowText: View {
    let window: UsageWindowDisplay
    @Environment(\.ground) private var ground

    var body: some View {
        Text(window.text)
            .foregroundStyle(window.warning ? DesignTokens.hotInk(on: ground) : DesignTokens.ink(.tertiary, on: ground))
    }
}

/// GitHub's budget clause: the readout's words, its tooltip, in the warning
/// tint while polling is paused.
private struct GitHubBudgetText: View {
    let text: String
    let tooltip: String?
    let warning: Bool
    @Environment(\.ground) private var ground

    var body: some View {
        Text(text)
            .foregroundStyle(warning ? DesignTokens.hotInk(on: ground) : DesignTokens.ink(.tertiary, on: ground))
            .help(tooltip ?? "")
    }
}

/// gnat's Claude Code update notice, as a small chip in the theme's orange.
/// A click opens the update window; nothing runs until its Update.
struct ClaudeUpdateNotice: View {
    let text: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Chip(text, tone: .attention, size: .small, systemImage: "arrow.down.circle").fixedSize()
        }
        .buttonStyle(.plain)
        .help("Update Claude Code")
    }
}

/// The update window: before the update, which version is installed, which
/// it would update to and how, with Cancel and Update; while it runs, a
/// spinner and nothing to press; then that it worked, with the version now
/// installed and what that means for agents already running, or that it
/// failed, the updater's words folded under Show details.
struct ClaudeUpdateSheet: View {
    let state: ClaudeUpdateState
    /// nat's latest reading — after a success, the one read since.
    let version: ClaudeVersion?
    let onUpdate: () -> Void
    let onDone: () -> Void

    @State private var detailsShown = false

    /// Live agents keep the binary they started with; the window says so
    /// rather than leaving the user to wonder why a pane still runs the old.
    static let agentsNote = "Agents already running keep the version they started with. "
        + "Agents launched from now on use the new version."

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: 12) {
                Text(title)
                    .font(.system(size: Typo.headline, weight: .semibold))
                    .ink(.primary)
                switch state {
                case .confirming:
                    facts
                case .running:
                    facts
                    HStack(spacing: 8) {
                        ProgressView().controlSize(.small)
                        Text("Updating Claude Code\u{2026}").ink(.secondary)
                    }
                case .finished:
                    Text("Claude Code \(version?.installedText ?? "unknown") is installed.")
                        .ink(.secondary)
                    Text(Self.agentsNote)
                        .font(.system(size: Typo.subhead))
                        .ink(.tertiary)
                        .fixedSize(horizontal: false, vertical: true)
                case .failed(let message):
                    Text("The update did not finish. The installed version is unchanged.")
                        .ink(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                    DisclosureGroup("Show details", isExpanded: $detailsShown) {
                        outputText(message).ink(.danger).padding(.top, 6)
                    }
                    .font(.system(size: Typo.subhead))
                }
            }
            .padding(.horizontal, 20)
            .padding(.top, 18)

            Rule()
                .padding(.top, 14)

            HStack(spacing: 8) {
                Spacer()
                switch state {
                case .confirming, .running:
                    Button("Cancel", action: onDone)
                        .buttonStyle(SecondaryButtonStyle())
                        .keyboardShortcut(.cancelAction)
                        .disabled(state == .running)
                    Button("Update", action: onUpdate)
                        .buttonStyle(PrimaryButtonStyle())
                        .keyboardShortcut(.defaultAction)
                        .disabled(state == .running)
                case .finished, .failed:
                    Button("Done", action: onDone)
                        .buttonStyle(PrimaryButtonStyle())
                        .keyboardShortcut(.defaultAction)
                }
            }
            .padding(.horizontal, 20)
            .padding(.top, 14)
            .padding(.bottom, 16)
        }
        .frame(width: 420)
        .surface(.card)
        .interactiveDismissDisabled(state == .running)
    }

    /// Installed, the version it would update to, and how — the method's
    /// row only where nat said one.
    private var facts: some View {
        Grid(alignment: .leading, horizontalSpacing: 16, verticalSpacing: 6) {
            factRow("Installed", version?.installedText ?? "unknown")
            factRow("Update to", version?.latest ?? "unknown")
            if let method = version?.updateMethod {
                factRow("Updates with", method.description)
            }
        }
    }

    private func factRow(_ key: String, _ value: String) -> some View {
        GridRow {
            Text(key).ink(.tertiary)
            Text(value).ink(.primary).textSelection(.enabled)
        }
    }

    /// nat's refusal, carrying the updater's words, as printed: monospaced,
    /// selectable, scrolling past a few lines.
    private func outputText(_ text: String) -> some View {
        ScrollView {
            Text(text.trimmingCharacters(in: .whitespacesAndNewlines))
                .font(Typo.mono(size: GnatMetrics.xs))
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .thinScrollers()
        .frame(maxHeight: 120)
        .fixedSize(horizontal: false, vertical: true)
    }

    private var title: String {
        switch state {
        case .confirming: "Update Claude Code"
        case .running: "Updating Claude Code"
        case .finished: "Claude Code updated"
        case .failed: "Claude Code could not update"
        }
    }
}

#Preview {
    StatusBarView(appModel: AppModel())
        .frame(width: 1320)
}
