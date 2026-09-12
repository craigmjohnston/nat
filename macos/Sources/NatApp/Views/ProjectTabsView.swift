import AppKit
import SwiftUI
import NatKit

struct ProjectTabsView: View {
    @Bindable var appModel: AppModel

    /// What the "+" beside the strip does: open the sheet offering the two
    /// ways a project comes to be on the board. The sheet is the shell's, not
    /// this row's — a sheet presented from inside the header band would be
    /// anchored to a 40pt strip.
    let onNewProject: () -> Void
    /// The tab the mouse is over, for the hover wash — parent state rather
    /// than per-tab, because the tabs are built by a function and a function
    /// has no `@State` of its own to keep.
    @State private var hoveredTabID: String?

    var body: some View {
        HStack(alignment: .bottom, spacing: 0) {
            ForEach(Array(appModel.projectTabs.enumerated()), id: \.element.id) { index, tab in
                let liveCount = appModel.liveCount(projectID: tab.id)
                let isActive = appModel.activeProjectID == tab.id
                let color = colorForProject(tab.id)

                projectTabView(tab: tab, liveCount: liveCount, isActive: isActive, color: color, index: index)

                // Hairline separator after an inactive tab, unless the next
                // one is active (a filled tab is its own edge). The last
                // tab has no next, so an inactive one carries the divider —
                // the mock's rule, and what puts a divider between the strip
                // and the "+" beside it.
                let nextIsActive = index + 1 < appModel.projectTabs.count
                    && appModel.activeProjectID == appModel.projectTabs[index + 1].id
                if !isActive && !nextIsActive {
                    Rectangle()
                        .fill(DesignTokens.rule(.border, on: .header))
                        // Full height, like the tabs either side of it: the
                        // strip is a row of abutting cells now, and a rule
                        // that stopped short would read as a gap between
                        // them rather than as the edge where they meet.
                        .frame(width: 1, height: 40)
                }
            }

            // "+" — the two ways a project comes to be on the board, both
            // behind one sheet.
            Button(action: onNewProject) {
                Image(systemName: "plus")
                    .font(.system(size: 14, weight: .medium))
                    .ink(.secondary)
                    .frame(width: 32, height: 32)
            }
            .buttonStyle(.plain)
            .hoverWash(cornerRadius: 7)
            .help("Open or Create a Project…")
            // The box is 32 in a 40pt band, so 4 off the foot is what puts
            // its icon on the same content line the full-height tabs' labels
            // and the toolbar cluster sit on.
            .padding(.bottom, 4)
            .padding(.leading, 6)

            Spacer()
        }
        // The row is 40pt tall outright, and a tab is the whole of it: the
        // strip is a row of hard-edged cells filling the band rather than
        // browser chrome raised off its foot, so there is no headroom above
        // a tab for an alignment to decide.
        .frame(height: 40, alignment: .bottom)
    }

