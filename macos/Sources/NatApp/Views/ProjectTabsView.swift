import SwiftUI
import NatKit

struct ProjectTabsView: View {
    @Bindable var appModel: AppModel

    var body: some View {
        HStack(alignment: .bottom, spacing: 0) {
            ForEach(Array(appModel.projectTabs.enumerated()), id: \.element.id) { index, tab in
                let liveCount = appModel.liveCount(projectID: tab.id)
                let isActive = appModel.activeProjectID == tab.id
                let color = colorForProject(tab.id)

                projectTabView(tab: tab, liveCount: liveCount, isActive: isActive, color: color, index: index)

                // Hairline separator between inactive tabs
                if !isActive && index + 1 < appModel.projectTabs.count {
                    let nextTab = appModel.projectTabs[index + 1]
                    let nextIsActive = appModel.activeProjectID == nextTab.id
                    if !nextIsActive {
                        Divider()
                            .frame(width: 1, height: 16)
                            .foregroundStyle(DesignTokens.labelQuaternary)
                    }
                }
            }

            // "+" button (disabled)
            Button(action: {}) {
                Image(systemName: "plus")
                    .font(.system(size: 14, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
            }
            .frame(width: 32, height: 32)
            .cornerRadius(7)
            .disabled(true)
            .help("Projects are created from the TUI or nat project-create")
            .padding(.bottom, 6)
            .padding(.leading, 6)

            Spacer()
        }
        .frame(height: 40)
        .padding(.top, 6)
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

            // Project name
            Text(tab.name)
                .font(.system(size: 12, weight: isActive ? .semibold : .regular))
                .foregroundStyle(isActive ? DesignTokens.label : DesignTokens.labelSecondary)
                .lineLimit(1)

            // Count badge
            if liveCount > 0 {
                Text("\(liveCount)")
                    .font(.system(size: 10, weight: .regular, design: .monospaced))
                    .foregroundStyle(isActive ? DesignTokens.labelSecondary : DesignTokens.labelTertiary)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(DesignTokens.labelQuaternary)
                    .cornerRadius(8)
            }
        }
        .frame(height: 34)
        .padding(.horizontal, 22)
        .padding(.bottom, 6)
        .background(
            isActive
                ? DesignTokens.windowBg
                : Color.clear
        )
        .cornerRadius(isActive ? 10 : 0)
        .contentShape(Rectangle())
        .onTapGesture {
            Task {
                await appModel.activateProject(tab.id)
            }
        }
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
    ProjectTabsView(appModel: appModel)
        .frame(height: 40)
}
