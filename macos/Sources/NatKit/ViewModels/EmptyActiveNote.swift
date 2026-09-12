import Foundation

/// What the ACTIVE section says when nothing is running. The section is drawn
/// whether or not anything is in flight — a rail that grew a heading the
/// moment the first agent started read as chrome appearing from nowhere — so
/// it needs a line of its own for the empty case, and this is the one place
/// that line is written.
public enum EmptyActiveNote {
    /// Deliberately the shortest true sentence: the section's own heading has
    /// already said what would be listed here.
    public static let text = "Nothing running"
}
