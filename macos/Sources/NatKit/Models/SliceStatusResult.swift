import Foundation

/// One slice's status read straight off its page — `nat slice-status`,
/// fresh rather than off a cached plan. This is the reaper's last word
/// before a kill: a session's own claim is always written before the
/// session exists, so a read taken after observing a live session can
/// never show a phantom state, where a plan load taken a poll ago might.
public enum SliceStatusResult: Sendable, Equatable {
    /// The page still exists and answers with its own Status column, and
    /// whether it has been moved to the trash — a trashed page still reads
    /// back its status, since Notion keeps it there to read.
    case found(status: String, trashed: Bool)
    /// Notion has no record of the page at all: one trashed for good, or an
    /// ID that never named one. Distinct from `found(trashed: true)`, which
    /// is a page still there to answer about.
    case gone
}
