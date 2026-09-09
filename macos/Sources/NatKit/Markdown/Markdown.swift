import SwiftUI

/// Markdown rendered the way `Text` can actually draw it, GitHub-comment
/// fashion. `AttributedString`'s full-syntax parser reads block structure into
/// `PresentationIntent`s and strips the newlines it was written with — and
/// `Text` ignores the intents, so paragraphs, lists and code fences all run
/// together on one line. It also folds a single newline into a space, where
/// GitHub's own comment rendering keeps it as a line break. So the block
/// structure is read here, line by line, and only the inline syntax — bold,
/// italics, code spans, links — is handed to the parser, one line at a time:
/// every newline the author typed survives, headings and code get their own
/// type, and a line that is just prose comes through exactly as written.
///
/// `size` is the base body size the surrounding `Text` draws in; runs that
/// carry no font of their own take it from the view, so only headings and
/// code set one here.
public func markdownAttributed(_ text: String, size: CGFloat) -> AttributedString {
    var out = AttributedString()
    var inFence = false
    var fenceMarker = ""
    var first = true

    for line in text.split(separator: "\n", omittingEmptySubsequences: false) {
        let line = String(line)
        let trimmed = line.trimmingCharacters(in: .whitespaces)

        if inFence {
            if trimmed.hasPrefix(fenceMarker) {
                inFence = false
                continue
            }
            append(codeLine(line, size: size), to: &out, first: &first)
            continue
        }
        if trimmed.hasPrefix("```") || trimmed.hasPrefix("~~~") {
            inFence = true
            fenceMarker = String(trimmed.prefix(3))
            continue
        }

        append(blockLine(line, size: size), to: &out, first: &first)
    }
    return out
}

/// Every rendered line joins the output on a newline of its own — the very
/// characters this renderer exists to keep.
private func append(_ line: AttributedString, to out: inout AttributedString, first: inout Bool) {
    if !first {
        out += AttributedString("\n")
    }
    first = false
    out += line
}

private func codeLine(_ line: String, size: CGFloat) -> AttributedString {
    var content = AttributedString(line)
    content.font = .system(size: size - 1, weight: .regular, design: .monospaced)
    return content
}

/// One line outside any fence: a heading gets its level's type, a bullet
/// marker becomes the bullet it stands for, and everything else — the
/// indentation and an ordered item's own number included — is kept as
/// written, with only the inline syntax parsed.
private func blockLine(_ line: String, size: CGFloat) -> AttributedString {
    if let heading = headingLine(line) {
        var content = inline(heading.text)
        content.font = .system(size: size + headingBump(level: heading.level), weight: .semibold)
        return content
    }
    if let item = bulletLine(line) {
        return AttributedString(item.indent + "• ") + inline(item.text)
    }
    return inline(line)
}

private func headingLine(_ line: String) -> (level: Int, text: String)? {
    let trimmed = line.trimmingCharacters(in: .whitespaces)
    let hashes = trimmed.prefix(while: { $0 == "#" })
    guard (1...6).contains(hashes.count) else { return nil }
    let rest = trimmed.dropFirst(hashes.count)
    guard rest.first == " " else { return nil }
    return (hashes.count, rest.trimmingCharacters(in: .whitespaces))
}

private func headingBump(level: Int) -> CGFloat {
    switch level {
    case 1: return 4
    case 2: return 2
    default: return 1
    }
}

private func bulletLine(_ line: String) -> (indent: String, text: String)? {
    let indent = line.prefix(while: { $0 == " " || $0 == "\t" })
    let rest = line.dropFirst(indent.count)
    guard let marker = rest.first, "-*+".contains(marker), rest.dropFirst().first == " " else { return nil }
    return (String(indent), String(rest.dropFirst(2)))
}

/// The parser is trusted with inline syntax alone, where it has no newlines
/// to lose; a line it refuses is shown as written rather than dropped.
private func inline(_ text: String) -> AttributedString {
    (try? AttributedString(
        markdown: text,
        options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace)
    )) ?? AttributedString(text)
}
