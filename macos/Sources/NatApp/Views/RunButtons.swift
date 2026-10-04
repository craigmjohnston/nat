import SwiftUI
import NatKit

/// The one control both run placements draw: a split button. The main part,
/// `▶ <label>`, runs the default — `nat run` with no `--label`, so nat picks
/// it — and the split part, past a thin divider, is a chevron opening the
/// menu of every run of that scope (`RunMenuList`), the default marked.
///
/// Two sizes: `.titlebar` is compact, sized to its text and washed under the
/// pointer as the titlebar's own glyph buttons are, with the project's badge
/// before the label; `.header` is full bleed to a navigator header's height,
/// in `GnatHeaderButtonStyle`'s shape — `HeaderActionLabel`'s words and glyph.
struct RunSplitButton: View {
    enum Size { case titlebar, header }

    let size: Size
    /// The runs of the scope, the default first; never empty where drawn.
    let runs: [RunCommand]
    /// The project's short mark, drawn before the label in the titlebar.
    var badge: String?
    var isBusy = false
    /// Whether the menu is open — the popover's, or a story's drawn below.
    @Binding var menuOpen: Bool
    /// Run a label — nil for the default.
    let onRun: (String?) -> Void

    @Environment(\.isEnabled) private var isEnabled

    private var defaultLabel: String { runs.first?.label ?? "Run" }

    var body: some View {
        HStack(spacing: 0) {
            mainPart
            divider
            chevronPart
        }
        .fixedSize(horizontal: true, vertical: size == .titlebar)
        .popover(isPresented: $menuOpen, arrowEdge: .bottom) {
            RunMenuList(runs: runs) { label in
                menuOpen = false
                onRun(label)
            }
        }
    }

    // MARK: - Parts

    @ViewBuilder
    private var mainPart: some View {
        switch size {
        case .titlebar:
            Button { onRun(nil) } label: { titlebarLabel }
                .buttonStyle(RunTitlebarPartStyle())
                .help("Run \(defaultLabel) from origin/main")
        case .header:
            Button { onRun(nil) } label: {
                HeaderActionLabel(title: defaultLabel, systemImage: isBusy ? nil : "play.fill", isBusy: isBusy)
            }
            .buttonStyle(GnatHeaderButtonStyle())
            .help("Run \(defaultLabel) in this task's worktree")
        }
    }

    private var titlebarLabel: some View {
        HStack(spacing: 5) {
            if isBusy {
                ProgressView().controlSize(.mini).frame(width: 9, height: 9)
            } else {
                Image(systemName: "play.fill").font(.system(size: 8.5))
            }
            if let badge, !badge.isEmpty {
                Text(badge)
                    .font(Typo.mono(size: Typo.scaled(10), weight: .medium))
                    .tracking(1)
                    .ink(.tertiary)
            }
            Text(defaultLabel)
                .font(.system(size: GnatMetrics.titlebarText))
                .lineLimit(1)
        }
        .ink(.secondary)
    }

    private var divider: some View {
        DesignTokens.rule(.separator, on: size == .titlebar ? .header : .chrome)
            .frame(width: 1)
            .padding(.vertical, size == .titlebar ? 4 : 8)
            .opacity(isEnabled ? 1 : 0.4)
    }

    @ViewBuilder
    private var chevronPart: some View {
        let chevron = Image(systemName: "chevron.down")
            .font(.system(size: size == .titlebar ? 8 : 9, weight: .semibold))
            .accessibilityLabel("Choose a run")
        switch size {
        case .titlebar:
            Button { menuOpen.toggle() } label: { chevron.ink(.tertiary) }
                .buttonStyle(RunTitlebarPartStyle(horizontal: 5))
                .help("Choose a run")
        case .header:
            Button { menuOpen.toggle() } label: { chevron }
                .buttonStyle(RunHeaderChevronStyle())
                .help("Choose a run")
        }
    }
}

