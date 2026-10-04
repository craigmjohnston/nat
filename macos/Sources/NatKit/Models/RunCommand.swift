import Foundation

/// Where a run command is offered, mirroring `internal/config`'s scope words:
/// `global` (the titlebar, run from origin/main), `slice` (a handed-back
/// slice's navigator, run from its worktree), or — the word left off — both.
/// A run is scopeless unless it does something different in a worktree: then
/// the same label is given once per scope.
public enum RunScope: String, CaseIterable, Equatable, Hashable, Sendable {
    case global, slice, both

    /// The word as config writes it: none at all for both.
    var word: String? { self == .both ? nil : rawValue }

    /// The scope a config word means; an absent or unknown one is both,
    /// since nat refuses an unknown word where it is written.
    init(word: String?) {
        self = word.flatMap(RunScope.init(rawValue:)) ?? .both
    }

    func offers(_ scope: RunScope) -> Bool { self == .both || self == scope }
}

/// One of a project's run commands, as its config entry holds it (mirrors
/// `config.RunCommand`): the label its button says, the shell command nat
/// runs by `sh -c`, and its scope.
public struct RunCommand: Codable, Equatable, Hashable, Sendable {
    public var label: String
    public var command: String
    public var scope: RunScope

    enum CodingKeys: String, CodingKey {
        case label, command, scope
    }

    public init(label: String, command: String, scope: RunScope = .both) {
        self.label = label
        self.command = command
        self.scope = scope
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        label = try c.decode(String.self, forKey: .label)
        command = try c.decode(String.self, forKey: .command)
        scope = RunScope(word: try c.decodeIfPresent(String.self, forKey: .scope))
    }

    /// Written as nat writes it: the scope only where it is one of the two.
    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(label, forKey: .label)
        try c.encode(command, forKey: .command)
        try c.encodeIfPresent(scope.word, forKey: .scope)
    }
}

extension Array where Element == RunCommand {
    /// The runs the titlebar offers, in the order written — the first is
    /// the default nat runs with no label.
    public var globalRuns: [RunCommand] { filter { $0.scope.offers(.global) } }

    /// The runs a handed-back slice's navigator offers, likewise.
    public var sliceRuns: [RunCommand] { filter { $0.scope.offers(.slice) } }
}

/// `nat run --json`'s answer (mirrors `internal/cli/run.go`'s `runJSON`): the
/// tmux session the run was started in, and what was run where.
public struct RunResult: Codable, Equatable, Sendable {
    public let session: String
    public let label: String
    public let command: String
    public let dir: String

    public init(session: String, label: String, command: String, dir: String) {
        self.session = session
        self.label = label
        self.command = command
        self.dir = dir
    }
}
