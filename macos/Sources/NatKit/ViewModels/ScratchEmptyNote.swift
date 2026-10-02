import Foundation

/// What an empty Scratch fold says: that it holds nothing, and the two ways
/// to give it something, each a link — the workshop agent, or a slice added
/// by hand (which in Scratch needs no milestone).
public enum ScratchEmptyNote {
    /// The note, its two ways in as markdown links to `Link`'s URLs.
    public static let markdown: AttributedString = {
        let source = "No slices. Use a [workshop agent](gnat-scratch:workshop) or [add one](gnat-scratch:add-slice) yourself."
        return (try? AttributedString(markdown: source)) ?? AttributedString(source)
    }()

    /// Which link was followed.
    public enum Link: Equatable {
        case workshop
        case addSlice

        /// The link a URL is, or nil for any URL the note does not hold.
        public init?(_ url: URL) {
            guard url.scheme == "gnat-scratch" else { return nil }
            switch url.absoluteString.dropFirst("gnat-scratch:".count) {
            case "workshop": self = .workshop
            case "add-slice": self = .addSlice
            default: return nil
            }
        }
    }
}
