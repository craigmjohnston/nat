import XCTest
@testable import NatKit

/// The theme's one rule, held over the source rather than over a rendered
/// window: nothing in the app constructs a colour. A view asks
/// `DesignTokens` for one by the role it plays, and the mapping from a role
/// to a Catppuccin swatch lives in exactly two files.
///
/// It is a source scan because that is the only place the rule can be
/// checked. A stray colour is not a crash and not a wrong value — it is a
/// value that is *right today* and drifts tomorrow, when macOS changes what
/// `controlBackgroundColor` means or when somebody presses the same tint at
/// a slightly different number two files away. Nothing a unit test can
/// resolve would tell the two apart; what tells them apart is where the
/// number was written.
///
/// `Color.clear` is deliberately allowed throughout: it is the absence of
/// paint rather than a colour, and the theme has nothing to say about it.
final class ColorSourcesTests: XCTestCase {
    /// The app's sources, found from this file rather than from the working
    /// directory, which `swift test` does not promise anything about.
    private var sourcesDirectory: URL {
        URL(fileURLWithPath: #filePath)          // …/Tests/NatKitTests/Theme/ColorSourcesTests.swift
            .deletingLastPathComponent()          // …/Tests/NatKitTests/Theme
            .deletingLastPathComponent()          // …/Tests/NatKitTests
            .deletingLastPathComponent()          // …/Tests
            .deletingLastPathComponent()          // …/macos
            .appendingPathComponent("Sources")
    }

    /// Where a colour is allowed to be written down: the two theme files,
    /// which are the mapping itself, and the terminal's bridge, which hands
    /// the palette's own hex strings to SwiftTerm because SwiftTerm takes
    /// `NSColor`s rather than reading `DesignTokens`.
    private let mayConstructColors = ["Theme/DesignTokens.swift", "Theme/Palette.swift", "TerminalTheme.swift"]

    /// SwiftUI's own named colours, which are the system's and drift with
    /// the OS appearance while everything around them is pinned. `clear` is
    /// not among them.
    private let systemColorNames = [
        "white", "black", "red", "green", "blue", "orange", "yellow", "pink",
        "purple", "gray", "grey", "teal", "mint", "indigo", "brown", "cyan",
        "primary", "secondary", "tertiary", "quaternary", "accentColor",
    ]

    private lazy var rules: [(String, String)] = [
        ("a colour built from an AppKit system colour", #"Color\(nsColor:\s*\."#),
        ("a colour built from a hex string", #"(Color|NSColor)\(hex:"#),
        ("a token pressed at the call site instead of named in the theme", #"DesignTokens\.[A-Za-z]+\s*\.opacity\("#),
        ("a SwiftUI system colour", #"\bColor\.(\#(systemColorNames.joined(separator: "|")))\b"#),
        (
            "a SwiftUI system colour, as a shorthand shape style",
            #"(foregroundStyle|foregroundColor|fill|background|tint|stroke|strokeBorder)\(\s*\.(\#(systemColorNames.joined(separator: "|")))\b"#
        ),
    ]

    func testNoSourceFileConstructsAColour() throws {
        let files = try swiftFiles()
        XCTAssertFalse(files.isEmpty, "no sources found at \(sourcesDirectory.path)")

        var strays: [String] = []
        for file in files {
            let relative = file.path.replacingOccurrences(of: sourcesDirectory.path + "/", with: "")
            if mayConstructColors.contains(where: relative.hasSuffix) { continue }
            let contents = try String(contentsOf: file, encoding: .utf8)
            for (line, text) in contents.components(separatedBy: .newlines).enumerated() {
                for (what, pattern) in rules where text.range(of: pattern, options: .regularExpression) != nil {
                    strays.append("\(relative):\(line + 1): \(what) — \(text.trimmingCharacters(in: .whitespaces))")
                }
            }
        }
        XCTAssertEqual(
            strays, [],
            """
            Every colour the app draws is a named `DesignTokens` value. \
            Give each of these a token — with the Catppuccin mapping and the \
            note on what it is for that the tokens beside it carry — rather \
            than a colour at the call site:
            \(strays.joined(separator: "\n"))
            """
        )
    }

    /// The scan is worth nothing if it is not reading the files it thinks it
    /// is: this is what would fail if the layout moved under it and left the
    /// test passing over an empty list.
    func testTheScanReadsTheAppsOwnSources() throws {
        let relatives = try swiftFiles().map {
            $0.path.replacingOccurrences(of: sourcesDirectory.path + "/", with: "")
        }
        XCTAssertTrue(relatives.contains("NatKit/Theme/DesignTokens.swift"), "\(relatives.count) files found")
        XCTAssertTrue(relatives.contains("NatApp/Views/DiffFileBoxView.swift"), "\(relatives.count) files found")
    }

    private func swiftFiles() throws -> [URL] {
        guard let walk = FileManager.default.enumerator(at: sourcesDirectory, includingPropertiesForKeys: nil) else {
            throw XCTSkip("no sources at \(sourcesDirectory.path)")
        }
        return walk.compactMap { $0 as? URL }.filter { $0.pathExtension == "swift" }.sorted { $0.path < $1.path }
    }
}
