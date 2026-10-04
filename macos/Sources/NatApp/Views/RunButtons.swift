import SwiftUI
import NatKit

/// The navigator's Run heading's one action: a split button. The main part,
/// `<label> ▶`, runs the default — `nat run` with no `--label`, so nat picks
/// it — and the split part, past a thin divider, is a chevron opening the
/// menu of every run the slice is offered (`RunMenuList`), the default
/// marked. Full bleed to the header's height, in `GnatHeaderButtonStyle`'s
/// shape — `HeaderActionLabel`'s words and glyph.
struct RunSplitButton: View {
    /// The runs offered, the default first; never empty where drawn.
    let runs: [RunCommand]
    var isBusy = false
    /// Whether the menu is open — the popover's, or a story's.
    @Binding var menuOpen: Bool
    /// Run a label — nil for the default.
    let onRun: (String?) -> Void

    @Environment(\.isEnabled) private var isEnabled

    private var defaultLabel: String { runs.first?.label ?? "Run" }

    var body: some View {
        HStack(spacing: 0) {
            Button { onRun(nil) } label: {
                HeaderActionLabel(title: defaultLabel, systemImage: isBusy ? nil : "play.fill", isBusy: isBusy)
            }
            .buttonStyle(GnatHeaderButtonStyle())
            .help("Run \(defaultLabel) in this task's worktree")
            DesignTokens.rule(.separator, on: .chrome)
                .frame(width: 1)
                .padding(.vertical, 8)
                .opacity(isEnabled ? 1 : 0.4)
            Button { menuOpen.toggle() } label: {
                Image(systemName: "chevron.down")
                    .font(.system(size: 9, weight: .semibold))
                    .accessibilityLabel("Choose a run")
            }
            .buttonStyle(RunHeaderChevronStyle())
            .help("Choose a run")
        }
        .fixedSize(horizontal: true, vertical: false)
        .popover(isPresented: $menuOpen, arrowEdge: .bottom) {
            RunMenuList(runs: runs) { label in
                menuOpen = false
                onRun(label)
            }
        }
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
    /// Whether it stands alone, with a menu's own inset and width.
    var padded = true
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
        .padding(padded ? 5 : 0)
        .frame(minWidth: padded ? 200 : nil, maxWidth: padded ? 320 : nil, alignment: .leading)
    }
}

/// The titlebar's run button: a play glyph, beside Settings, drawn while
/// any project has runs to offer. It opens the run tree (`RunTreeList`) —
/// every such project, its runs under it — and a pick runs that project's
/// run from its origin/main.
struct TitlebarRunButton: View {
    @Bindable var appModel: AppModel
    @State private var treeOpen = false

    var body: some View {
        let projects = appModel.runProjects
        if !projects.isEmpty {
            Button { treeOpen.toggle() } label: {
                Group {
                    if appModel.runsStarting.isEmpty {
                        Image(systemName: "play.fill").font(.system(size: 11))
                    } else {
                        ProgressView().controlSize(.mini)
                    }
                }
                .ink(.tertiary)
                .frame(width: 18, height: 18)
                .contentShape(Rectangle())
            }
            .buttonStyle(GnatIconButtonStyle())
            .help("Run\u{2026}")
            .popover(isPresented: $treeOpen, arrowEdge: .bottom) {
                RunTreeList(projects: projects) { project, label in
                    treeOpen = false
                    Task { await appModel.startRun(projectID: project, label: label) }
                }
            }
        }
    }
}

/// The titlebar's run tree: each project with runs, then its runs, each
/// with its command — the first of each project's marked as its default.
struct RunTreeList: View {
    let projects: [RunProject]
    /// A pick: the project and the run's label.
    let onPick: (String, String) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            ForEach(projects) { project in
                VStack(alignment: .leading, spacing: 0) {
                    Text(project.name)
                        .font(.system(size: GnatMetrics.body, weight: .medium))
                        .ink(.secondary)
                        .lineLimit(1)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 4)
                    RunMenuList(runs: project.runs, padded: false) { onPick(project.id, $0) }
                        .padding(.leading, 12)
                }
            }
        }
        .padding(5)
        .frame(minWidth: 220, maxWidth: 340, alignment: .leading)
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
                RunSplitButton(runs: runs, isBusy: isBusy, menuOpen: $menuOpen, onRun: onRun)
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
