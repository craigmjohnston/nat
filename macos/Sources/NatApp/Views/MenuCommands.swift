import SwiftUI

/// What the menu bar can reach of the window. Each part is published by the
/// view that already owns the action (`focusedSceneValue`) — the sidebar its
/// sheets, the shell its new project, the slice navigator its launch, brief
/// and pull request — so a menu item runs exactly what the control beside
/// it runs, and is disabled (a nil action) wherever that control is not.
struct SidebarMenuActions {
    var newSlice: (() -> Void)?
    var newMilestone: (() -> Void)?
    var newSession: (() -> Void)?
    var workshop: (() -> Void)?
    var revealWorkingDirectory: (() -> Void)?
    var openProjectInNotion: (() -> Void)?
}

struct ShellMenuActions {
    var newProject: () -> Void
    var refresh: () -> Void
}

struct SliceMenuActions {
    /// The slice's name, which heads the Slice menu so every item in it
    /// reads as being about that slice.
    var title: String?
    var launch: (() -> Void)?
    var editBrief: (() -> Void)?
    var merge: (() -> Void)?
    var openPullRequest: (() -> Void)?
    var openInNotion: (() -> Void)?
    var showThread: (() -> Void)?
    var showChanges: (() -> Void)?
    var showVisuals: (() -> Void)?
    var showPullRequest: (() -> Void)?
}

struct WorkshopMenuActions {
    var showTerminal: (() -> Void)?
    var showPlan: (() -> Void)?
}

extension FocusedValues {
    @Entry var sidebarMenu: SidebarMenuActions?
    @Entry var shellMenu: ShellMenuActions?
    @Entry var sliceMenu: SliceMenuActions?
    @Entry var workshopMenu: WorkshopMenuActions?
}

/// File's new-item group, View's own items and the Slice menu.
struct GnatCommands: Commands {
    @Binding var showsDoneItems: Bool
    @Binding var diffWrapsLines: Bool

    @FocusedValue(\.sidebarMenu) private var sidebar
    @FocusedValue(\.shellMenu) private var shell
    @FocusedValue(\.sliceMenu) private var slice
    @FocusedValue(\.workshopMenu) private var workshop

    var body: some Commands {
        // One window, so no New Window: File ▸ New makes plan items instead.
        CommandGroup(replacing: .newItem) {
            item("New project\u{2026}", shell?.newProject).keyboardShortcut("n", modifiers: [.command, .shift])
            item("Workshop\u{2026}", sidebar?.workshop)
            item("New milestone\u{2026}", sidebar?.newMilestone).keyboardShortcut("n", modifiers: [.command, .option])
            item("New task\u{2026}", sidebar?.newSlice).keyboardShortcut("n")
            item("New ad hoc session", sidebar?.newSession).keyboardShortcut("n", modifiers: [.command, .control])
            Divider()
            item("Open project in Notion", sidebar?.openProjectInNotion)
            item("Reveal working directory in Finder", sidebar?.revealWorkingDirectory)
        }

        // The View menu's own first group: Finder's Show/Hide Hidden Files,
        // for finished work, on the same ⇧⌘. Finder uses; then refreshing,
        // the navigator's three sections, and how the diff sets long lines.
        CommandGroup(before: .toolbar) {
            Button(showsDoneItems ? "Hide done items" : "Show done items",
                   systemImage: showsDoneItems ? "eye.slash" : "eye") {
                showsDoneItems.toggle()
            }
            .keyboardShortcut(".", modifiers: [.command, .shift])
            item("Refresh", shell?.refresh).keyboardShortcut("r")
            Divider()
            // The workshop's two tabs take ⌘1 and ⌘2 while it is on screen.
            if let workshop {
                item("Terminal", workshop.showTerminal).keyboardShortcut("1")
                item("Plan", workshop.showPlan).keyboardShortcut("2")
            } else {
                item("Task", slice?.showThread).keyboardShortcut("1")
                item("Changes", slice?.showChanges).keyboardShortcut("2")
                // Out of numeric order on purpose, so the menu reads in the
                // navigator's order; ⌘3 stays Pull request's.
                item("Visual changes", slice?.showVisuals).keyboardShortcut("4")
                item("Pull request", slice?.showPullRequest).keyboardShortcut("3")
            }
            Divider()
            Toggle("Wrap lines in diffs", isOn: $diffWrapsLines)
            Divider()
        }

        // The menu's own title stays "Task"; which task is said by a
        // section header over its items, so a long name never reaches the
        // menu bar.
        CommandMenu("Task") {
            Section(sliceMenuHeader(slice?.title)) {
                item("Launch agent", slice?.launch).keyboardShortcut("l")
                item("Edit brief\u{2026}", slice?.editBrief)
            }
            Divider()
            item("Merge pull request\u{2026}", slice?.merge)
            item("Open pull request in GitHub", slice?.openPullRequest)
            item("Open task in Notion", slice?.openInNotion)
        }
    }

    /// The Slice menu's header: the slice's name, cut to a menu's width.
    private func sliceMenuHeader(_ title: String?) -> String {
        guard let title, !title.isEmpty else { return "No task selected" }
        return title.count > 48 ? title.prefix(47).trimmingCharacters(in: .whitespaces) + "\u{2026}" : title
    }

    private func item(_ title: String, _ action: (() -> Void)?) -> some View {
        Button(title) { action?() }.disabled(action == nil)
    }
}
