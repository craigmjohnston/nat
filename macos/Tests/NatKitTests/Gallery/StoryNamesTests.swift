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

    private func shippedNames() throws -> [String] {
        let source = try String(contentsOf: catalogSource, encoding: .utf8)
        let pattern = try NSRegularExpression(pattern: #"Story\(name:\s*"([^"]*)""#)
        let range = NSRange(source.startIndex..., in: source)
        return pattern.matches(in: source, range: range).compactMap { match in
            Range(match.range(at: 1), in: source).map { String(source[$0]) }
        }
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
}
