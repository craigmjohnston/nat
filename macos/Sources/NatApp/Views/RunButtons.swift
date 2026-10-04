import SwiftUI
import NatKit

/// A handed-back slice's runs, among its Task section's header actions: a
/// split button. The main part,
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
    /// Whether a run, by label, is starting or still live — disabled
    /// wherever it is offered, so it is not started twice.
    var isRunning: (String) -> Bool = { _ in false }
    /// Run a label — nil for the default.
    let onRun: (String?) -> Void

    @Environment(\.isEnabled) private var isEnabled

    private var defaultLabel: String { runs.first?.label ?? "Run" }
    /// The default already running: the main part greys, the chevron stays
    /// live so another run can still be picked.
    private var defaultRunning: Bool { runs.first.map { isRunning($0.label) } ?? false }

    var body: some View {
        HStack(spacing: 0) {
            Button { onRun(nil) } label: {
                HeaderActionLabel(title: defaultLabel, systemImage: isBusy ? nil : "play.fill", isBusy: isBusy)
            }
            .buttonStyle(GnatHeaderButtonStyle())
            .disabled(defaultRunning)
            .help(defaultRunning ? "\(defaultLabel) is running" : "Run \(defaultLabel) in this task's worktree")
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
            RunMenuList(runs: runs, isRunning: isRunning) { label in
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
/// label, its command in mono under it — the default marked; a run already
/// running greyed and not to be picked.
struct RunMenuList: View {
    let runs: [RunCommand]
    var isRunning: (String) -> Bool = { _ in false }
    let onPick: (String) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(Array(runs.enumerated()), id: \.element.label) { index, run in
                let running = isRunning(run.label)
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
                                    .ink(running ? .tertiary : .primary)
                                if index == 0 {
                                    Text("default").monoXS().ink(.tertiary)
                                }
                            }
                            Text(run.command)
                                .font(Typo.mono(size: Typo.subhead))
                                .ink(running ? .tertiary : .secondary)
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }
                        Spacer(minLength: 0)
                    }
                    .padding(.horizontal, 10)
                    .padding(.vertical, 6)
                    .hoverWash(cornerRadius: 5, enabled: !running)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .disabled(running)
                .help(running ? "\(run.label) is running" : "")
            }
        }
        .padding(5)
        .frame(minWidth: 200, maxWidth: 320, alignment: .leading)
    }
}

/// The titlebar's run button: a play glyph, beside Settings, drawn while
/// any project has runs to offer. It opens the run tree (`RunTreePicker`) —
/// projects, then the open one's runs — and a pick runs that project's run
/// from its origin/main. It spins while any run is starting or still live.
struct TitlebarRunButton: View {
    @Bindable var appModel: AppModel
    @State private var treeOpen = false

    var body: some View {
        let projects = appModel.runProjects
        if !projects.isEmpty {
            Button { treeOpen.toggle() } label: {
                Group {
                    if !appModel.anyRunBusy {
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
                RunTreePicker(
                    projects: projects, openProjectID: appModel.activeProjectID,
                    isRunning: { appModel.isRunning(projectID: $0, sliceID: nil, label: $1) }
                ) { project, label in
                    treeOpen = false
                    Task { await appModel.startRun(projectID: project, label: label) }
                }
            }
        }
    }
}

/// The titlebar's run tree, the breadcrumb's tree picker's shape
/// (`CrumbTreePicker`): every project with runs, then the open project's
/// runs, a column apiece — each run its label over its command, the first
/// marked default. It opens on `openProjectID` where that project has runs,
/// else the first; picking a run runs it and closes the tree. A run already
/// running is greyed and not to be picked.
struct RunTreePicker: View {
    let projects: [RunProject]
    @State private var openID: String
    /// Whether a project's run, by label, is starting or still live.
    let isRunning: (String, String) -> Bool
    /// A pick: the project and the run's label.
    let onPick: (String, String) -> Void

    init(
        projects: [RunProject], openProjectID: String?, isRunning: @escaping (String, String) -> Bool = { _, _ in false },
        onPick: @escaping (String, String) -> Void
    ) {
        self.projects = projects
        self.isRunning = isRunning
        let open = projects.first { $0.id == openProjectID } ?? projects.first
        _openID = State(initialValue: open?.id ?? "")
        self.onPick = onPick
    }

    private static let columnWidth: CGFloat = 230
    private static let height: CGFloat = 240

    private var open: RunProject? { projects.first { $0.id == openID } }

    var body: some View {
        HStack(spacing: 0) {
            column {
                ForEach(projects) { project in
                    row(selected: project.id == openID, opens: true) {
                        StackedFolderGlyph(
                            open: project.id == openID,
                            color: DesignTokens.ink(.tertiary, on: .header),
                            backColor: DesignTokens.ink(.tertiary, on: .header))
                            .frame(width: 16)
                        Text(project.name)
                    } action: {
                        openID = project.id
                    }
                }
            }
            DesignTokens.rule(.separator, on: .header).frame(width: 1)
            column {
                if let open {
                    ForEach(Array(open.runs.enumerated()), id: \.offset) { index, run in
                        runRow(run, isDefault: index == 0, running: isRunning(open.id, run.label)) {
                            onPick(open.id, run.label)
                        }
                    }
                }
            }
        }
        .frame(height: Self.height)
        .surface(.header)
    }

    private func column<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        ScrollView {
            VStack(spacing: 0) { content() }
                .padding(.vertical, 6)
        }
        .thinScrollers()
        .frame(width: Self.columnWidth)
    }

    private func runRow(_ run: RunCommand, isDefault: Bool, running: Bool, action: @escaping () -> Void) -> some View {
        // Running, the label drops to the tertiary ink and the glyph and
        // command, tertiary already, a step below it, so the row reads greyed.
        HStack(alignment: .firstTextBaseline, spacing: 7) {
            Image(systemName: "play.fill")
                .font(.system(size: 8.5))
                .ink(running ? .quaternary : .tertiary)
                .frame(width: 16)
            VStack(alignment: .leading, spacing: 1) {
                HStack(spacing: 6) {
                    Text(run.label).ink(running ? .tertiary : .secondary)
                    if isDefault { Text("default").monoXS().ink(.tertiary) }
                }
                .font(.system(size: GnatMetrics.body))
                Text(run.command)
                    .font(Typo.mono(size: Typo.subhead))
                    .ink(running ? .quaternary : .tertiary)
                    .truncationMode(.middle)
            }
            .lineLimit(1)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 5)
        .gnatRow(selected: false, hoverable: !running)
        .contentShape(Rectangle())
        .onTapGesture { if !running { action() } }
        .help(running ? "\(run.label) is running" : run.command)
    }

    private func row<Label: View>(
        selected: Bool, opens: Bool, @ViewBuilder label: () -> Label, action: @escaping () -> Void
    ) -> some View {
        HStack(spacing: 7) {
            label()
                .font(.system(size: GnatMetrics.body))
                .ink(.secondary)
                .lineLimit(1)
                .truncationMode(.tail)
            Spacer(minLength: 0)
            if opens {
                Image(systemName: "chevron.right")
                    .font(.system(size: 9, weight: .semibold))
                    .ink(.tertiary)
            }
        }
        .padding(.horizontal, 10)
        .frame(height: GnatMetrics.sidebarRowHeight)
        .gnatRow(selected: selected)
        .contentShape(Rectangle())
        .onTapGesture(perform: action)
    }
}
