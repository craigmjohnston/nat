import SwiftUI
import NatKit

/// One file's diff, drawn GitHub-fashion: a rounded, hairline-bordered box
/// with a header row (chevron, path, ± tally, viewed toggle) and the file's
/// body rows beneath it. A collapsed file is the header row alone, ticked —
/// the fold is this view's own state, held by `DiffStore` and cleared by a
/// re-read, never written anywhere.
struct DiffFileBoxView: View {
    let file: DiffFileModel
    let numberWidth: Int
    let isViewed: Bool
    let isCollapsed: Bool
    let onToggleViewed: () -> Void
    let onToggleCollapsed: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header

            if !isCollapsed {
                Divider().frame(height: 0.5)

                LazyVStack(alignment: .leading, spacing: 0) {
                    ForEach(file.rows) { row in
                        DiffRowView(row: row, numberWidth: numberWidth)
                    }
                }
            }
        }
        .background(DesignTokens.controlBg)
        .clipShape(RoundedRectangle(cornerRadius: 10))
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .stroke(DesignTokens.controlBorder, lineWidth: 0.5)
        )
    }

    private var header: some View {
        HStack(spacing: 10) {
            Image(systemName: isCollapsed ? "chevron.right" : "chevron.down")
                .font(.system(size: 9, weight: .bold))
                .foregroundStyle(DesignTokens.labelTertiary)

            if isViewed {
                Image(systemName: "checkmark")
                    .font(.system(size: 10, weight: .bold))
                    .foregroundStyle(DesignTokens.systemGreen)
            }

            Text(file.path)
                .font(.system(size: 12, weight: .regular, design: .monospaced))
                .foregroundStyle(DesignTokens.label)
                .lineLimit(1)
                .truncationMode(.head)

            if file.isRenamed {
                Text("was \(file.oldPath)")
                    .font(.system(size: 11, weight: .regular, design: .monospaced))
                    .foregroundStyle(DesignTokens.labelTertiary)
                    .lineLimit(1)
            }

            Spacer()

            if file.adds > 0 {
                Text("+\(file.adds)")
                    .font(.system(size: 11, weight: .regular, design: .monospaced))
                    .foregroundStyle(DesignTokens.systemGreen)
            }
            if file.dels > 0 {
                Text("\u{2212}\(file.dels)")
                    .font(.system(size: 11, weight: .regular, design: .monospaced))
                    .foregroundStyle(DesignTokens.systemRed)
            }

            Button(action: onToggleViewed) {
                Text("Viewed")
                    .font(.system(size: 11, weight: .regular))
            }
            .buttonStyle(.borderless)
        }
        .padding(.horizontal, 12)
        .frame(height: 32)
        .background(DesignTokens.rowAltBg)
        .contentShape(Rectangle())
        .onTapGesture {
            onToggleCollapsed()
        }
    }
}

/// One row of a file's body: the gutter (old/new line numbers), the +/-
/// glyph, and the row's text. `hunkBreak` draws a dashed separator with the
/// hunk header in tertiary instead. Plain text for now — syntax highlighting
/// is a later task.
struct DiffRowView: View {
    let row: DiffRow
    let numberWidth: Int

    private var numberColumnWidth: CGFloat {
        CGFloat(numberWidth) * 7.5 + 4
    }

    var body: some View {
        switch row.kind {
        case .hunkBreak:
            hunkBreakRow
        default:
            contentRow
        }
    }

    private var hunkBreakRow: some View {
        HStack(spacing: 8) {
            Rectangle()
                .fill(DesignTokens.labelQuaternary)
                .frame(height: 0.5)
                .overlay(dashedLine)
                .frame(width: numberColumnWidth * 2 + 20)

            Text(row.text)
                .font(.system(size: 12, weight: .regular, design: .monospaced))
                .foregroundStyle(DesignTokens.labelTertiary)
                .lineLimit(1)

            Spacer(minLength: 0)
        }
        .frame(minHeight: 24)
        .background(DesignTokens.accent.opacity(0.1))
    }

    private var dashedLine: some View {
        Rectangle()
            .fill(DesignTokens.accent)
            .frame(height: 0.5)
    }

    private var contentRow: some View {
        HStack(spacing: 0) {
            HStack(spacing: 6) {
                Text(row.oldNumber.map(String.init) ?? "")
                    .frame(width: numberColumnWidth, alignment: .trailing)
                Text(row.newNumber.map(String.init) ?? "")
                    .frame(width: numberColumnWidth, alignment: .trailing)
            }
            .font(.system(size: 11, weight: .regular, design: .monospaced))
            .foregroundStyle(DesignTokens.labelTertiary)
            .padding(.horizontal, 8)
            .frame(minHeight: 19)
            .background(gutterFill)

            Text(glyph)
                .font(.system(size: 12, weight: .regular, design: .monospaced))
                .foregroundStyle(glyphColor)
                .frame(width: 12)
                .padding(.leading, 4)

            Text(row.text.isEmpty ? " " : row.text)
                .font(.system(size: 12, weight: .regular, design: .monospaced))
                .foregroundStyle(row.kind == .described ? DesignTokens.labelSecondary : DesignTokens.label)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)

            Spacer(minLength: 0)
        }
        .frame(minHeight: 19)
        .background(rowFill)
    }

    private var glyph: String {
        switch row.prefix {
        case "+": return "+"
        case "-": return "\u{2212}"
        default: return " "
        }
    }

    private var glyphColor: Color {
        switch row.kind {
        case .added: return DesignTokens.systemGreen
        case .removed: return DesignTokens.systemRed
        default: return .clear
        }
    }

    private var rowFill: Color {
        switch row.kind {
        case .added: return DesignTokens.systemGreen.opacity(0.2)
        case .removed: return DesignTokens.systemRed.opacity(0.2)
        default: return .clear
        }
    }

    private var gutterFill: Color {
        switch row.kind {
        case .added: return DesignTokens.systemGreen.opacity(0.32)
        case .removed: return DesignTokens.systemRed.opacity(0.32)
        default: return DesignTokens.rowAltBg
        }
    }
}
