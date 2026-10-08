import Foundation

/// What Send back to agent offers to say, before the user says anything: the
/// pull request's own trouble, where it has any — its failing checks, a
/// conflict with its base (or a handed-back branch's, with none) — as the
/// reason the slice goes back. "" where it has none, and the field opens
/// empty.
public func sendBackReason(checks: ChecksNotice?, conflict: ConflictNotice?) -> String {
    var reasons: [String] = []
    if let checks {
        reasons.append(
            checks.checks.isEmpty
                ? "Checks are failing on the pull request."
                : "Checks are failing on the pull request: \(checks.checks.joined(separator: ", ")).")
    }
    if let conflict {
        let base = conflict.conflict.base ?? "its base"
        reasons.append("The branch conflicts with \(base): \(conflict.remedy) and resolve the conflicts.")
    }
    return reasons.joined(separator: " ")
}

/// What a live agent is told when its handed-back slice is sent back to it:
/// why, as the user put it, then the hand-back that ends the new work —
/// `complete-slice --branch` on the same branch, which is what re-records
/// the Branch `nat slice-resume` cleared, and so what brings the slice back
/// for review. `branch` is the branch the slice had recorded before it was
/// resumed; nil words the flag for the agent to fill in.
public func sendBackPrompt(note: String, branch: String?, handBack: HandBackInstruction) -> String {
    let branchArgument = branch.flatMap { $0.isEmpty ? nil : $0 } ?? "<your branch>"
    return "I am sending this task back to you for more work:\n\n"
        + note.trimmingCharacters(in: .whitespacesAndNewlines) + "\n\n"
        + "Make the change on the same branch. When it is done and committed, "
        + "hand the slice back for review by running exactly:\n\n"
        + "nat complete-slice \(handBack.sliceRef) --project \(handBack.projectID) "
        + "--branch \(branchArgument) --summary '<what you changed>'\n"
        + "\nThat hand-back pushes the branch: do not push it yourself.\n"
}

/// The trouble the action bar offers to fix in place of Approve or Merge: a
/// pull request's failing checks while no agent is live — with one live, nat
/// has already sent them to it — and a conflict with its base (or, in
/// review, a handed-back branch's), live agent or not, since nothing sends a
/// conflict on its own. Nil where neither counts: the bar is as it was.
/// Fixing is Send back to agent with `note` as the reason, sent straight
/// away — no editor.
public struct BarFix: Equatable, Sendable {
    public let checks: ChecksNotice?
    public let conflict: ConflictNotice?

    public init?(checks: ChecksNotice?, conflict: ConflictNotice?) {
        let checks = checks?.action == .sendBack ? checks : nil
        let conflict = conflict.flatMap { $0.action == .sendBack || $0.action == .liveAgent ? $0 : nil }
        guard checks != nil || conflict != nil else { return nil }
        self.checks = checks
        self.conflict = conflict
    }

    /// The bar button's words, saying what the agent will be sent to do.
    public var title: String {
        switch (checks, conflict) {
        case (nil, _): return "Resolve conflicts"
        case (_, nil): return "Fix failing checks"
        default: return "Fix checks and conflicts"
        }
    }

    /// The reason the slice goes back: the troubles that count, in
    /// `sendBackReason`'s words.
    public var note: String { sendBackReason(checks: checks, conflict: conflict) }
}
