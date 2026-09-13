import Foundation

/// The models and effort levels a launch's picker offers — the one place
/// both the Settings scene's Agents tab and the Brief tab's launch popover
/// read them from, so there is exactly one list of each to go stale.
///
/// `effort` is a closed set the CLI rejects anything outside of (see
/// `internal/tui/launch.go`'s own `effortLevels`), so it is worth reading
/// live: the installed `claude`'s own `--help` spells it out in full,
/// `(low, medium, high, xhigh, max)`, next to its `--effort` flag.
///
/// `model` is not a closed set at all — `claude --help` names a few aliases
/// as examples ("e.g. 'sonnet'"), not an exhaustive list, and a full model ID
/// is always a valid value the CLI passes straight through. So a picker over
/// `model` is suggestions rather than a menu of everything acceptable, and
/// every caller of this type must still let the field be typed into freely.
public struct AgentOptions: Equatable, Sendable {
    public var models: [String]
    public var efforts: [String]

    public init(models: [String], efforts: [String]) {
        self.models = models
        self.efforts = efforts
    }

    /// What every picker showed before there was a live source, and what one
    /// degrades to when `claude` cannot be found, its `--help` cannot be
    /// parsed, or it fails outright.
    public static let fallback = AgentOptions(
        models: ["sonnet", "opus", "haiku"],
        efforts: ["low", "medium", "high", "xhigh", "max"]
    )
}

/// Reads `AgentOptions` from the installed `claude` binary's own `--help`,
/// falling back to `AgentOptions.fallback` wherever that read or its parsing
/// comes up short — a machine with no `claude` on PATH included, since that
/// is simply the fallback list's whole reason to exist.
public enum AgentOptionsSource {
    /// Resolves live options, never throwing: any failure along the way —
    /// `claude` missing, a non-zero exit, output the parser does not
    /// recognise — reads as "no live source" rather than an error the caller
    /// has to handle.
    public static func resolve(commandRunner: CommandRunning = ProcessRunner()) async -> AgentOptions {
        guard let helpText = try? await helpText(commandRunner: commandRunner) else {
            return .fallback
        }
        return parse(helpText: helpText)
    }

    static func helpText(commandRunner: CommandRunning) async throws -> String {
        let (stdout, _, exitCode) = try await commandRunner.run(
            executable: "claude",
            arguments: ["--help"],
            workingDirectory: nil,
            standardInput: nil
        )
        guard exitCode == 0, let text = String(data: stdout, encoding: .utf8), !text.isEmpty else {
            throw NatError.missingOutput
        }
        return text
    }

    /// The pure parse, kept apart from the process spawn so it can be tested
    /// against real `--help` text without shelling out to anything.
    static func parse(helpText: String) -> AgentOptions {
        AgentOptions(
            models: parseModels(from: helpText) ?? AgentOptions.fallback.models,
            efforts: parseEfforts(from: helpText) ?? AgentOptions.fallback.efforts
        )
    }

    /// The comma list `--effort`'s own description parenthesises, e.g.
    /// `(low, medium, high, xhigh, max)` — read as the whole answer, since
    /// this one is a closed set rather than examples.
    static func parseEfforts(from helpText: String) -> [String]? {
        guard let block = flagBlock(named: "--effort", in: helpText),
              let list = firstParenthesised(in: block) else {
            return nil
        }
        let levels = list
            .split(separator: ",")
            .map { $0.trimmingCharacters(in: .whitespaces) }
            .filter { !$0.isEmpty }
        return levels.isEmpty ? nil : levels
    }

    /// The single-quoted aliases `--model`'s own description gives as
    /// examples, e.g. `'fable', 'opus', or 'sonnet'` — read as hints rather
    /// than the whole answer, since the description says "e.g." and a full
    /// model ID (also single-quoted there, as `'claude-fable-5'`) is not an
    /// alias to offer beside them. Anything hyphenated is a full ID rather
    /// than an alias and is dropped; what live aliases remain are put ahead
    /// of the fallback list, so a newly introduced alias (say, a family this
    /// build predates) shows up without losing one this `--help` no longer
    /// bothers to name as an example.
    static func parseModels(from helpText: String) -> [String]? {
        guard let block = flagBlock(named: "--model", in: helpText) else { return nil }
        // Alphanumeric only, with the quotes hugging it on both sides: an
        // alias reads `'sonnet'`, where a full model ID (`'claude-fable-5'`)
        // has a hyphen breaking the run and a stray apostrophe elsewhere in
        // the sentence (`model's full name`) has no closing quote right
        // after it — neither matches.
        let quoted = matches(of: "'([a-zA-Z0-9]+)'", in: block)
        guard !quoted.isEmpty else { return nil }
        var seen = Set<String>()
        var models: [String] = []
        for alias in quoted + AgentOptions.fallback.models where seen.insert(alias).inserted {
            models.append(alias)
        }
        return models
    }

    /// The text of one flag's own entry in `--help`'s output: its own line
    /// plus every line after it that continues the description, which
    /// `--help` wraps at a fixed indent rather than the flag's own two-space
    /// one. Recognising that difference — not just the following blank line
    /// — is what keeps a wrapped description (`--effort`'s split across two
    /// lines) from being cut off after its first line.
    private static func flagBlock(named flag: String, in helpText: String) -> String? {
        let lines = helpText.components(separatedBy: "\n")
        guard let start = lines.firstIndex(where: { $0.trimmingCharacters(in: .whitespaces).hasPrefix(flag) }) else {
            return nil
        }
        var collected = [lines[start]]
        var index = lines.index(after: start)
        while index < lines.endIndex {
            let line = lines[index]
            let indent = line.prefix { $0 == " " }.count
            let isNewFlag = indent <= 2 && line.trimmingCharacters(in: .whitespaces).hasPrefix("-")
            if isNewFlag || line.trimmingCharacters(in: .whitespaces).isEmpty {
                break
            }
            collected.append(line)
            index = lines.index(after: index)
        }
        return collected.joined(separator: " ")
    }

    private static func firstParenthesised(in text: String) -> String? {
        matches(of: "\\(([^()]+)\\)", in: text).first
    }

    private static func matches(of pattern: String, in text: String) -> [String] {
        guard let regex = try? NSRegularExpression(pattern: pattern) else { return [] }
        let range = NSRange(text.startIndex..., in: text)
        return regex.matches(in: text, range: range).compactMap { match in
            guard let group = Range(match.range(at: 1), in: text) else { return nil }
            return String(text[group])
        }
    }
}

/// Caches the resolved `AgentOptions` for the process's lifetime: reading it
/// is a process spawn, and every picker that shows it — Settings' Agents tab,
/// the Brief tab's launch popover — would otherwise repeat that spawn on
/// every appearance. An actor rather than a lock, since the cache is filled
/// by whichever caller gets there first and every later caller in the
/// meantime simply waits its turn.
public actor AgentOptionsCache {
    public static let shared = AgentOptionsCache()

    private var cached: AgentOptions?

    public func resolve(commandRunner: CommandRunning = ProcessRunner()) async -> AgentOptions {
        if let cached {
            return cached
        }
        let resolved = await AgentOptionsSource.resolve(commandRunner: commandRunner)
        cached = resolved
        return resolved
    }
}
