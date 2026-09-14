import XCTest
@testable import NatKit

/// A `--help` transcript shaped like the real `claude --help`'s: an
/// `--effort` entry whose parenthesised list wraps onto its own line, and a
/// `--model` entry whose examples are single-quoted aliases followed by a
/// single-quoted full model ID — the one the parser must not offer as an
/// alias. Flags before and after are there so the block reader has a real
/// boundary to stop at on both sides.
private let realisticHelpText = """
Options:
  --add-dir <directories...>            Additional directories to allow tool
                                        access to
  --effort <level>                      Effort level for the current session
                                        (low, medium, high, xhigh, max)
  --model <model>                       Model for the current session. Provide
                                        an alias for the latest model (e.g.
                                        'fable', 'opus', or 'sonnet') or a
                                        model's full name (e.g.
                                        'claude-fable-5').
  -n, --name <name>                     Set a display name for this session
"""

/// A fake `CommandRunning` that answers `claude --help` (or fails it) without
/// spawning anything.
private final class FakeClaudeRunner: CommandRunning, @unchecked Sendable {
    enum Answer {
        case helpText(String)
        case nonZeroExit
        case notFound
    }

    private let answer: Answer
    private(set) var lastExecutable: String?

    init(_ answer: Answer) {
        self.answer = answer
    }

    func run(
        executable: String,
        arguments: [String],
        workingDirectory: String?,
        standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
        lastExecutable = executable
        switch answer {
        case .helpText(let text):
            return (text.data(using: .utf8)!, Data(), 0)
        case .nonZeroExit:
            return (Data(), "claude: command not found".data(using: .utf8)!, 1)
        case .notFound:
            throw NatError.missingOutput
        }
    }
}

final class AgentOptionsTests: XCTestCase {

    // MARK: - fallback

    /// The documented alias set from
    /// https://code.claude.com/docs/en/model-config, including the
    /// bracketed `[1m]` variants — there is no enumeration API, so this list
    /// is what the fallback (and every picker's Custom escape hatch) is
    /// built on.
    func testFallbackModelsAreTheDocumentedAliasSet() {
        XCTAssertEqual(
            AgentOptions.fallback.models,
            ["default", "fable", "opus", "sonnet", "haiku", "sonnet[1m]", "opus[1m]", "opusplan"]
        )
    }

    /// The `--help` alias regex is alphanumeric-only and must not itself
    /// match a bracketed alias — bracketed variants only ever reach a picker
    /// through the fallback list, never through a live `--help` parse.
    func testBracketedFallbackAliasesAreNotDroppedByTheLiveParse() {
        let options = AgentOptionsSource.parse(helpText: realisticHelpText)
        XCTAssertTrue(options.models.contains("sonnet[1m]"))
        XCTAssertTrue(options.models.contains("opus[1m]"))
        XCTAssertTrue(options.models.contains("opusplan"))
        XCTAssertTrue(options.models.contains("default"))
    }

    // MARK: - Parsing

    func testParsesEffortsFromTheirParenthesisedList() {
        let options = AgentOptionsSource.parse(helpText: realisticHelpText)
        XCTAssertEqual(options.efforts, ["low", "medium", "high", "xhigh", "max"])
    }

    func testParsesModelAliasesAheadOfTheFallbackList() {
        let options = AgentOptionsSource.parse(helpText: realisticHelpText)
        // The live aliases come first, in the order --help lists them...
        XCTAssertEqual(Array(options.models.prefix(2)), ["fable", "opus"])
        // ...folded with the fallback list rather than replacing it, so an
        // alias this --help no longer bothers to give as an example (haiku)
        // is not lost.
        XCTAssertTrue(options.models.contains("sonnet"))
        XCTAssertTrue(options.models.contains("haiku"))
        // A hyphenated example is a full model ID, not an alias, and must
        // not be offered as one.
        XCTAssertFalse(options.models.contains("claude-fable-5"))
    }

    func testModelAliasesAreNotDuplicatedWithTheFallbackList() {
        let options = AgentOptionsSource.parse(helpText: realisticHelpText)
        XCTAssertEqual(options.models.filter { $0 == "opus" }.count, 1)
    }

    func testFallsBackWhenTheHelpTextNamesNeitherFlag() {
        let options = AgentOptionsSource.parse(helpText: "Options:\n  --version   Print the version\n")
        XCTAssertEqual(options, .fallback)
    }

    func testFallsBackWhenTheEffortLineHasNoParenthesisedList() {
        let helpText = """
        Options:
          --effort <level>   Effort level for the current session
          --model <model>    Model for the current session (e.g. 'sonnet')
        """
        let options = AgentOptionsSource.parse(helpText: helpText)
        XCTAssertEqual(options.efforts, AgentOptions.fallback.efforts)
        // "sonnet" is already in the fallback list, so folding it in moves it
        // to the front and changes nothing else about the result.
        XCTAssertEqual(options.models.first, "sonnet")
        XCTAssertEqual(Set(options.models), Set(AgentOptions.fallback.models))
        XCTAssertEqual(options.models.count, AgentOptions.fallback.models.count)
    }

    // MARK: - resolve()

    func testResolveReadsLiveOptionsWhenClaudeSucceeds() async {
        let runner = FakeClaudeRunner(.helpText(realisticHelpText))
        let options = await AgentOptionsSource.resolve(commandRunner: runner)
        XCTAssertEqual(options.efforts, ["low", "medium", "high", "xhigh", "max"])
        XCTAssertEqual(runner.lastExecutable, "claude")
    }

    func testResolveDegradesToFallbackOnANonZeroExit() async {
        let runner = FakeClaudeRunner(.nonZeroExit)
        let options = await AgentOptionsSource.resolve(commandRunner: runner)
        XCTAssertEqual(options, .fallback)
    }

    func testResolveDegradesToFallbackWhenClaudeCannotBeRun() async {
        let runner = FakeClaudeRunner(.notFound)
        let options = await AgentOptionsSource.resolve(commandRunner: runner)
        XCTAssertEqual(options, .fallback)
    }

    // MARK: - AgentOptionsCache

    func testCacheResolvesOnceAndReusesTheResultAfter() async {
        let runner = FakeClaudeRunner(.helpText(realisticHelpText))
        let cache = AgentOptionsCache()

        let first = await cache.resolve(commandRunner: runner)
        // A second, differently-answering runner: if the cache actually
        // shells out again, this would be what it reads instead.
        let stale = FakeClaudeRunner(.nonZeroExit)
        let second = await cache.resolve(commandRunner: stale)

        XCTAssertEqual(first, second)
        XCTAssertEqual(second.efforts, ["low", "medium", "high", "xhigh", "max"])
    }
}
