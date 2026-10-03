import Foundation

/// Context use at or above this percent renders in the warning tint.
public let contextWarningThreshold = 80.0

/// What the titlebar band draws for its agent, kept small since it stands
/// beside the tabs: the model and effort apart ("Sonnet 5", "high"), and the
/// context as its bare percent ("42%") — each independently absent when
/// `nat` had no value for it, never a zero. `detail` is the long form for
/// the readout's tooltip: "Sonnet 5 / high · context 42% (84k tokens)".
public struct AgentReadout: Equatable {
    public struct Context: Equatable {
        public let text: String
        public let warning: Bool

        public init(text: String, warning: Bool) {
            self.text = text
            self.warning = warning
        }
    }

    public let model: String?
    public let effort: String?
    public let context: Context?
    public let detail: String

    public init(model: String?, effort: String?, context: Context?, detail: String) {
        self.model = model
        self.effort = effort
        self.context = context
        self.detail = detail
    }
}

/// Builds the readout from an agent's status; nil — nothing drawn, no
/// placeholder — with no agent or nothing known about it yet.
public func buildAgentReadout(from agent: AgentStatus?) -> AgentReadout? {
    guard let agent else { return nil }
    let model = agent.model.flatMap { $0.isEmpty ? nil : $0 }
    let effort = agent.effort.flatMap { $0.isEmpty ? nil : $0 }
    let percent = agent.contextPercent.map { Int($0.rounded()) }
    let context = agent.contextPercent.map { value in
        AgentReadout.Context(text: "\(Int(value.rounded()))%", warning: value >= contextWarningThreshold)
    }
    guard model != nil || effort != nil || context != nil else { return nil }
    let tokens = agent.contextTokens.map { " (\(formatTokenCount($0)) tokens)" } ?? ""
    let detail = [
        modelEffortLabel(model: model ?? "", effort: effort ?? ""),
        percent.map { "context \($0)%\(tokens)" },
    ].compactMap { $0 }.joined(separator: " \u{00B7} ")
    return AgentReadout(model: model, effort: effort, context: context, detail: detail)
}

/// A model and effort as the heading writes them, "Sonnet 5 / high" — a
/// live agent's, or the ones a launch will run with. Nil with neither.
public func modelEffortLabel(model: String, effort: String) -> String? {
    let parts = [model, effort].filter { !$0.isEmpty }
    return parts.isEmpty ? nil : parts.joined(separator: " / ")
}

/// A token count as the heading writes it: whole below a thousand, then
/// thousands ("326k"), then millions to one place ("1.2m").
func formatTokenCount(_ count: Int) -> String {
    if count < 1000 { return "\(count)" }
    if count < 999_500 { return "\(Int((Double(count) / 1000).rounded()))k" }
    return String(format: "%.1fm", Double(count) / 1_000_000)
}
