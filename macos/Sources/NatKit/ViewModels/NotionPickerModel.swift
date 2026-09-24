import Foundation

/// The Notion page picker's state: the workspace search, the row chosen, and
/// the create in flight. The view binds to it and draws; nothing here is a
/// view's to compute.
///
/// The list is `nat notion-search`'s — the same search the workspace has,
/// narrowed by what is typed — and it is read afresh for every query rather
/// than filtered here, since what matches is Notion's to say. A newer query
/// supersedes an older one still in flight: an answer for text no longer in
/// the field is dropped, not drawn.
@MainActor
@Observable
public final class NotionPickerModel {
    public private(set) var places: [NotionPlace] = []
    public private(set) var isSearching = false

    /// What the last search refused with, shown in place of the list.
    public private(set) var searchError: String?

    /// What the last create refused with, shown above the footer with the row
    /// still chosen, so the user can try again or choose elsewhere.
    public private(set) var createError: String?
    public private(set) var isCreating = false

    public var query = ""
    public var selectedID: String?

    @ObservationIgnored private let client: NatClientProtocol
    @ObservationIgnored private var searchGeneration = 0

    public init(client: NatClientProtocol) {
        self.client = client
    }

    public var selection: NotionPlace? {
        places.first { $0.id == selectedID }
    }

    /// "Create page" needs a row and nothing already under way.
    public var canCreate: Bool {
        selection != nil && !isCreating
    }

    /// Search for the query as it stands, replacing the list. A row chosen
    /// that the new list no longer holds is un-chosen, since nothing on screen
    /// would say what "Create page" is about to use.
    public func search() async {
        searchGeneration += 1
        let generation = searchGeneration
        isSearching = true
        defer { if generation == searchGeneration { isSearching = false } }
        do {
            let found = try await client.notionSearch(
                query: query.trimmingCharacters(in: .whitespacesAndNewlines))
            guard generation == searchGeneration else { return }
            places = found
            searchError = nil
            if selectedID != nil, selection == nil { selectedID = nil }
        } catch {
            guard generation == searchGeneration else { return }
            places = []
            selectedID = nil
            searchError = Self.message(error)
        }
    }

    /// Create the project page under the chosen row through `mirror` — the
    /// app's own mirror, which answers nil on success and the refusal
    /// otherwise. True once it took.
    public func create(_ mirror: (NotionPlace) async -> String?) async -> Bool {
        guard let place = selection, !isCreating else { return false }
        isCreating = true
        createError = nil
        defer { isCreating = false }
        if let refusal = await mirror(place) {
            createError = refusal
            return false
        }
        return true
    }

    private static func message(_ error: Error) -> String {
        if case NatError.commandFailed(let message) = error { return message }
        return error.localizedDescription
    }
}