    @ViewBuilder
    private func projectTabView(
        tab: (id: String, name: String),
        liveCount: Int,
        isActive: Bool,
        color: Color?,
        index: Int
    ) -> some View {
        HStack(spacing: 7) {
            // Colored dot (pulsing when live sessions exist)
            if let color = color {
                if liveCount > 0 && !isActive {
                    Circle()
                        .fill(color)
                        .frame(width: 8, height: 8)
                        .modifier(PulseModifier())
                } else {
                    Circle()
                        .fill(color)
                        .frame(width: 8, height: 8)
                }
            }

            // Project name. The width is reserved at semibold whichever
            // weight is drawn — activating a tab bolds its label, and a
            // label measured at its own weight resized the whole tab with
            // every switch.
            Text(tab.name)
                .font(.system(size: Typo.subhead, weight: .semibold))
                .lineLimit(1)
                .opacity(0)
                .overlay(alignment: .leading) {
                    Text(tab.name)
                        .font(.system(size: Typo.subhead, weight: isActive ? .semibold : .regular))
                        .ink(isActive ? .primary : .secondary)
                        .lineLimit(1)
                }

            // Count badge — tight, caption-scale, tabular digits rather than
            // a switch to monospaced design (there's no code here to align).
            if liveCount > 0 {
                // A badge is a chip: its own tint washed into the band behind
                // it, with a word readable on that. It used to fill with
                // `labelQuaternary` and write `labelTertiary` on it — ink as
                // ground, the same mistake as the hover fill, and the last one
                // left in the app.
                Text("\(liveCount)")
                    .font(.system(size: Typo.caption, weight: .regular))
                    .monospacedDigit()
                    .foregroundStyle(DesignTokens.chipInk(.labelSecondary, on: .header))
                    .padding(.horizontal, 6)
                    .background(DesignTokens.wash(.chip, tone: .labelSecondary, on: .header))
                    .cornerRadius(8)
            }

            // Everything above is the tab's identity and reads from its
            // leading edge; the close button belongs to the trailing one,
            // browser-fashion, with the tab's own width between them. It sat
            // hard against the label before, which on a tab stretched to its
            // 220pt maximum left some 120pt of tab to the right of it — every
            // click aimed where a ✕ lives landed on the tab body and
            // activated it instead, which is what made the button read as
            // dead when it had always worked.
            //
            // A Spacer rather than a reserved slot: the label is leading-
            // aligned whatever follows it, so nothing moves as the ✕ fades in
            // under the mouse.
            Spacer(minLength: 0)

            if ProjectTabRules.showsClose(tabCount: appModel.projectTabs.count) {
                let showClose = ProjectTabRules.closeIsVisible(
                    isActive: isActive,
                    isHovered: hoveredTabID == tab.id
                )
                Button(action: {
                    Task { await appModel.closeProject(tab.id) }
                }) {
                    Image(systemName: "xmark")
                        .font(.system(size: 9, weight: .semibold))
                        .ink(.tertiary)
                        .frame(width: 16, height: 16)
                }
                .buttonStyle(.plain)
                .hoverWash(cornerRadius: 4)
                .opacity(showClose ? 1 : 0)
                // The same answer the opacity takes: a ✕ faded out is one
                // nobody can see, and it sits exactly where a click meaning
                // to select the tab would land.
                .allowsHitTesting(showClose)
                .help("Close Tab")
            }
        }
        .padding(.horizontal, 22)
        // The tab fills the band outright rather than being seated on its
        // foot: no bottom padding to lift it and no shortfall to center its
        // contents in — the 40pt cell's own center is the band's center line,
        // which is where the toolbar cluster and the slice count already sit.
        .frame(height: 40)
        // Tab bounds: no narrower than 130 whatever its name, no wider than
        // 220 however long — padding included, as the mock's border-box is.
        .frame(minWidth: 130, maxWidth: 220, alignment: .leading)
        .background(
            // A flat cell, not browser chrome: square corners, full height
            // and hard against its neighbours, so the strip reads as a row
            // of abutting tabs. An inactive tab under the mouse takes the
            // hover wash in the same rectangle, so what lights up is exactly
            // what a click would raise.
            Rectangle()
                .fill(
                    isActive
                        ? DesignTokens.fill(.window)
                        : (hoveredTabID == tab.id ? DesignTokens.fill(.hover) : Color.clear)
                )
        )
        .contentShape(Rectangle())
        .onHover { inside in
            if inside {
                hoveredTabID = tab.id
            } else if hoveredTabID == tab.id {
                hoveredTabID = nil
            }
        }
        .onTapGesture {
            Task {
                await appModel.activateProject(tab.id)
            }
        }
        .contextMenu {
            tabMenu(tab: tab)
        }
    }

    /// The right-click menu on a project tab: close it, open its page in
    /// Notion, or show the checkout its agents work in.
    ///
    /// Close is `closeProject` itself, which is what lets an inactive tab be
    /// closed without being switched to first — the ✕ is the same call, and
    /// the last tab standing carries neither, since a board with no project
    /// is the onboarding screen's shape. Reveal is offered only where config
    /// records a working directory: a project opened from the "+" tab has
    /// none until Settings is given one.
    @ViewBuilder
    private func tabMenu(tab: (id: String, name: String)) -> some View {
        if ProjectTabRules.showsClose(tabCount: appModel.projectTabs.count) {
            Button("Close Tab") {
                Task { await appModel.closeProject(tab.id) }
            }

            Divider()
        }

        if let url = NotionPageURL.forPage(tab.id) {
            Button("Open in Notion") {
                NSWorkspace.shared.open(url)
            }
        }

        if let directory = workingDirectory(of: tab.id) {
            Button("Reveal Working Directory in Finder") {
                NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: directory)])
            }
        }
    }

    /// The project's checkout as local config records it, or nil where it
    /// records none — which is every project opened rather than created.
    private func workingDirectory(of projectID: String) -> String? {
        let directory = appModel.config?.projects[projectID]?.workingDir
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return directory.isEmpty ? nil : directory
    }

    /// Return a color for each project (cycling through a palette).
    private func colorForProject(_ projectID: String) -> Color? {
        let colors: [Color] = [
            DesignTokens.systemOrange,
            DesignTokens.systemGreen,
            DesignTokens.systemYellow,
            DesignTokens.systemRed
        ]

        // Use hash to consistently map projects to colors
        let hash = projectID.hashValue
        let index = abs(hash) % colors.count
        return colors[index]
    }
}

// MARK: - Helpers

struct PulseModifier: ViewModifier {
    @State private var isAnimating = false

    func body(content: Content) -> some View {
        content
            .opacity(isAnimating ? 0.6 : 1)
            .animation(
                Animation.easeInOut(duration: 1.5)
                    .repeatForever(autoreverses: true),
                value: isAnimating
            )
            .onAppear {
                isAnimating = true
            }
    }
}


#Preview {
    let appModel = AppModel()
    ProjectTabsView(appModel: appModel, onNewProject: {})
        .frame(height: 40)
}
