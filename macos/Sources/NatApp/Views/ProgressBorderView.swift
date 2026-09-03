import SwiftUI
import NatKit

struct ProgressBorderView: View {
    @Bindable var appModel: AppModel

    var segments: [ProgressSegment] {
        if let projectInfo = appModel.projectStore?.state.projectInfo {
            return buildProgressSegments(from: projectInfo)
        }
        return []
    }

    var body: some View {
        HStack(spacing: 4) {
            ForEach(Array(segments.enumerated()), id: \.offset) { index, segment in
                segmentView(for: segment)
                    .help(segment.title)
            }
        }
        .frame(height: 7)
        .padding(.horizontal, 20)
        .padding(.vertical, 6)
        .background(DesignTokens.windowBg)
        .rectBorder(width: 0.5, edges: [.top], color: DesignTokens.separator)
    }

    private func segmentView(for segment: ProgressSegment) -> some View {
        ZStack(alignment: .leading) {
            // Background
            RoundedRectangle(cornerRadius: 3.5)
                .fill(DesignTokens.labelQuaternary)

            // Progress fill
            GeometryReader { geometry in
                RoundedRectangle(cornerRadius: 3.5)
                    .fill(
                        segment.isComplete
                            ? DesignTokens.systemGreen
                            : DesignTokens.accent
                    )
                    .frame(width: geometry.size.width * segment.fraction)
            }
        }
        .frame(maxWidth: .infinity)
    }
}

#Preview {
    let appModel = AppModel()
    ProgressBorderView(appModel: appModel)
        .frame(height: 20)
}
