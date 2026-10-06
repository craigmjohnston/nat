/// Whether ending a workshop session must ask first: only where something
/// would be lost — planning still under way, or a plan proposed and not yet
/// accepted. Every End session (the Brief's, the workshop row's ✕, a
/// project tab's close) takes this one rule.
public enum WorkshopEndRules {
    public static let workingMessage = "The planning agent is still working. Ending it loses the work in progress."
    public static let proposalMessage = "A plan has been proposed and not yet accepted. Ending the session discards it."

    /// The confirmation's message, or nil where the session can end at once.
    /// `activity` is the planning agent's, nil where none runs — nothing to
    /// end, so nothing to ask. A proposal up or an Accept in flight asks
    /// whatever the agent is doing; otherwise an agent working, or one whose
    /// activity could not be read (taken as working), asks. One waiting
    /// with nothing pending — after an Accept, or a round of workshopping
    /// that came to nothing — ends without asking.
    public static func confirmation(activity: AgentActivityState?, hasProposal: Bool, accepting: Bool) -> String? {
        guard let activity else { return nil }
        if hasProposal || accepting { return proposalMessage }
        switch activity {
        case .working, .unknown: return workingMessage
        case .waiting: return nil
        }
    }
}
