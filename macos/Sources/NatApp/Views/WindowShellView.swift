import SwiftUI
import NatKit

struct WindowShellView: View {
    @Bindable var appModel: AppModel

    var body: some View {
        ZStack {
            DesignTokens.windowBg
                .ignoresSafeArea()

            VStack(spacing: 0) {
                // Header
                VStack(spacing: 0) {
                    HStack(spacing: 12) {
                        // Traffic lights - left-aligned (handled by system)
                        Spacer()
                            .frame(maxWidth: .infinity, alignment: .leading)

                        // Project tab (placeholder for single project)
                        if let config = appModel.config,
                           let projectName = config.projects
                            .sorted(by: { $0.key < $1.key })
                            .first?.value.name {
                            Text(projectName)
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundStyle(DesignTokens.label)
                                .padding(.horizontal, 12)
                                .padding(.vertical, 6)
                                .background(DesignTokens.controlBg)
                                .cornerRadius(8)
                        }

                        Spacer()
                            .frame(maxWidth: .infinity, alignment: .trailing)

                        // Right-side toolbar
                        HStack(spacing: 12) {
                            // Slice count
                            if let projectInfo = appModel.projectStore?.state.projectInfo {
                                Text("\(projectInfo.slices.filter { $0.status != "Done" }.count)/\(projectInfo.slices.count) slices")
                                    .font(.system(size: 11, weight: .regular, design: .monospaced))
                                    .foregroundStyle(DesignTokens.labelTertiary)
                            }

                            // Toolbar icons (disabled for now)
                            Image(systemName: "plus.rectangle.on.rectangle")
                                .font(.system(size: 12, weight: .regular))
                                .foregroundStyle(DesignTokens.labelTertiary)
                                .opacity(0.5)

                            Image(systemName: "wand.and.stars")
                                .font(.system(size: 12, weight: .regular))
                                .foregroundStyle(DesignTokens.labelTertiary)
                                .opacity(0.5)
                        }
                    }
                    .frame(height: 40)
                    .padding(.horizontal, 16)
                    .background(
                        Color(hex: "1e1e23").opacity(0.85)
                            .blur(radius: 8)
                    )
                }
                .frame(height: 40)

                // Main content: Rail | Pane
                HStack(spacing: 0) {
                    RailView(appModel: appModel)
                        .frame(maxWidth: 372)

                    PaneView(appModel: appModel)
                        .frame(maxWidth: .infinity)
                }
                .frame(maxHeight: .infinity)

                // Progress border
                ProgressBorderView(appModel: appModel)
                    .frame(height: 7)
            }
        }
        .task {
            await appModel.start()
        }
    }
}

#Preview {
    WindowShellView(appModel: AppModel())
        .frame(width: 1360, height: 840)
}
