import SwiftUI
import NatKit

/// The one-time card at the rail's foot after a plan is accepted into a local
/// project — `NFNudgeCard` in `docs/design/nat-new-project/ui-npflow.jsx`: the
/// Notion mark and the question, a ✕ that never lets it come back, the line
/// saying the plan stays local either way, and the button that opens the
/// picker. It decides nothing itself; `AppModel.mirrorNudgeShown` is whether it
/// is drawn.
struct MirrorNudgeCardView: View {
    let onChoose: () -> Void
    let onDismiss: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 9) {
                NotionMark(size: 15)
                Text(MirrorText.cardTitle)
                    .font(.system(size: Typo.body, weight: .semibold))
                    .ink(.primary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                Button(action: onDismiss) {
                    Image(systemName: "xmark")
                        .font(.system(size: 9, weight: .semibold))
                        .ink(.tertiary)
                        .frame(width: 18, height: 18)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .help(MirrorText.dismiss)
                .accessibilityLabel(MirrorText.dismiss)
            }
            .padding(.bottom, 4)

            Text(MirrorText.cardBody)
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.tertiary)
                .lineSpacing(3)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.bottom, 10)

            Button(MirrorText.choosePage, action: onChoose)
                .buttonStyle(SecondaryButtonStyle())
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .card(radius: 8)
    }
}
