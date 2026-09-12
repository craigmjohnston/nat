import Foundation
import NatKit

/// One line of a canned diff, written as its pieces so the text and the
/// syntax runs can never disagree: a run's `length` is the byte count of the
/// very substring it names, rather than a number typed out beside it and
/// left to rot when the line is edited.
struct FixtureLine {
    /// The line's leading `+`, `-` or space, exactly as git writes it — nil
    /// for a line that carries none: git's own file headers and the hunk
    /// headers, which are written verbatim.
    let mark: Character?
    let pieces: [(TokenKind, String)]

    init(_ mark: Character, _ pieces: [(TokenKind, String)]) {
        self.mark = mark
        self.pieces = pieces
    }

    private init(raw: String) {
        self.mark = nil
        self.pieces = [(.text, raw)]
    }

    /// A line git wrote about the file rather than in it — a header, or a
    /// hunk header — carried verbatim, since that is what the parser matches
    /// on.
    static func raw(_ line: String) -> FixtureLine { FixtureLine(raw: line) }

    /// The line as git wrote it: the mark and every piece run together.
    var text: String {
        (mark.map(String.init) ?? "") + pieces.map(\.1).joined()
    }

    /// The line's content lexed into runs — what `SliceDiffFile.tokens`
    /// carries for it, measured in the bytes the Go side measures in.
    var runs: [TokenRun] {
        pieces.map { TokenRun(kind: $0.0, length: $0.1.utf8.count) }
    }
}

extension Array where Element == FixtureLine {
    var texts: [String] { map(\.text) }
    var tokens: [[TokenRun]] { map(\.runs) }
}

extension Fixtures {
    /// The branch every diff fixture is of, and the base it was measured
    /// against.
    public static let diffBranch = "slice/draw-the-merge-box-on-the-pr-tab"
    public static let diffBase = "origin/main"

    // MARK: - The files

    /// A Swift file with two hunks, so the diff has a hunk break in it.
    static let mergeBoxLines: [FixtureLine] = [
        FixtureLine.raw("diff --git a/Sources/NatApp/Views/PRTabView.swift b/Sources/NatApp/Views/PRTabView.swift"),
        FixtureLine.raw("index 3a1f2b4c..9c0d5e6f 100644"),
        FixtureLine.raw("--- a/Sources/NatApp/Views/PRTabView.swift"),
        FixtureLine.raw("+++ b/Sources/NatApp/Views/PRTabView.swift"),
        FixtureLine.raw("@@ -18,6 +18,8 @@ struct PRTabView: View {"),
        FixtureLine(" ", [(.keyword, "    let"), (.text, " pr"), (.text, ": "), (.name, "PRDetail")]),
        FixtureLine("+", [(.keyword, "    let"), (.text, " verdicts"), (.text, ": ["), (.name, "MergeVerdict"), (.text, "]")]),
        FixtureLine("+", [(.keyword, "    let"), (.text, " heading"), (.text, ": "), (.name, "MergeHeading")]),
        FixtureLine(" ", [(.text, "")]),
        FixtureLine(" ", [(.keyword, "    var"), (.text, " body"), (.text, ": "), (.keyword, "some"), (.text, " "), (.name, "View"), (.text, " {")]),
        FixtureLine.raw("@@ -44,9 +46,12 @@ struct PRTabView: View {"),
        FixtureLine(" ", [(.text, "            "), (.name, "PRConversationView"), (.text, "(entries: entries)")]),
        FixtureLine("-", [(.comment, "            // The merge box is still to come.")]),
        FixtureLine("-", [(.name, "            EmptyView"), (.text, "()")]),
        FixtureLine("+", [(.name, "            MergeBoxView"), (.text, "(")]),
        FixtureLine("+", [(.text, "                heading: heading,")]),
        FixtureLine("+", [(.text, "                verdicts: verdicts")]),
        FixtureLine("+", [(.text, "            )")]),
        FixtureLine(" ", [(.text, "        }")]),
        FixtureLine(" ", [(.text, "    }")]),
    ]

