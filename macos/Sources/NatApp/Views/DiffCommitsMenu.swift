import SwiftUI
import NatKit

/// The diff's commits dropdown: titled "All commits" (or, with one
/// selected, that commit's own subject) with a count badge, listing "All
/// commits" first and then every commit — subject, and its short sha in a
/// monospaced font, per the mock. Picking one is `onSelectCommit`'s to act
/// on; this view holds no opinion about what a selection means to the diff
/// beside it.
///
/// Its own view because it is chrome rather than content — the same control
/// before the branch is read and after, saying "All commits" and nothing of
/// no commits either way — so `DiffSkeletonView` draws this very thing
/// instead of a block that would be replaced by it.
struct DiffCommitsMenu: View {
    var commits: [SliceCommit] = []
    var selectedCommit: String?
    var onSelectCommit: (String?) -> Void = { _ in }
    var bottomPadding: CGFloat = 6

    private var selectedCommitTitle: String {
        guard let selectedCommit, let commit = commits.first(where: { $0.sha == selectedCommit }) else {
            return "All commits"
        }
        return commit.subject
    }

    var body: some View {
        Menu {
            Button {
                onSelectCommit(nil)
            } label: {
                Text("All commits")
            }

            if !commits.isEmpty {
                Divider()
                ForEach(commits) { commit in
                    Button {
                        onSelectCommit(commit.sha)
                    } label: {
                        Text("\(commit.subject)  \(commit.shortSHA)")
                    }
                }
            }
        } label: {
            HStack(spacing: 6) {
                Text(selectedCommitTitle)
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .ink(.secondary)
                    .lineLimit(1)

                Spacer()

                Text("\(commits.count)")
                    .font(.system(size: Typo.subhead, weight: .regular))
                    .monospacedDigit()
                    .ink(.tertiary)

                Image(systemName: "chevron.up.chevron.down")
                    .font(.system(size: 12, weight: .medium))
                    .ink(.tertiary)
            }
            .padding(.horizontal, 8)
            .frame(height: 22)
            .control(radius: 6)
        }
        .menuStyle(.borderlessButton)
        .menuIndicator(.hidden)
        .padding(.bottom, bottomPadding)
    }
}
