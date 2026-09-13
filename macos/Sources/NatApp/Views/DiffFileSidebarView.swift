import SwiftUI
import NatKit

/// The file list beside the diff: the "All commits" dropdown, then one row
/// per file — a viewed checkmark, the path truncated from the front so its
/// filename stays visible, an A/M/R change-kind badge, and the ± tally.
/// Clicking a row scrolls the content pane to that file. The dropdown is
/// `DiffCommitsMenu`, and real.
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
                DiffCommitsMenu(
                    commits: commits,
                    selectedCommit: selectedCommit,
                    onSelectCommit: onSelectCommit
                )

                ForEach(files) { file in
                    DiffFileSidebarRow(file: file, isViewed: isViewed(file.path), commentCount: commentCount(file.path))
                        .hoverWash(cornerRadius: 5)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            onSelect(file.path)
                        }
                }
            }
            .padding(8)
            .inelastic()
        }
    }
}

/// The file list's own commits dropdown: titled "All commits" (or, with one
/// selected, that commit's own subject) with a count badge, listing "All
/// commits" first and then every commit — subject, and its short sha in a
/// monospaced font, per the mock. Picking one is `onSelectCommit`'s to act
/// on; this view holds no opinion about what a selection means to the diff
/// beside it.
///
/// Its own view rather than a piece of `DiffFileSidebarView`, because it is
/// chrome rather than content — the same control before the branch is read
/// and after, saying "All commits" and nothing of no commits either way — so
/// `DiffSkeletonView` draws this very thing instead of a block that would be
/// replaced by it.
struct DiffCommitsMenu: View {
    var commits: [SliceCommit] = []
    var selectedCommit: String?
    var onSelectCommit: (String?) -> Void = { _ in }

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
        case .added: return DesignTokens.chipInk(.green, on: .window)
        case .modified, .renamed: return DesignTokens.chipInk(.orange, on: .window)
        }
    }

    /// The letter's own colour behind it. Paired with `color` here rather
    /// than derived at the call site, so the badge cannot be drawn in one
    /// colour over a wash of another — or over a wash at some number the
    /// theme never chose.
    var wash: Color {
        switch self {
        case .added: return DesignTokens.systemGreenWash(on: .window)
        case .modified, .renamed: return DesignTokens.systemOrangeWash(on: .window)
        }
    }
}

struct DiffFileSidebarRow: View {
    /// The row's own geometry, named rather than inline so `DiffSkeletonView`
    /// reserves exactly the rows this draws: the tick's slot, which is held
    /// whether the file has been viewed or not, the gap after it, and the
    /// height of the row itself.
    static let tickWidth: CGFloat = 13
    static let spacing: CGFloat = 5
    static let height: CGFloat = 28

    let file: DiffFileModel
    let isViewed: Bool
    var commentCount: Int = 0

    private var badge: ChangeBadge { ChangeBadge(file: file) }

    var body: some View {
        HStack(spacing: Self.spacing) {
            if isViewed {
                Image(systemName: "checkmark")
                    .font(.system(size: 12, weight: .medium))
                    .ink(.success)
                    .frame(width: Self.tickWidth)
            } else {
                Color.clear.frame(width: Self.tickWidth)
            }

            // Truncated from the front rather than the tail: what names a
            // file is its end, so the directory is what should give way
            // first when the path does not fit.
            Text(file.path)
                .font(.system(size: Typo.subhead, weight: .regular))
                .ink(.primary)
                .lineLimit(1)
                .truncationMode(.head)
                .opacity(isViewed ? 0.55 : 1)

            if commentCount > 0 {
                HStack(spacing: 3) {
                    Image(systemName: "text.bubble")
                        .font(.system(size: 12, weight: .medium))
                    Text("\(commentCount)")
                        .font(.system(size: Typo.caption, weight: .regular))
                        .monospacedDigit()
                }
                .ink(.accent)
            }

            Text(badge.letter)
                .font(.system(size: 12, weight: .medium))
                .foregroundStyle(badge.color)
                .frame(width: 16, height: 16)
                .background(badge.wash)
                .clipShape(RoundedRectangle(cornerRadius: 4))

            if file.adds > 0 {
                Text("+\(file.adds)")
                    .font(.system(size: Typo.caption, weight: .regular))
                    .monospacedDigit()
                    .ink(.success)
            }
            if file.dels > 0 {
                Text("\u{2212}\(file.dels)")
                    .font(.system(size: Typo.caption, weight: .regular))
                    .monospacedDigit()
                    .ink(.danger)
            }
        }
        .padding(.horizontal, 8)
        .frame(height: Self.height)
        .clipShape(RoundedRectangle(cornerRadius: 5))
    }
}
