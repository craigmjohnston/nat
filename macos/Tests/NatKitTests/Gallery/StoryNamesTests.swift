import XCTest
@testable import NatKit

/// The naming rule, held over the catalog the app actually ships.
///
/// It is a source scan for the same reason `ColorSourcesTests` is: the rule
/// cannot be checked where it would naturally be checked. The catalog is an
/// array of views and the views live in the executable, which a test target
/// cannot import — so what is read is the file the names are written in.
/// A name is a file name and a command-line argument, and a story called
/// "Window Shell" is a PNG nobody can name at a shell prompt and a `--story`
/// that needs quoting.
final class StoryNamesTests: XCTestCase {
    /// The shipped catalog, found from this file rather than from the working
    /// directory, which `swift test` does not promise anything about.
    private var catalogSource: URL {
        URL(fileURLWithPath: #filePath)      // …/Tests/NatKitTests/Gallery/StoryNamesTests.swift
            .deletingLastPathComponent()      // …/Tests/NatKitTests/Gallery
            .deletingLastPathComponent()      // …/Tests/NatKitTests
            .deletingLastPathComponent()      // …/Tests
            .deletingLastPathComponent()      // …/macos
            .appendingPathComponent("Sources/NatApp/Gallery/AppStories.swift")
    }

    /// Every shipped story's name and the summary written beside it.
    private func shippedStories() throws -> [(name: String, summary: String)] {
        let source = try String(contentsOf: catalogSource, encoding: .utf8)
        let pattern = try NSRegularExpression(
            pattern: #"Story\(\s*name:\s*"([^"]*)",\s*summary:\s*"([^"]*)""#)
        let range = NSRange(source.startIndex..., in: source)
        return pattern.matches(in: source, range: range).compactMap { match in
            guard let name = Range(match.range(at: 1), in: source),
                  let summary = Range(match.range(at: 2), in: source) else { return nil }
            return (String(source[name]), String(source[summary]))
        }
    }

    private func shippedNames() throws -> [String] {
        try shippedStories().map(\.name)
    }

    func testTheCatalogHasStoriesInIt() throws {
        // Belt and braces on the scan itself: a regex that stopped matching
        // would otherwise pass every assertion below by finding nothing.
        XCTAssertFalse(try shippedNames().isEmpty, "no stories found in \(catalogSource.path)")
    }

    func testEveryShippedStoryIsNamedAsASlug() throws {
        for name in try shippedNames() {
            XCTAssertTrue(
                StoryCatalog.isSlug(name),
                "story name \"\(name)\" is not a slug: lowercase letters, digits and hyphens")
        }
    }

    /// Two stories of one name would have the lookup answer with the first
    /// and a sweep write one over the other.
    func testNoTwoShippedStoriesShareAName() throws {
        let names = try shippedNames()
        XCTAssertEqual(Set(names).count, names.count, "a story name occurs twice in \(names)")
    }

    /// `--list` is an index, and a story with nothing said about it is a row
    /// of that index saying only its own file name.
    func testEveryShippedStoryHasASummary() throws {
        for story in try shippedStories() {
            XCTAssertFalse(
                story.summary.trimmingCharacters(in: .whitespaces).isEmpty,
                "story \"\(story.name)\" has no summary")
        }
    }

    /// The catalog covers the app rather than whatever was easiest to draw:
    /// the window shell, the rail's own load states, every tab of the
    /// workflow, and the two screens that belong to no slice. Held by name
    /// prefix, so a state may be renamed but a whole surface cannot quietly
    /// fall out of the gallery.
    func testEverySurfaceOfTheAppIsInTheCatalog() throws {
        let names = try shippedNames()
        for prefix in [
            "window-", "window-onboarding", "rail-skeleton", "rail-loaded", "rail-empty",
            "rail-error", "brief-", "agent-", "diff-", "pr-", "workshop-", "settings",
        ] {
            XCTAssertTrue(
                names.contains { $0.hasPrefix(prefix) },
                "no story for \(prefix) in \(names)")
        }
    }
}
