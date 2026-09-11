import XCTest
@testable import NatKit

/// The face's one rule, held over the source the way `ColorSourcesTests`
/// holds the palette's: no view names a monospaced font of its own. It asks
/// `Typo.mono`, which is where the app's face — and the fallback for a Mac
/// that cannot have it — is decided.
///
/// A source scan for the same reason the colour one is: `design: .monospaced`
/// is not a crash and not a wrong value, it is SF Mono on this machine and
/// something else on the next OS release, and nothing a rendered assertion
/// could tell apart from the face this app ships. What tells them apart is
/// where the font was named.
///
/// `.monospacedDigit()` is deliberately allowed: it is a proportional label
/// asking for lining digits so a count does not jitter as it changes, which
/// is a different thing from monospaced text and has nothing to do with the
/// code face.
final class MonoSourcesTests: XCTestCase {
    /// The app's sources, found from this file rather than from the working
    /// directory, which `swift test` does not promise anything about.
    private var sourcesDirectory: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent("Sources")
    }

    /// Where the monospaced system font may be named: the fallback inside
    /// `Typo.mono` itself, which is the whole point of there being one place.
    private let mayNameTheSystemFace = ["Theme/DesignTokens.swift"]

    private let rules: [(String, String)] = [
        ("a monospaced system font at a call site", #"design:\s*\.monospaced\b"#),
        ("AppKit's monospaced system font at a call site", #"monospacedSystemFont\("#),
        ("the `.monospaced()` modifier, which is the system's face again", #"\.monospaced\(\)"#),
    ]

    func testNoViewNamesAMonospacedFontOfItsOwn() throws {
        let files = try swiftFiles()
        XCTAssertFalse(files.isEmpty, "no sources found at \(sourcesDirectory.path)")
        var strays: [String] = []
        for file in files {
            let relative = file.path.replacingOccurrences(of: sourcesDirectory.path + "/", with: "")
            if mayNameTheSystemFace.contains(where: relative.hasSuffix) { continue }
            let contents = try String(contentsOf: file, encoding: .utf8)
            for (line, text) in contents.components(separatedBy: .newlines).enumerated() {
                // Prose about the rule is not a breach of it.
                if text.trimmingCharacters(in: .whitespaces).hasPrefix("//") { continue }
                for (what, pattern) in rules where text.range(of: pattern, options: .regularExpression) != nil {
                    strays.append("\(relative):\(line + 1): \(what) — \(text.trimmingCharacters(in: .whitespaces))")
                }
            }
        }
        XCTAssertEqual(
            strays, [],
            """
            Every monospaced thing the app draws is set in `Typo.mono`, which \
            is the app's own face with the system's as its fallback. Route \
            each of these through it, at the size it already has:
            \(strays.joined(separator: "\n"))
            """
        )
    }

    /// Every text input is set in that face too — the rule the brief pane,
    /// the comment box and the settings fields all follow, and the one a new
    /// input is likeliest to be written without.
    func testEveryTextInputIsSetInTheAppsFace() throws {
        var strays: [String] = []
        for file in try swiftFiles() {
            let relative = file.path.replacingOccurrences(of: sourcesDirectory.path + "/", with: "")
            let lines = try String(contentsOf: file, encoding: .utf8).components(separatedBy: .newlines)
            for (line, text) in lines.enumerated() {
                guard text.range(of: #"\b(TextField|TextEditor)\("#, options: .regularExpression) != nil,
                      // The declaration of a wrapper around one is not a use.
                      text.range(of: #"\bstruct\b"#, options: .regularExpression) == nil else { continue }
                // The font goes on the input or on one of the modifiers
                // chained to it, which is the handful of lines that follow.
                let chain = lines[line..<min(line + 8, lines.count)].joined(separator: "\n")
                if chain.contains("Typo.mono(") { continue }
                strays.append("\(relative):\(line + 1): \(text.trimmingCharacters(in: .whitespaces))")
            }
        }
        XCTAssertEqual(
            strays, [],
            """
            Every input in the app is typed in `Typo.mono`. Give each of \
            these the face at the size it already draws in:
            \(strays.joined(separator: "\n"))
            """
        )
    }

    /// The scan is worth nothing if it is not reading the files it thinks it
    /// is — the same guard `ColorSourcesTests` keeps, for the same reason.
    func testTheScanReadsTheAppsOwnSources() throws {
        let relatives = try swiftFiles().map {
            $0.path.replacingOccurrences(of: sourcesDirectory.path + "/", with: "")
        }
        XCTAssertTrue(relatives.contains("NatKit/Theme/MonoFont.swift"), "\(relatives.count) files found")
        XCTAssertTrue(relatives.contains("NatApp/Views/DiffFileBoxView.swift"), "\(relatives.count) files found")
    }

    private func swiftFiles() throws -> [URL] {
        guard let walk = FileManager.default.enumerator(at: sourcesDirectory, includingPropertiesForKeys: nil) else {
            throw XCTSkip("no sources at \(sourcesDirectory.path)")
        }
        return walk.compactMap { $0 as? URL }.filter { $0.pathExtension == "swift" }.sorted { $0.path < $1.path }
    }
}
