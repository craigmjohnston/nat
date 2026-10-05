import Foundation

/// What Send back to agent offers to say, before the user says anything: the
/// pull request's own trouble, where it has any — its failing checks, a
/// conflict with its base — as the reason the slice goes back. "" where it
/// has none, and the field opens empty.
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
        reasons.append("The branch conflicts with \(base): merge \(base) in and resolve the conflicts.")
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
        + "Make the change on the same branch. When it is done and the branch is pushed, "
        + "hand the slice back for review by running exactly:\n\n"
        + "nat complete-slice \(handBack.sliceRef) --project \(handBack.projectID) "
        + "--branch \(branchArgument) --summary '<what you changed>'\n"
}
