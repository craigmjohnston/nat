import SwiftUI
import XCTest
@testable import NatKit

final class StoryTests: XCTestCase {
    /// A catalog of the smallest stories there can be — a name, a size and a
    /// view that draws nothing. What is under test is the registry, not the
    /// rendering: drawing needs a WindowServer and lives in the app.
    @MainActor
    private func catalog(_ names: String...) -> StoryCatalog {
        StoryCatalog(names.map { name in
            Story(name: name, size: CGSize(width: 10, height: 10)) { EmptyView() }
        })
    }

    @MainActor
    func testNamesAreInCatalogOrder() {
        XCTAssertEqual(catalog("b", "a", "c").names, ["b", "a", "c"])
    }

    @MainActor
    func testLookupFindsAStory() {
        let story = catalog("one", "two").story(named: "two")
        XCTAssertEqual(story?.name, "two")
        // The story is its own name: the id is what `--story` matches on.
        XCTAssertEqual(story?.id, "two")
    }

    @MainActor
    func testLookupOfAStoryTheCatalogDoesNotHold() {
        XCTAssertNil(catalog("one").story(named: "two"))
    }

    @MainActor
    func testASweepWritesOnePNGPerStory() {
        XCTAssertEqual(catalog("one", "two").stories.map(\.fileName), ["one.png", "two.png"])
    }

    @MainActor
    func testTheSizeAndSchemeAreTheStorysOwn() {
        let story = Story(
            name: "light", size: CGSize(width: 320, height: 200), colorScheme: .light
        ) { EmptyView() }
        XCTAssertEqual(story.size, CGSize(width: 320, height: 200))
        XCTAssertEqual(story.colorScheme, .light)
    }

    /// Dark unless a story says otherwise: the app is drawn dark in the mock,
    /// and a gallery that followed the Mac would render differently on two
    /// machines.
    @MainActor
    func testTheDefaultSchemeIsDark() {
        XCTAssertEqual(catalog("one").stories[0].colorScheme, .dark)
    }

    /// The content is a recipe and not a view: a catalog is a value anything
    /// may hold, and building a story's view starts a fixture app model, so
    /// `--list` must not build one.
    @MainActor
    func testContentIsBuiltWhenItIsAsked() async {
        let built = Counter()
        let story = Story(name: "counted", size: CGSize(width: 10, height: 10)) {
            built.value += 1
            return EmptyView()
        }
        XCTAssertEqual(built.value, 0, "a catalog should build nothing by being made")
        _ = await story.content()
        _ = await story.content()
        XCTAssertEqual(built.value, 2)
    }

    /// A box, because a story's builder escapes and a local cannot be
    /// mutated from one.
    @MainActor
    private final class Counter {
        var value = 0
    }

    // MARK: - The naming rule

    func testSlugs() {
        for name in ["a", "window-shell", "pr-ready-to-merge", "diff2", "0"] {
            XCTAssertTrue(StoryCatalog.isSlug(name), name)
        }
    }

    func testNotSlugs() {
        for name in ["", "-leading", "trailing-", "Window-Shell", "with space", "under_score", "dot.png", "é"] {
            XCTAssertFalse(StoryCatalog.isSlug(name), name)
        }
    }
}
