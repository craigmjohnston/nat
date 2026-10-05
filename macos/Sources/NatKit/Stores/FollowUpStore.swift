import Foundation
import Observation

/// What the user chose for one proposed follow-up — the three segments of
/// the sidebar's picker, in their order.
public enum FollowUpChoice: String, CaseIterable, Equatable, Sendable {
    case queue = "Queue"
    case fold = "Fold in"
    case drop = "Drop"
}

/// The triage cards' own state: the per-row choices the user has made so
/// far, the apply in flight and the error the last one left — every one
/// keyed by slice ID **and batch**, so two pending batches' cards share
/// nothing, and switching tabs or slices loses nothing. A choice is keyed by
/// the follow-up's place within its batch's pending items, 1-based — not by
/// the index `slice-triage` takes, which counts every batch's pending items and
/// so moves when an earlier batch is decided. None of it is persisted: the
/// proposals themselves are, on the slice page, and a choice not yet applied
/// is the user's to make again.
///
/// The decision itself is nat's (`slice-triage`); this only gathers it.
@MainActor
@Observable
public final class FollowUpStore {
    /// One batch of one slice's follow-ups.
    private struct Key: Hashable {
        let sliceID: String
        let batch: Int
    }

    private var choices: [Key: [Int: FollowUpChoice]] = [:]
    private var applying: Set<Key> = []
    private var errors: [Key: String] = [:]

    public init() {}

    /// The choice made for the follow-up at `position` (1-based) in a batch,
    /// nil while undecided.
    public func choice(sliceID: String, batch: Int, position: Int) -> FollowUpChoice? {
        choices[Key(sliceID: sliceID, batch: batch)]?[position]
    }

    /// Records (or, with nil, clears) the choice for one follow-up of a batch.
    public func setChoice(_ choice: FollowUpChoice?, sliceID: String, batch: Int, position: Int) {
        let key = Key(sliceID: sliceID, batch: batch)
        choices[key, default: [:]][position] = choice
        errors[key] = nil
    }

    /// Every choice made for a batch, by position in it.
    public func choices(sliceID: String, batch: Int) -> [Int: FollowUpChoice] {
        choices[Key(sliceID: sliceID, batch: batch)] ?? [:]
    }

    /// Whether an apply or a discard of this batch is in flight.
    public func isApplying(sliceID: String, batch: Int) -> Bool {
        applying.contains(Key(sliceID: sliceID, batch: batch))
    }

    /// Whether an apply or a discard of any batch of the slice is in flight.
    /// Every other batch's card waits it out: `slice-triage` names follow-ups
    /// by an index counting every batch, so until the slice is read again
    /// after one batch is decided, another card's indexes are stale.
    public func isApplying(sliceID: String) -> Bool {
        applying.contains { $0.sliceID == sliceID }
    }

    /// What the last apply or discard of the batch refused with.
    public func error(sliceID: String, batch: Int) -> String? {
        errors[Key(sliceID: sliceID, batch: batch)]
    }

    /// Apply is offered only once every row of the batch is decided — a
    /// partial batch is never applied, which is what makes a card's going
    /// away mean "all dealt with" — and never with a fold-in and no live
    /// agent to send it to. `choices` is by position in `followUps`.
    public nonisolated static func canApply(
        followUps: [FollowUp], choices: [Int: FollowUpChoice], hasLiveAgent: Bool
    ) -> Bool {
        guard !followUps.isEmpty else { return false }
        for position in followUps.indices.map({ $0 + 1 }) {
            guard let choice = choices[position] else { return false }
            if choice == .fold && !hasLiveAgent { return false }
        }
        return true
    }

    /// The card foot's summary of what Apply will do, nil until every row
    /// is decided — "Apply queues 1 slice under M53, folds 1 into this slice
    /// and drops 1." — or, undecided, the instruction to decide them.
    public nonisolated static func summary(
        followUps: [FollowUp], choices: [Int: FollowUpChoice], milestone: String
    ) -> String {
        let decided = followUps.indices.compactMap { choices[$0 + 1] }
        guard decided.count == followUps.count, !followUps.isEmpty else {
            return "Decide every follow-up to apply, or discard them all."
        }
        let queued = decided.filter { $0 == .queue }.count
        let folded = decided.filter { $0 == .fold }.count
        let dropped = decided.filter { $0 == .drop }.count
        var parts: [String] = []
        if queued > 0 {
            let place = milestone.isEmpty ? "" : " under \(milestone)"
            parts.append("queues \(queued) task\(queued == 1 ? "" : "s")\(place)")
        }
        if folded > 0 { parts.append("folds \(folded) into this task") }
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

    /// Applies one batch's choices through `slice-triage`, by the indexes
    /// `followUps` (that batch's pending items, as last read) carry, clearing
    /// them on success; a refusal is kept for the foot and the choices with
    /// it, so the user can change one and try again. `then` runs before the
    /// apply is over — the re-read that gives every other batch fresh indexes.
    @discardableResult
    public func apply(
        projectID: String, sliceID: String, batch: Int, followUps: [FollowUp], client: NatClientProtocol,
        then: () async -> Void = {}
    ) async -> TriageResult? {
        let chosen = choices(sliceID: sliceID, batch: batch)
        let pick = { (c: FollowUpChoice) in
            followUps.enumerated().filter { chosen[$0.offset + 1] == c }.map(\.element.index)
        }
        return await run(Key(sliceID: sliceID, batch: batch), then: then) {
            try await client.sliceTriage(
                projectID: projectID, sliceRef: sliceID,
                queue: pick(.queue), fold: pick(.fold), drop: pick(.drop)
            )
        }
    }

    /// Drops every follow-up of one batch through `slice-triage --drop` —
    /// no confirmation, since the proposals stay on the page with the drop
    /// recorded beneath them — leaving every other batch pending.
    @discardableResult
    public func discard(
        projectID: String, sliceID: String, batch: Int, followUps: [FollowUp], client: NatClientProtocol,
        then: () async -> Void = {}
    ) async -> TriageResult? {
        await run(Key(sliceID: sliceID, batch: batch), then: then) {
            try await client.sliceTriage(
                projectID: projectID, sliceRef: sliceID, queue: [], fold: [], drop: followUps.map(\.index))
        }
    }

    private func run(
        _ key: Key, then: () async -> Void, _ call: () async throws -> TriageResult
    ) async -> TriageResult? {
        guard !isApplying(sliceID: key.sliceID) else { return nil }
        applying.insert(key)
        errors[key] = nil
        defer { applying.remove(key) }
        do {
            let result = try await call()
            choices[key] = nil
            await then()
            return result
        } catch {
            errors[key] = error.localizedDescription
            return nil
        }
    }

    /// Holds an apply in flight for a batch, for a gallery story drawing a
    /// card mid-apply.
    public func markApplying(sliceID: String, batch: Int) {
        applying.insert(Key(sliceID: sliceID, batch: batch))
    }
}
