import SwiftUI

/// One frame of the gallery: a name, the view to draw under it, and the size
/// to draw it at.
///
/// A story is the app's answer to the Go side's golden snapshots. The TUI can
/// be rendered to a string and diffed; a window cannot, so what is written
/// down here instead is the *recipe* — which fixture state, which view, how
/// big — and the runner turns it into a PNG somebody can look at. Nothing
/// about it reaches outside the process: the state comes from `NatFixtures`
/// and the size is declared rather than measured off whatever window happens
/// to be open.
///
/// The content is built rather than held, and built `async`, because the
/// states worth drawing are loaded ones: a fixture app model has to be
/// started before its rail has anything in it, and starting is `await`. It is
/// `@MainActor` for the same reason every view is.
public struct Story: Identifiable {
    /// What `--story` names it by, and what its PNG is called — so it is
    /// written as a slug rather than as a sentence. `StoryCatalog` holds the
    /// rule.
    public let name: String

    /// The size the story is drawn at, in points. Declared per story because
    /// a whole window and a single pane are worth looking at at different
    /// sizes, and a pane stretched to a window's height says nothing true
    /// about how it is used.
    public let size: CGSize

    /// Which palette the story is drawn in. The gallery pins one rather than
    /// following the Mac, since a PNG that changes with the machine that
    /// rendered it is not a reference anybody can compare against.
    public let colorScheme: ColorScheme

    /// The view, built when the story is rendered.
    public let content: @MainActor () async -> AnyView

    public var id: String { name }

    /// The file a sweep writes this story to, relative to `--out`.
    public var fileName: String { "\(name).png" }

    /// A story takes any view rather than an `AnyView`, since a story body
    /// is a view expression and not an erasure; the erasure happens here, at
    /// the one place there is.
    public init<Content: View>(
        name: String,
        size: CGSize,
        colorScheme: ColorScheme = .dark,
        content: @escaping @MainActor () async -> Content
    ) {
        self.name = name
        self.size = size
        self.colorScheme = colorScheme
        self.content = { AnyView(await content()) }
    }
}

/// The stories there are, in the order `--list` and `--all` walk them.
///
/// A value rather than a global, so a test can hold a catalog of its own —
/// which is the only way the lookup can be checked without an AppKit window
/// in the way.
public struct StoryCatalog {
    public let stories: [Story]

    public init(_ stories: [Story]) {
        self.stories = stories
    }

    /// Every story's name, in catalog order.
    public var names: [String] { stories.map(\.name) }

    /// The story `--story` named, or nil for a name the catalog does not
    /// hold — which the runner reports with the names it does.
    public func story(named name: String) -> Story? {
        stories.first { $0.name == name }
    }

    /// Whether a name is safe as a file name and as a shell argument:
    /// lowercase letters, digits and hyphens, starting and ending with one of
    /// the first two.
    ///
    /// The rule is held over the shipped catalog by `StoryNamesTests`, which
    /// reads the names out of the source — a catalog of views cannot be built
    /// in a test target, so where the rule is checked is where the names are
    /// written.
    static func isSlug(_ name: String) -> Bool {
        guard let first = name.first, let last = name.last else { return false }
        let allowed = Set("abcdefghijklmnopqrstuvwxyz0123456789-")
        guard name.allSatisfy({ allowed.contains($0) }) else { return false }
        return first != "-" && last != "-"
    }
}
