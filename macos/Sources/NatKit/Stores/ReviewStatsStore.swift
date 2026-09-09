import Foundation

/// The NEEDS REVIEW rail rows' own "+N −N": each handed-back slice's branch
/// diff totals, summed across its files. Fetched lazily through the existing
/// `sliceDiff` read (there is no dedicated endpoint for a tally alone) and
/// cached by slice id — a slice already carrying a stat for the branch it is
/// still handed back on is left alone rather than re-read on every poll; only
/// a branch that has changed (a new hand-back, after a `release-slice` and a
/// fresh claim, say) is worth reading again.
///
/// A failure is quiet: the slice's row simply has no stat, and the next
/// `update` call — the next plan reload — tries again on its own, since
/// nothing here distinguishes "never fetched" from "fetched and failed"
/// beyond both leaving `stats` without an entry.
@MainActor
@Observable
public final class ReviewStatsStore {
    /// "+N −N" per slice id, tabular and ready to draw — absent for a slice
    /// never fetched, still in flight, or whose last fetch failed.
    public private(set) var stats: [String: String] = [:]

    /// How many files each branch touched, from the same fetch as `stats`
    /// and kept in step with it — the review row's "N files" meta.
    public private(set) var fileCounts: [String: Int] = [:]

    /// The readiness words of every pull request positively read as open,
    /// by slice id — `nat pr-status`'s reading, the same one the Go board's
    /// Active panel rides. A slice absent here has no open pull request as
    /// far as anything has read, which for a Done slice is what keeps a
    /// project's whole finished history out of the NEEDS REVIEW section.
    public private(set) var prReadiness: [String: String] = [:]

    private let client: NatClientProtocol
    private var branchBySlice: [String: String] = [:]

    public init(client: NatClientProtocol = NatClient()) {
        self.client = client
    }

    /// One handed-back slice, as `update` needs to know it: which slice, and
    /// which branch it is currently handed back on — a stat already held for
    /// the same branch is left alone, and one for a slice no longer in this
    /// list is dropped.
    public struct HandedBackSlice: Equatable, Sendable {
        public let sliceID: String
        public let branch: String

        public init(sliceID: String, branch: String) {
            self.sliceID = sliceID
            self.branch = branch
        }
    }

    /// Brings `stats` in line with the current set of handed-back slices:
    /// drops any slice no longer in the list, and fetches a fresh tally for
    /// every slice whose branch is new or has changed since the last fetch —
    /// in place, one at a time, so a test (or a caller) sees every fetch this
    /// call made once it returns rather than some still in flight.
    public func update(projectID: String, handedBack: [HandedBackSlice]) async {
        let currentIDs = Set(handedBack.map(\.sliceID))
        stats = stats.filter { currentIDs.contains($0.key) }
        fileCounts = fileCounts.filter { currentIDs.contains($0.key) }
        branchBySlice = branchBySlice.filter { currentIDs.contains($0.key) }

        for entry in handedBack {
            if branchBySlice[entry.sliceID] == entry.branch, stats[entry.sliceID] != nil {
                continue
            }
            branchBySlice[entry.sliceID] = entry.branch
            do {
                let diff = try await client.sliceDiff(projectID: projectID, sliceRef: entry.sliceID)
                let adds = diff.files.reduce(0) { $0 + $1.adds }
                let dels = diff.files.reduce(0) { $0 + $1.dels }
                stats[entry.sliceID] = "+\(adds) \u{2212}\(dels)"
                fileCounts[entry.sliceID] = diff.files.count
            } catch {
                // Quiet: the row is simply left with no stat. Unlike a
                // successful fetch, a failure does not stick — `stats` has no
                // entry for this slice afterwards, so the next `update` call
                // (the next plan reload) tries again on its own rather than
                // giving up on a branch for good over one bad read.
                stats.removeValue(forKey: entry.sliceID)
                fileCounts.removeValue(forKey: entry.sliceID)
            }
        }
    }

    /// Takes a fresh PR-readiness reading and replaces the last one with it.
    /// A reading that fails outright leaves the last one standing — stale
    /// news about an open pull request beats no news, and the next plan
    /// reload retries on its own. Within a reading that succeeded, a slice
    /// reported "unread" simply has no entry, exactly as the Go board holds
    /// it: nothing distinguishes a landed pull request from one gh could not
    /// be asked about, and neither keeps a slice in the section.
    public func updatePRStatus(projectID: String) async {
        guard let doc = try? await client.prStatus(projectID: projectID) else { return }
        prReadiness = doc.slices.reduce(into: [:]) { map, slice in
            if slice.isOpen {
                map[slice.sliceID] = slice.readiness
            }
        }
    }

    /// Clear everything, as if nothing had ever been fetched.
    public func clear() {
        stats = [:]
        fileCounts = [:]
        branchBySlice = [:]
        prReadiness = [:]
    }
}
