import Foundation

/// One workshop's proposal, as the app holds it: a small state machine over
/// the readings of `nat plan-proposal` and the user's Accept, so that what is
/// on screen never depends on which subprocess happens to finish first.
///
/// Every reading takes a ticket before it asks nat and hands it back with
/// what it found. A reading lands only if nothing has happened to the
/// proposal since it began — no later reading has landed, and no Accept has
/// begun or ended — and is otherwise dropped as stale. So readings that
/// finish out of order cannot put an older proposal over a newer one, and a
/// reading taken before an Accept finished cannot bring the accepted proposal
/// back. While an Accept is under way nothing lands at all: the proposal is
/// the one being accepted until nat says how that went.
///
/// The proposal file is the one source: a reading that finds none clears the
/// proposal (it was accepted, here or by another nat), and a reading that
/// failed concludes nothing and is simply never handed back.
public struct ProposalState: Equatable, Sendable {
    public private(set) var proposal: PlanProposal?
    public private(set) var accepting = false
    /// What the last Accept refused with, or the name field's own refusal —
    /// drawn at the name field.
    public private(set) var error: String?

    /// The next reading's ticket.
    private var nextTicket = 0
    /// The oldest ticket that may still land: raised past every reading
    /// begun before a landing, an Accept's start or its end.
    private var floor = 0

    public init() {}

    /// A reading is about to ask nat: its ticket, to hand back with what it
    /// found.
    public mutating func beginReading() -> Int {
        defer { nextTicket += 1 }
        return nextTicket
    }

    /// What the reading with `ticket` found — nil for no proposal file.
    /// Returns whether it landed.
    @discardableResult
    public mutating func land(_ ticket: Int, found: PlanProposal?) -> Bool {
        guard !accepting, ticket >= floor else { return false }
        floor = ticket + 1
        if found != proposal { error = nil }
        proposal = found
        return true
    }

    /// The user's Accept is starting: the proposal it is accepting, or nil
    /// when there is none to accept or one is already being accepted. Every
    /// reading already asked is stale from here.
    public mutating func beginAccept() -> PlanProposal? {
        guard let proposal, !accepting else { return nil }
        accepting = true
        error = nil
        floor = nextTicket
        return proposal
    }

    /// The Accept is over: accepted, the proposal is gone; refused, it stays
    /// with the reason. Every reading asked during it is stale either way —
    /// it may have read the file before nat removed it.
    public mutating func endAccept(refusal: String?) {
        guard accepting else { return }
        accepting = false
        floor = nextTicket
        if let refusal {
            error = refusal
        } else {
            proposal = nil
        }
    }

    /// The tab is gone: nothing is held, and every reading already asked is
    /// stale — so one still in flight cannot bring this tab's proposal back.
    public mutating func discard() {
        proposal = nil
        accepting = false
        error = nil
        floor = nextTicket
    }

    /// The user wrote to the workshop's agent: the proposal is stale and
    /// comes down at once, before nat removes the file — every reading
    /// already asked is stale, so one still in flight cannot bring it back.
    /// Returns whether there was one to withdraw: none, or one being
    /// accepted, is left as it is.
    @discardableResult
    public mutating func withdraw() -> Bool {
        guard proposal != nil, !accepting else { return false }
        proposal = nil
        error = nil
        floor = nextTicket
        return true
    }

    /// A refusal made before nat is asked — the name field's.
    public mutating func refuse(_ reason: String) {
        error = reason
    }

    /// The user changed what they are accepting; the last refusal no longer
    /// stands.
    public mutating func clearError() {
        error = nil
    }
}
