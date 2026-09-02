import SwiftUI
import NatKit

/// The file list beside the diff: a disabled "All commits" stub (there is
/// only ever one range to show today), then one row per file — a viewed
/// checkmark, the path truncated from the front so its filename stays
/// visible, an A/M/R change-kind badge, and the ± tally. Clicking a row
/// scrolls the content pane to that file.
struct DiffFileSidebarView: View {
    let files: [DiffFileModel]
    let isViewed: (String) -> Bool
    let onSelect: (String) -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 2) {
                allCommitsStub

                ForEach(files) { file in
                    DiffFileSidebarRow(file: file, isViewed: isViewed(file.path))
                        .contentShape(Rectangle())
                        .onTapGesture {
                            onSelect(file.path)
                        }
                }
            }
            .padding(8)
        }
    }

    private var allCommitsStub: some View {
        HStack(spacing: 6) {
            Text("All commits")
                .font(.system(size: 12, weight: .regular))
                .foregroundStyle(DesignTokens.labelSecondary)

            Spacer()

            Text("\(files.count)")
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
        .opacity(0.6)
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