/// A part of the titlebar's split button: bare at rest, the hover wash under
/// the pointer as `GnatIconButtonStyle` draws it — the titlebar's own
/// control style — dimmed while disabled.
private struct RunTitlebarPartStyle: ButtonStyle {
    var horizontal: CGFloat = 6
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .padding(.horizontal, horizontal)
            .frame(height: 22)
            .hoverWash(cornerRadius: 4, enabled: isEnabled)
            .opacity(isEnabled ? (configuration.isPressed ? 0.7 : 1) : 0.4)
            .contentShape(Rectangle())
    }
}

/// The header split button's chevron: `GnatHeaderButtonStyle`'s full-bleed
/// part, narrower — a glyph needs less room than words.
private struct RunHeaderChevronStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        HoverReader { hovering in
            configuration.label
                .padding(.horizontal, 9)
                .frame(maxHeight: .infinity)
                .foregroundStyle(DesignTokens.ink(.primary, on: .chrome))
                .background(hovering ? DesignTokens.rowWash(selected: false, on: .chrome) : .clear)
                .opacity(isEnabled ? (configuration.isPressed ? 0.85 : 1) : 0.4)
                .contentShape(Rectangle())
        }
    }
}

/// The split part's menu, drawn as a view of gnat's own rather than an
/// `NSMenu` so a story can render it: one row per run of the scope — its
/// label, its command in mono under it — the default marked.
struct RunMenuList: View {
    let runs: [RunCommand]
    let onPick: (String) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(Array(runs.enumerated()), id: \.element.label) { index, run in
                Button { onPick(run.label) } label: {
                    HStack(alignment: .firstTextBaseline, spacing: 8) {
                        Image(systemName: "play.fill")
                            .font(.system(size: 8.5))
                            .ink(.tertiary)
                            .frame(width: 10)
                        VStack(alignment: .leading, spacing: 2) {
                            HStack(spacing: 6) {
                                Text(run.label)
                                    .font(.system(size: GnatMetrics.body))
                                    .ink(.primary)
                                if index == 0 {
                                    Text("default").monoXS().ink(.tertiary)
                                }
                            }
                            Text(run.command)
                                .font(Typo.mono(size: Typo.subhead))
                                .ink(.secondary)
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }
                        Spacer(minLength: 0)
                    }
                    .padding(.horizontal, 10)
                    .padding(.vertical, 6)
                    .hoverWash(cornerRadius: 5)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
        }
        .padding(5)
        .frame(minWidth: 200, maxWidth: 320, alignment: .leading)
    }
}

/// The titlebar's global run: the split button for the front project's
/// global runs, drawn only where it has any.
struct TitlebarRunButton: View {
    @Bindable var appModel: AppModel
    @State private var menuOpen = false

    var body: some View {
        if let projectID = appModel.activeProjectID {
            let runs = appModel.globalRuns(ofProject: projectID)
            if !runs.isEmpty {
                RunSplitButton(
                    size: .titlebar, runs: runs, badge: appModel.projectTag(projectID),
                    isBusy: appModel.isStartingRun(projectID: projectID, sliceID: nil), menuOpen: $menuOpen
                ) { label in
                    Task { await appModel.startRun(projectID: projectID, label: label) }
                }
            }
        }
    }
}

/// The navigator's Run heading: a header's height and metrics, with no
/// section under it and no fold — `Run` on the left, the split button as its
/// one action, full bleed. Greyed and disabled once the slice is merged, its
/// worktree being gone.
struct RunHeadingRow: View {
    let runs: [RunCommand]
    let merged: Bool
    var isBusy = false
    @Binding var menuOpen: Bool
    let onRun: (String?) -> Void

    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                // The chevron's slot, empty: the label lines up with every
                // other section's.
                Color.clear.frame(width: 12)
                Text("Run")
                    .font(.system(size: GnatMetrics.body))
                    .ink(merged ? .tertiary : .primary)
                    .fixedSize()
                Spacer(minLength: 0)
                RunSplitButton(size: .header, runs: runs, isBusy: isBusy, menuOpen: $menuOpen, onRun: onRun)
                    .frame(maxHeight: .infinity)
                    .disabled(merged)
            }
            .padding(.leading, 10)
            .frame(height: GnatMetrics.sectionHeadHeight)
            .background(DesignTokens.fill(.chrome))
            DesignTokens.rule(.separator, on: .chrome).frame(height: 1)
        }
    }
}
