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
    var launch: (() -> Void)?
    var editBrief: (() -> Void)?
    var merge: (() -> Void)?
    var openPullRequest: (() -> Void)?
    var openInNotion: (() -> Void)?
    var showThread: (() -> Void)?
    var showChanges: (() -> Void)?
    var showPullRequest: (() -> Void)?
}

extension FocusedValues {
    @Entry var sidebarMenu: SidebarMenuActions?
    @Entry var shellMenu: ShellMenuActions?
    @Entry var sliceMenu: SliceMenuActions?
}

/// File's new-item group, View's own items and the Slice menu.
struct GnatCommands: Commands {
    @Binding var showsDoneItems: Bool

    @FocusedValue(\.sidebarMenu) private var sidebar
    @FocusedValue(\.shellMenu) private var shell
    @FocusedValue(\.sliceMenu) private var slice

    var body: some Commands {
        // One window, so no New Window: File ▸ New makes plan items instead.
        CommandGroup(replacing: .newItem) {
            item("New Project\u{2026}", shell?.newProject).keyboardShortcut("n", modifiers: [.command, .shift])
            item("New Slice\u{2026}", sidebar?.newSlice).keyboardShortcut("n")
            item("New Milestone\u{2026}", sidebar?.newMilestone).keyboardShortcut("n", modifiers: [.command, .option])
            item("New Ad Hoc Session", sidebar?.newSession).keyboardShortcut("n", modifiers: [.command, .control])
            item("Workshop\u{2026}", sidebar?.workshop)
            Divider()
            item("Open Project in Notion", sidebar?.openProjectInNotion)
            item("Reveal Working Directory in Finder", sidebar?.revealWorkingDirectory)
        }

        // The View menu's own first group: Finder's Show/Hide Hidden Files,
        // for finished work, on the same ⇧⌘. Finder uses; then refreshing,
        // and the navigator's three sections.
        CommandGroup(before: .toolbar) {
            Button(showsDoneItems ? "Hide Done Items" : "Show Done Items",
                   systemImage: showsDoneItems ? "eye.slash" : "eye") {
                showsDoneItems.toggle()
            }
            .keyboardShortcut(".", modifiers: [.command, .shift])
            item("Refresh", shell?.refresh).keyboardShortcut("r")
            Divider()
            item("Thread", slice?.showThread).keyboardShortcut("1")
            item("Changes", slice?.showChanges).keyboardShortcut("2")
            item("Pull Request", slice?.showPullRequest).keyboardShortcut("3")
            Divider()
        }

        CommandMenu("Slice") {
            item("Launch Agent", slice?.launch).keyboardShortcut("l")
            item("Edit Brief\u{2026}", slice?.editBrief)
            Divider()
            item("Merge Pull Request\u{2026}", slice?.merge)
            item("Open Pull Request in GitHub", slice?.openPullRequest)
            item("Open Slice in Notion", slice?.openInNotion)
        }
    }

    private func item(_ title: String, _ action: (() -> Void)?) -> some View {
        Button(title) { action?() }.disabled(action == nil)
    }
}