    /// A second Swift file, added whole — every line of it new.
    static let mergeBoxViewLines: [FixtureLine] = [
        FixtureLine.raw("diff --git a/Sources/NatApp/Views/MergeBoxView.swift b/Sources/NatApp/Views/MergeBoxView.swift"),
        FixtureLine.raw("new file mode 100644"),
        FixtureLine.raw("index 00000000..7f1a2b3c"),
        FixtureLine.raw("--- /dev/null"),
        FixtureLine.raw("+++ b/Sources/NatApp/Views/MergeBoxView.swift"),
        FixtureLine.raw("@@ -0,0 +1,9 @@"),
        FixtureLine("+", [(.keyword, "import"), (.text, " "), (.name, "SwiftUI")]),
        FixtureLine("+", [(.text, "")]),
        FixtureLine("+", [(.comment, "/// The three verdicts a merge is weighed on.")]),
        FixtureLine("+", [(.keyword, "struct"), (.text, " "), (.name, "MergeBoxView"), (.text, ": "), (.name, "View"), (.text, " {")]),
        FixtureLine("+", [(.keyword, "    let"), (.text, " heading"), (.text, ": "), (.name, "MergeHeading")]),
        FixtureLine("+", [(.keyword, "    let"), (.text, " verdicts"), (.text, ": ["), (.name, "MergeVerdict"), (.text, "]")]),
        FixtureLine("+", [(.text, "")]),
        FixtureLine("+", [(.keyword, "    var"), (.text, " body"), (.text, ": "), (.keyword, "some"), (.text, " "), (.name, "View"), (.text, " { "), (.name, "EmptyView"), (.text, "() }")]),
        FixtureLine("+", [(.text, "}")]),
    ]

    /// A Go file, so the diff spans more than one language.
    static let mergeRefusalLines: [FixtureLine] = [
        FixtureLine.raw("diff --git a/internal/actions/mergerefusal.go b/internal/actions/mergerefusal.go"),
        FixtureLine.raw("index 5d6e7f80..1a2b3c4d 100644"),
        FixtureLine.raw("--- a/internal/actions/mergerefusal.go"),
        FixtureLine.raw("+++ b/internal/actions/mergerefusal.go"),
        FixtureLine.raw("@@ -7,7 +7,7 @@ package actions"),
        FixtureLine(" ", [(.comment, "// MergeRefusal is why the merge would be refused.")]),
        FixtureLine(" ", [(.keyword, "func"), (.text, " "), (.name, "MergeRefusal"), (.text, "(pr "), (.name, "gh.PR"), (.text, ") "), (.keyword, "string"), (.text, " {")]),
        FixtureLine("-", [(.keyword, "\tfor"), (.text, " _, v "), (.text, ":= "), (.keyword, "range"), (.text, " "), (.name, "Verdicts"), (.text, "(pr) {")]),
        FixtureLine("+", [(.keyword, "\tfor"), (.text, " _, v "), (.text, ":= "), (.keyword, "range"), (.text, " "), (.name, "MergeVerdicts"), (.text, "(pr) {")]),
        FixtureLine(" ", [(.keyword, "\t\tif"), (.text, " v."), (.name, "Outcome"), (.text, " == "), (.name, "Failing"), (.text, " {")]),
        FixtureLine(" ", [(.keyword, "\t\t\treturn"), (.text, " v."), (.name, "Label")]),
        FixtureLine(" ", [(.text, "\t\t}")]),
        FixtureLine(" ", [(.text, "\t}")]),
    ]

    /// A file git described rather than diffed — the shape a viewer has to
    /// draw without any lines to line up.
    static let iconLines = [
        "diff --git a/macos/Sources/NatApp/Resources/AppIcon.icns b/macos/Sources/NatApp/Resources/AppIcon.icns",
        "Binary files a/macos/Sources/NatApp/Resources/AppIcon.icns and b/macos/Sources/NatApp/Resources/AppIcon.icns differ",
    ]

    /// The wire shape `nat slice-diff --json` hands back for the handed-back
    /// branch: four files, two languages, an added file, a deleted-context
    /// hunk break and one file git only described.
    public static let sliceDiff = SliceDiff(
        base: diffBase,
        branch: diffBranch,
        files: [
            SliceDiffFile(
                path: "Sources/NatApp/Views/PRTabView.swift",
                oldPath: "Sources/NatApp/Views/PRTabView.swift",
                adds: 6,
                dels: 2,
                described: false,
                lines: mergeBoxLines.texts,
                language: "Swift",
                tokens: mergeBoxLines.tokens
            ),
            SliceDiffFile(
                path: "Sources/NatApp/Views/MergeBoxView.swift",
                oldPath: "Sources/NatApp/Views/MergeBoxView.swift",
                adds: 9,
                dels: 0,
                described: false,
                lines: mergeBoxViewLines.texts,
                language: "Swift",
                tokens: mergeBoxViewLines.tokens
            ),
            SliceDiffFile(
                path: "internal/actions/mergerefusal.go",
                oldPath: "internal/actions/mergerefusal.go",
                adds: 1,
                dels: 1,
                described: false,
                lines: mergeRefusalLines.texts,
                language: "Go",
                tokens: mergeRefusalLines.tokens
            ),
            SliceDiffFile(
                path: "macos/Sources/NatApp/Resources/AppIcon.icns",
                oldPath: "macos/Sources/NatApp/Resources/AppIcon.icns",
                adds: 0,
                dels: 0,
                described: true,
                lines: iconLines
            ),
        ]
    )

