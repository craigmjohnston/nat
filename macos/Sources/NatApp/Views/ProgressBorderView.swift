import SwiftUI
import NatKit

struct ProgressBorderView: View {
    @Bindable var appModel: AppModel

    var segments: [ProgressSegment] {
        if let projectInfo = appModel.projectStore?.state.projectInfo {
            // The PR-readiness reading keeps a Done slice whose pull request
            // is still open from counting as progress — merged is done, the
            // same rule the rail's NEEDS REVIEW section rides.
            return buildProgressSegments(
                from: projectInfo,
                openPRSliceIDs: Set((appModel.reviewStatsStore?.prReadiness ?? [:]).keys)
            )
        }
        return []
    }

    private static let segmentSpacing: CGFloat = 4

    var body: some View {
        // Widths are shared out by each segment's weight — its slice count —
        // rather than equally, so a ten-slice milestone reads as ten slices'
        // worth of bar and the combined Done segment as everything finished.
        GeometryReader { geometry in
            let segments = self.segments
            let totalWeight = max(1, segments.reduce(0) { $0 + $1.weight })
            let available = max(0, geometry.size.width - Self.segmentSpacing * CGFloat(max(0, segments.count - 1)))
            HStack(spacing: Self.segmentSpacing) {
                ForEach(Array(segments.enumerated()), id: \.offset) { index, segment in
                    segmentView(for: segment)
                        .frame(width: available * CGFloat(segment.weight) / CGFloat(totalWeight))
                        .help(segment.title)
                }
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

            // Progress fill — one hue for the whole bar, with brightness
            // saying what's finished: the folded Done run sits back at a
            // muted accent so a mostly-finished plan doesn't shout, and the
            // full accent is saved for the milestones still moving, which is
            // where the eye should land.
            GeometryReader { geometry in
                RoundedRectangle(cornerRadius: 3.5)
                    .fill(
                        segment.isComplete
                            ? DesignTokens.accentMuted
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
