import Foundation

/// The one rule for whether a slice's work is actually done, applied
/// everywhere the app draws done-ness — the progress bar, the rail's DONE
/// folders and their counts: Notion says Done, and no reading says its pull
/// request is still open.
///
/// Notion's own Done now follows the merge too (`nat`'s merge writes it, and
/// the PR-readiness reading marks a slice Done when it finds the merge
/// happened on GitHub), so this gate matters only for the window before a
/// reading lands — and for slices marked Done under the old rule, at approve,
/// whose pull requests are still open. `openPRSliceIDs` is that reading's
/// keys; with none taken the set is empty and Notion's word stands, which is
/// what every finished project must go on reading as.
public func sliceWorkDone(_ slice: Slice, openPRSliceIDs: Set<String>) -> Bool {
    slice.status == "Done" && !openPRSliceIDs.contains(slice.id)
}