    /// A one-file diff, for a view that wants a small change rather than a
    /// realistic one.
    public static let smallSliceDiff = SliceDiff(
        base: diffBase,
        branch: diffBranch,
        files: [sliceDiff.files[2]]
    )

    /// The render-ready diff, built through `buildDiffModel` itself so the
    /// fixture is exactly what the app would have parsed.
    public static var diffModel: DiffModel { buildDiffModel(from: sliceDiff) }

    public static var smallDiffModel: DiffModel { buildDiffModel(from: smallSliceDiff) }

    /// A branch whose diff came back empty — nothing pushed to it yet.
    public static var emptyDiffModel: DiffModel {
        buildDiffModel(from: SliceDiff(base: diffBase, branch: diffBranch, files: []))
    }

    // MARK: - The review left on it

    /// Two pending comments, anchored onto rows of `diffModel` — one on a
    /// single line, one on a run of three — so a view drawing a review has
    /// both shapes to draw.
    public static var pendingComments: [PendingComment] {
        let files = diffModel.files
        let prTab = files[0].rows
        let mergeBox = files[1].rows

        let single = prTab
            .filter { $0.kind == .added }
            .prefix(1)
            .map(\.id)
        let run = mergeBox
            .filter { $0.kind == .added }
            .dropFirst(3)
            .prefix(3)
            .map(\.id)

        return [
            PendingComment(
                id: UUID(uuidString: "F1000000-0000-4000-8000-000000000001")!,
                path: files[0].path,
                anchorRowIDs: Array(single),
                text: "These two want to be one value — the heading is read off the verdicts anyway."
            ),
            PendingComment(
                id: UUID(uuidString: "F1000000-0000-4000-8000-000000000002")!,
                path: files[1].path,
                anchorRowIDs: Array(run),
                text: "Worth a doc comment saying which of the three verdicts wins the colour."
            ),
        ]
    }

    /// The branch's own commits, as the "All commits" dropdown lists them.
    public static let commits: [SliceCommit] = [
        SliceCommit(
            sha: "9f2c1ab6c0de4b118a7f0b7d2c5e6f8a9b0c1d2e",
            subject: "Draw the merge box on the PR tab",
            author: "Craig Johnston",
            date: minutesAgo(220)
        ),
        SliceCommit(
            sha: "3b7d8e5f1a2c4d6e8f0a1b2c3d4e5f6a7b8c9d0e",
            subject: "Read the refusal off the verdicts the box draws",
            author: "Craig Johnston",
            date: minutesAgo(95)
        ),
        SliceCommit(
            sha: "c41a2b3c4d5e6f708192a3b4c5d6e7f809a1b2c3",
            subject: "Cover the unknown mergeability case",
            author: "Craig Johnston",
            date: minutesAgo(41)
        ),
    ]

    public static var commitsDoc: SliceCommitsDoc {
        SliceCommitsDoc(base: diffBase, branch: diffBranch, commits: commits)
    }

    // MARK: - Load states

    public static let diffStateIdle: DiffLoadState = .idle
    public static let diffStateLoading: DiffLoadState = .loading
    public static var diffStateLoaded: DiffLoadState { .loaded(diffModel) }
    public static var diffStateEmpty: DiffLoadState { .loaded(emptyDiffModel) }
    /// A read that failed with nothing behind it — the only case with an
    /// empty pane to show.
    public static let diffStateFailed: DiffLoadState = .failed(diffErrorMessage, previous: nil)
    /// A read that failed over a diff already on screen, which is what stays
    /// drawn under the error.
    public static var diffStateStale: DiffLoadState {
        .failed(diffErrorMessage, previous: diffModel)
    }

    public static let diffErrorMessage =
        "nat slice-diff: git: fatal: ambiguous argument 'origin/main': unknown revision"
}
