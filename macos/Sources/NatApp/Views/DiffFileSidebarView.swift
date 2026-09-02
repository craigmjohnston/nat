import SwiftUI
import NatKit

/// The file list beside the diff: the "All commits" dropdown, then one row
/// per file — a viewed checkmark, the path truncated from the front so its
/// filename stays visible, an A/M/R change-kind badge, and the ± tally.
/// Clicking a row scrolls the content pane to that file.
///
/// The dropdown is real: titled "All commits" (or, with one selected, that
/// commit's own subject) with a count badge, it lists "All commits" first and
/// then every commit — subject, and its short sha in a monospaced font, per
/// the mock. Picking one is `onSelectCommit`'s to act on; this view holds no
/// opinion about what a selection means to the diff beside it.
struct DiffFileSidebarView: View {
    let files: [DiffFileModel]
    let isViewed: (String) -> Bool
    var commentCount: (String) -> Int = { _ in 0 }
    var commits: [SliceCommit] = []
    var selectedCommit: String?
    var onSelectCommit: (String?) -> Void = { _ in }
    let onSelect: (String) -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 2) {
                allCommitsMenu

                ForEach(files) { file in
                    DiffFileSidebarRow(file: file, isViewed: isViewed(file.path), commentCount: commentCount(file.path))
                        .contentShape(Rectangle())
                        .onTapGesture {
                            onSelect(file.path)
                        }
                }
            }
            .padding(8)
        }
    }

    private var selectedCommitTitle: String {
        guard let selectedCommit, let commit = commits.first(where: { $0.sha == selectedCommit }) else {
            return "All commits"
        }
        return commit.subject
    }

    private var allCommitsMenu: some View {
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
                    .font(.system(size: 12, weight: .regular))
                    .foregroundStyle(DesignTokens.labelSecondary)
                    .lineLimit(1)

                Spacer()

                Text("\(commits.count)")
                    .font(.system(size: 11, weight: .regular, design: .monospaced))
                    .foregroundStyle(DesignTokens.labelTertiary)

                Image(systemName: "chevron.up.chevron.down")
                    .font(.system(size: 8, weight: .bold))
                    .foregroundStyle(DesignTokens.labelTertiary)
            }
            .padding(.horizontal, 8)
            .frame(height: 22)
            .background(DesignTokens.controlFace)
            .clipShape(RoundedRectangle(cornerRadius: 6))
            .overlay(
                RoundedRectangle(cornerRadius: 6)
                    .stroke(DesignTokens.controlBorder, lineWidth: 0.5)
            )
        }
        .menuStyle(.borderlessButton)
        .menuIndicator(.hidden)
        .padding(.bottom, 6)
    }
}

/// A change-kind badge letter: `R` for a rename, `A` for a file with nothing
/// but additions (a new file, typically), `M` otherwise.
private enum ChangeBadge {
    case added, modified, renamed

    init(file: DiffFileModel) {
        if file.isRenamed {
            self = .renamed
        } else if file.dels == 0 && file.adds > 0 {
            self = .added
        } else {
            self = .modified
        }
    }

    var letter: String {
        switch self {
        case .added: return "A"
        case .modified: return "M"
        case .renamed: return "R"
        }
    }

    var color: Color {
        switch self {
        case .added: return DesignTokens.systemGreen
        case .modified, .renamed: return DesignTokens.systemOrange
        }
    }
}

struct DiffFileSidebarRow: View {
    let file: DiffFileModel
    let isViewed: Bool
    var commentCount: Int = 0

    private var badge: ChangeBadge { ChangeBadge(file: file) }

    var body: some View {
        HStack(spacing: 5) {
            if isViewed {
                Image(systemName: "checkmark")
                    .font(.system(size: 9, weight: .bold))
                    .foregroundStyle(DesignTokens.systemGreen)
                    .frame(width: 11)
            } else {
                Color.clear.frame(width: 11)
            }

            // Truncated from the front rather than the tail: what names a
            // file is its end, so the directory is what should give way
            // first when the path does not fit.
            Text(file.path)
                .font(.system(size: 12, weight: .regular))
                .foregroundStyle(DesignTokens.label)
                .lineLimit(1)
                .truncationMode(.head)
                .opacity(isViewed ? 0.55 : 1)

            if commentCount > 0 {
                HStack(spacing: 3) {
                    Image(systemName: "text.bubble")
                        .font(.system(size: 9, weight: .regular))
                    Text("\(commentCount)")
                        .font(.system(size: 10, weight: .regular, design: .monospaced))
                }
                .foregroundStyle(DesignTokens.accent)
            }

            Text(badge.letter)
                .font(.system(size: 8, weight: .semibold))
                .foregroundStyle(badge.color)
                .frame(width: 13, height: 13)
                .background(badge.color.opacity(0.18))
                .clipShape(RoundedRectangle(cornerRadius: 3.5))

            if file.adds > 0 {
                Text("+\(file.adds)")
                    .font(.system(size: 10, weight: .regular, design: .monospaced))
                    .foregroundStyle(DesignTokens.systemGreen)
            }
            if file.dels > 0 {
                Text("\u{2212}\(file.dels)")
                    .font(.system(size: 10, weight: .regular, design: .monospaced))
                    .foregroundStyle(DesignTokens.systemRed)
            }
        }
        .padding(.horizontal, 8)
        .frame(height: 28)
        .clipShape(RoundedRectangle(cornerRadius: 5))
    }
}
