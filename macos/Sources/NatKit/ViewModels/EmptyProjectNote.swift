import Foundation

/// What a project with nothing in it says for itself — the state every
/// project opened or created from the "+" tab starts in, and the one place
/// the rail and the pane take their wording from, so the two halves of an
/// empty board cannot say different things.
public enum EmptyProjectNote {
    /// The heading: there is a plan, and it holds nothing yet.
    public static let title = "No slices yet"

    /// What to do about it. A project opened from the "+" tab has no working
    /// directory recorded — opening records where a plan lives and nothing
    /// about where its code does — and that is the first thing to fix, since
    /// every agent this board launches starts in it.
    public static func subtitle(needsWorkingDir: Bool) -> String {
        if needsWorkingDir {
            return "This project has no working directory on this Mac. Set one in Settings (⌘,) so agents launch in its checkout, then workshop the plan with the wand above."
        }
        return "Workshop the plan with the wand above, or file a slice with the plus beside it."
    }
}
