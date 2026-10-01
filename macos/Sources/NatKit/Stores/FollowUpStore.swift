import Foundation
import Observation

/// What the user chose for one proposed follow-up — the three segments of
/// the sidebar's picker, in their order.
public enum FollowUpChoice: String, CaseIterable, Equatable, Sendable {
    case queue = "Queue"
    case fold = "Fold in"
    case drop = "Drop"
}

/// The Follow-ups sidebar's own state: the per-row choices the user has made
/// so far, the apply in flight and the error the last one left — every one
/// keyed by slice ID, the way `DiffStore.comments` holds a review's pending
/// comments, so switching tabs or slices loses nothing. None of it is
/// persisted: the proposals themselves are, on the slice page, and a choice
/// not yet applied is the user's to make again.
///
/// The decision itself is nat's (`slice-triage`); this only gathers it.
@MainActor
@Observable
public final class FollowUpStore {
    private var choices: [String: [Int: FollowUpChoice]] = [:]
    private var applying: Set<String> = []
    private var errors: [String: String] = [:]

    public init() {}

    /// The choice made for one follow-up of a slice, nil while undecided.
    public func choice(sliceID: String, index: Int) -> FollowUpChoice? {
        choices[sliceID]?[index]
    }

    /// Records (or, with nil, clears) the choice for one follow-up.
    public func setChoice(_ choice: FollowUpChoice?, sliceID: String, index: Int) {
        choices[sliceID, default: [:]][index] = choice
        errors[sliceID] = nil
    }

    /// Every choice made for a slice, by follow-up index.
    public func choices(sliceID: String) -> [Int: FollowUpChoice] {
        choices[sliceID] ?? [:]
    }

    /// Whether an apply or a discard is in flight for the slice.
    public func isApplying(sliceID: String) -> Bool {
        applying.contains(sliceID)
    }

    /// What the last apply or discard for the slice refused with.
    public func error(sliceID: String) -> String? {
        errors[sliceID]
    }

    /// Apply is offered only once every row is decided — a partial triage is
    /// never applied, which is what makes the sidebar's going away mean
    /// "all dealt with" — and never with a fold-in and no live agent to send
    /// it to.
    public nonisolated static func canApply(
        followUps: [FollowUp], choices: [Int: FollowUpChoice], hasLiveAgent: Bool
    ) -> Bool {
        guard !followUps.isEmpty else { return false }
        for followUp in followUps {
            guard let choice = choices[followUp.index] else { return false }
            if choice == .fold && !hasLiveAgent { return false }
        }
        return true
    }

    /// The sidebar foot's summary of what Apply will do, nil until every row
    /// is decided — "Apply queues 1 slice under M53, folds 1 into this slice
    /// and drops 1." — or, undecided, the instruction to decide them.
    public nonisolated static func summary(
        followUps: [FollowUp], choices: [Int: FollowUpChoice], milestone: String
    ) -> String {
        let decided = followUps.compactMap { choices[$0.index] }
        guard decided.count == followUps.count, !followUps.isEmpty else {
            return "Decide every follow-up to apply, or discard them all."
        }
        let queued = decided.filter { $0 == .queue }.count
        let folded = decided.filter { $0 == .fold }.count
        let dropped = decided.filter { $0 == .drop }.count
        var parts: [String] = []
        if queued > 0 {
            let place = milestone.isEmpty ? "" : " under \(milestone)"
            parts.append("queues \(queued) slice\(queued == 1 ? "" : "s")\(place)")
        }
        if folded > 0 { parts.append("folds \(folded) into this slice") }
        if dropped > 0 { parts.append("drops \(dropped)") }
        let joined = parts.count > 1
            ? parts.dropLast().joined(separator: ", ") + " and " + parts.last!
            : parts[0]
        return "Apply \(joined)."
    }

    /// The foot while an apply is in flight: "Queued 1 · sending 1 to the
    /// agent…", or "Recording…" where nothing is queued or sent.
    public nonisolated static func applyingSummary(choices: [Int: FollowUpChoice]) -> String {
        let queued = choices.values.filter { $0 == .queue }.count
        let folded = choices.values.filter { $0 == .fold }.count
        var parts: [String] = []
        if queued > 0 { parts.append("Queued \(queued)") }
        if folded > 0 { parts.append("sending \(folded) to the agent…") }
        if parts.isEmpty { return "Recording…" }
        if folded == 0 { return parts[0] + "…" }
        return parts.joined(separator: " · ")
    }

    /// Applies the slice's choices through `slice-triage`, clearing them on
    /// success; a refusal is kept for the foot and the choices with it, so
    /// the user can change one and try again.
    @discardableResult
    public func apply(
        projectID: String, sliceID: String, followUps: [FollowUp], client: NatClientProtocol
    ) async -> TriageResult? {
        let chosen = choices(sliceID: sliceID)
        let pick = { (c: FollowUpChoice) in followUps.map(\.index).filter { chosen[$0] == c } }
        return await run(sliceID: sliceID) {
            try await client.sliceTriage(
                projectID: projectID, sliceRef: sliceID,
                queue: pick(.queue), fold: pick(.fold), drop: pick(.drop)
            )
        }
    }

    /// Drops every proposal through `slice-triage --drop-all` — no
    /// confirmation, since the proposals stay on the page with the drop
    /// recorded beneath them.
    @discardableResult
    public func discardAll(projectID: String, sliceID: String, client: NatClientProtocol) async -> TriageResult? {
        await run(sliceID: sliceID) {
            try await client.sliceDiscardFollowUps(projectID: projectID, sliceRef: sliceID)
        }
    }

    private func run(sliceID: String, _ call: () async throws -> TriageResult) async -> TriageResult? {
        guard !applying.contains(sliceID) else { return nil }
        applying.insert(sliceID)
        errors[sliceID] = nil
        defer { applying.remove(sliceID) }
        do {
            let result = try await call()
            choices[sliceID] = nil
            return result
        } catch {
            errors[sliceID] = error.localizedDescription
            return nil
        }
    }

    /// Holds an apply in flight for a slice, for a gallery story drawing the
    /// sidebar mid-apply.
    public func markApplying(sliceID: String) {
        applying.insert(sliceID)
    }
}
