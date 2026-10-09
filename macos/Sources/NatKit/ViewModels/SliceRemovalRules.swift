import Foundation

/// What a slice row menu's two destructive items — Delete and Cancel and
/// discard work — say and when they are offered, worked out from the slice's
/// status as nat reports it, so the sidebar draws the confirmation and decides
/// nothing.
///
/// The rules are the CLI's own (`internal/cli/slicedelete.go`,
/// `internal/cli/slicecancel.go`): a delete of any slice is allowed, one in
/// progress stopping its agent and discarding its worktree and branch; a
/// cancel is only for a slice in progress, so the item is greyed elsewhere —
/// a refusal the user never has to read.
public enum SliceRemovalRules {
    /// The status nat writes for a slice in progress.
    static let inProgress = "In progress"

    /// Whether "Cancel and discard work…" is offered: only a slice in
    /// progress has work to cancel — a Todo one has none started, and a Done
    /// one is merged.
    public static func canCancel(status: String?) -> Bool {
        status == inProgress
    }

    /// The delete confirmation's message for a slice of `status`: a Done
    /// slice is finished work whose record goes, one in progress takes its
    /// agent and work with it, and any page goes to Notion's trash.
    public static func deleteMessage(status: String?) -> String {
        switch status {
        case "Done":
            return "This task is Done, so deleting it removes the record of finished work. "
                + "The page goes to Notion's trash."
        case inProgress:
            return "This task is in progress. Its agent is stopped, and its worktree and branch, with any work "
                + "not yet on a pull request, are discarded. The page goes to Notion's trash."
        default:
            return "The page goes to Notion's trash."
        }
    }

    /// The cancel confirmation's message: everything a cancel throws away,
    /// where the task goes, and the one thing it leaves alone.
    public static let cancelMessage =
        "Its agent is stopped, and its branch and worktree, with all the work so far, are discarded. "
        + "The task goes back to Todo. An open pull request is left as it is."

    /// The cancel confirmation's destructive button — never "Cancel", which is
    /// the alert's own way out.
    public static let cancelButton = "Discard work"

    /// The slice row menu's cancel item.
    public static let cancelItem = "Cancel and discard work\u{2026}"
}
