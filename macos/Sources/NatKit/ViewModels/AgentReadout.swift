import Foundation

/// Context use at or above this percent renders in the warning tint.
public let contextWarningThreshold = 80.0

/// What the status bar's left readout draws for the attached agent:
/// "Sonnet 5 · high" (`label`) and "42%" (`context`), each independently
/// absent when `nat` had no value for it — never a zero.
public struct AgentReadout: Equatable {
    public struct Context: Equatable {
        public let text: String
        public let warning: Bool

        public init(text: String, warning: Bool) {
            self.text = text
            self.warning = warning
        }
    }

    public let label: String?
    public let context: Context?

    public init(label: String?, context: Context?) {
        self.label = label
        self.context = context
    }
}

/// Builds the readout from an agent's status; nil — nothing drawn, no
/// placeholder — with no agent or nothing known about it yet.
public func buildAgentReadout(from agent: AgentStatus?) -> AgentReadout? {
    guard let agent else { return nil }
    let parts = [agent.model, agent.effort].compactMap { $0 }.filter { !$0.isEmpty }
    let label = parts.isEmpty ? nil : parts.joined(separator: " · ")
    let context = agent.contextPercent.map {
        AgentReadout.Context(text: "\(Int($0.rounded()))%", warning: $0 >= contextWarningThreshold)
    }
    guard label != nil || context != nil else { return nil }
    return AgentReadout(label: label, context: context)
}
