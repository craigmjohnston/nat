import SwiftUI
import NatKit

/// The one loading indicator every content-loading state in the app uses,
/// in place of an ad-hoc `ProgressView()`: small, quiet, and — the point of
/// it — late. It shows nothing at all until `LoadingDelay` says the wait has
/// gone on long enough to be worth admitting to, which is what keeps a cache
/// hit or a fast read from ever flashing a spinner: by the time 250ms have
/// passed, the view that would have shown this is usually gone already,
/// replaced by the content that loaded.
///
/// Callers mount this exactly where they would have put a `ProgressView()` —
/// conditionally, only while there is genuinely nothing else to show for
/// this content yet (no cached reading of any kind). It fills the space it
/// is given, so a caller wanting it centered over a whole pane frames it
/// `maxWidth: .infinity, maxHeight: .infinity` the same way a plain
/// `ProgressView()` would have been framed.
public struct QuietLoadingView: View {
    private let label: String
    @State private var isRevealed = false

    public init(label: String) {
        self.label = label
    }

    public var body: some View {
        Group {
            if isRevealed {
                VStack(spacing: 6) {
                    ProgressView()
                        .controlSize(.small)
                    Text(label)
                        .font(.system(size: 11))
                        .foregroundStyle(DesignTokens.labelTertiary)
                }
            } else {
                Color.clear
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .task {
            isRevealed = await LoadingDelay().shouldReveal()
        }
    }
}

#Preview {
    QuietLoadingView(label: "Loading…")
        .frame(width: 300, height: 200)
}
