import Foundation

/// Envelope for status response.
private struct StatusEnvelope: Codable {
    let agents: [AgentStatus]
}

/// Envelope for `slice-approve --json`'s output.
private struct ApproveEnvelope: Codable {
    let url: String
}

/// Envelope for `pr-merge --json`'s output — always `{"merged":true}` on
/// success, since a refusal comes back as a non-zero exit rather than
/// `false`. Decoded (and discarded) anyway, so a `nat` whose shape has
/// drifted fails loudly here rather than being read as a silent success.
private struct MergedEnvelope: Codable {
    let merged: Bool
}

/// Envelope for `slice-add --json`'s output: the created slice, wrapped the
/// same way `add.go`'s `sliceAddedJSON` wraps it.
private struct SliceAddedEnvelope: Codable {
    let slice: SliceAddResult
}

/// A typed client for running nat commands and decoding their JSON output.
public final class NatClient: Sendable {
    private nonisolated let commandRunner: CommandRunning
    private nonisolated let workingDirectory: String?

    public init(
        commandRunner: CommandRunning = ProcessRunner(),
        workingDirectory: String? = nil
    ) {
        self.commandRunner = commandRunner
        self.workingDirectory = workingDirectory
    }

    /// Get information about a project.
    ///
    /// - Parameter projectID: The project's Notion page ID
    /// - Returns: ProjectInfo containing the project, milestones, and slices
    /// - Throws: NatError if the command fails or output is invalid
    public func info(projectID: String) async throws -> ProjectInfo {
        let output = try await runNat(arguments: ["info", "--project", projectID, "--json"])
        return try decodeJSON(ProjectInfo.self, from: output)
    }

    /// Get paths to nat's configuration and runtime files.
    ///
    /// - Returns: NatPaths containing config path, log directory, and nudge file path
    /// - Throws: NatError if the command fails or output is invalid
    public func paths() async throws -> NatPaths {
        let output = try await runNat(arguments: ["paths", "--json"])
        return try decodeJSON(NatPaths.self, from: output)
    }

    /// Get the live status of all running agents.
    ///
    /// - Returns: Array of AgentStatus for all running agents (gone agents omitted)
    /// - Throws: NatError if the command fails or output is invalid
    public func status() async throws -> [AgentStatus] {
        let output = try await runNat(arguments: ["status", "--json"])
        let envelope = try decodeJSON(StatusEnvelope.self, from: output)
        return envelope.agents
    }

    /// Get full details of a slice including its brief.
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    /// - Returns: SliceDetail with all slice information
    /// - Throws: NatError if the command fails or output is invalid
    public func sliceShow(projectID: String, sliceRef: String) async throws -> SliceDetail {
        let output = try await runNat(arguments: ["slice-show", "--project", projectID, "--json", sliceRef])
        return try decodeJSON(SliceDetail.self, from: output)
    }

    /// Get the diff of a handed-back slice's branch against the base it was cut
    /// from, already split into files — or, with `commit` given, the diff of
    /// exactly one commit of that branch's history against its own parent,
    /// mirroring `nat slice-diff --commit <sha>`.
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    ///   - commit: A commit's sha, to diff it alone instead of the whole branch
    /// - Returns: SliceDiff with the base, branch, and per-file diff sections
    /// - Throws: NatError if the command fails or output is invalid
    public func sliceDiff(projectID: String, sliceRef: String, commit: String?) async throws -> SliceDiff {
        var arguments = ["slice-diff", "--project", projectID, "--json"]
        if let commit, !commit.isEmpty {
            arguments.append(contentsOf: ["--commit", commit])
        }
        arguments.append(sliceRef)
        let output = try await runNat(arguments: arguments)
        return try decodeJSON(SliceDiff.self, from: output)
    }

    /// List a handed-back slice's branch's own commits since the merge base,
    /// without diffing any of them — the sidebar's "All commits" dropdown,
    /// mirroring `nat slice-diff --commits`.
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    /// - Returns: SliceCommitsDoc with the base, branch, and the commits themselves
    /// - Throws: NatError if the command fails or output is invalid
    public func sliceCommits(projectID: String, sliceRef: String) async throws -> SliceCommitsDoc {
        let output = try await runNat(arguments: ["slice-diff", "--project", projectID, "--json", "--commits", sliceRef])
        return try decodeJSON(SliceCommitsDoc.self, from: output)
    }

    /// Send an interrupt signal to a running agent's tmux session.
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    /// - Throws: NatError if the command fails or no live session exists
    public func agentInterrupt(projectID: String, sliceRef: String) async throws {
        _ = try await runNat(arguments: ["agent-interrupt", "--project", projectID, sliceRef])
    }

    /// Send a prompt to a running agent's tmux session — the pending review
    /// comments, typed at the pane and submitted as one turn.
    ///
    /// The prompt goes over stdin (`--text -`) rather than as an argument:
    /// a review is several lines, and an argument would have to survive the
    /// shell as well as the process boundary.
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    ///   - text: The prompt to type at the agent's pane
    /// - Throws: NatError if the command fails or no live session exists
    public func agentSend(projectID: String, sliceRef: String, text: String) async throws {
        _ = try await runNatRaw(
            arguments: ["agent-send", "--project", projectID, sliceRef, "--text", "-"],
            standardInput: text.data(using: .utf8)
        )
    }

    /// Launch an agent on a slice in a tmux session.
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    ///   - model: Optional model name (e.g., "sonnet", "opus", "haiku")
    ///   - effort: Optional effort level (e.g., "low", "medium", "high")
    /// - Returns: LaunchResult with session info and optional warning
    /// - Throws: NatError if the command fails
    public func sliceLaunch(projectID: String, sliceRef: String, model: String?, effort: String?) async throws -> LaunchResult {
        var arguments = ["slice-launch", "--project", projectID, "--json"]
        if let model = model {
            arguments.append(contentsOf: ["--model", model])
        }
        if let effort = effort {
            arguments.append(contentsOf: ["--effort", effort])
        }
        arguments.append(sliceRef)
        let output = try await runNat(arguments: arguments)
        return try decodeJSON(LaunchResult.self, from: output)
    }

    /// Open a pull request for a handed-back slice's branch and record it on
    /// the slice, mirroring the board's own approve key
    /// (`internal/tui/approve.go`).
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    /// - Returns: The URL of the pull request that was opened
    /// - Throws: NatError if the slice is not handed back, or gh refuses
    public func sliceApprove(projectID: String, sliceRef: String) async throws -> String {
        let output = try await runNat(arguments: ["slice-approve", "--project", projectID, "--json", sliceRef])
        return try decodeJSON(ApproveEnvelope.self, from: output).url
    }

    /// Read one pull request in full — the PR tab's own reading, mirroring
    /// the board's `v` key (`internal/cli/prview.go`).
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    /// - Returns: PRDetail with gh's own fields
    /// - Throws: NatError if the slice has no pull request recorded, or gh fails
    public func prView(projectID: String, sliceRef: String) async throws -> PRDetail {
        let output = try await runNat(arguments: ["pr-view", "--project", projectID, "--json", sliceRef])
        return try decodeJSON(PRDetail.self, from: output)
    }

    /// Merge a slice's recorded pull request, mirroring the PR screen's own
    /// merge key (`internal/cli/prmerge.go`). The refusal is the merge box's
    /// own: gh is asked to attempt the merge only once the pull request's own
    /// verdicts allow it, which the CLI (and this client's caller) checks
    /// first — a gh that still refuses comes back as `NatError.commandFailed`
    /// with its own first line.
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    /// - Throws: NatError if the slice has no pull request recorded, or gh refuses
    public func prMerge(projectID: String, sliceRef: String) async throws {
        let output = try await runNat(arguments: ["pr-merge", "--project", projectID, "--json", sliceRef])
        _ = try decodeJSON(MergedEnvelope.self, from: output)
    }

    /// Leave a top-level comment on a slice's pull request — the PR tab's own
    /// composers, which post through `nat pr-comment` rather than write
    /// anything of their own to GitHub. GitHub's per-line review threads have
    /// a reply API of their own that `nat` does not wrap, so this is always a
    /// new comment on the conversation as a whole, whichever composer sent it.
    ///
    /// The body goes over stdin (`--body -`), exactly as `agentSend`'s prompt
    /// does: a comment is free-form text and may run to several lines.
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    ///   - body: The comment's markdown body
    /// - Throws: NatError if the slice has no pull request recorded, or gh refuses
    public func prComment(projectID: String, sliceRef: String, body: String) async throws {
        _ = try await runNatRaw(
            arguments: ["pr-comment", sliceRef, "--project", projectID, "--body", "-"],
            standardInput: body.data(using: .utf8)
        )
    }

    /// Launch the planning agent, detached in tmux, on the active project —
    /// the wand toolbar button's own action, mirroring the board's `W`
    /// (`internal/cli/workshoplaunch.go`). The model and effort are the
    /// caller's own to pass or leave nil; nothing here defaults them, since a
    /// planning launch that asks nothing takes the config's `workshop_agent`
    /// pair exactly as it stands.
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - model: Optional model name, overriding the config's workshop_agent
    ///   - effort: Optional effort level, overriding the config's workshop_agent
    /// - Returns: The launched session, its working directory, and whether it
    ///   was launched on the project's pending wishlist
    /// - Throws: NatError if a planning agent is already live, or the command fails
    public func workshopLaunch(projectID: String, model: String?, effort: String?) async throws -> WorkshopLaunchResult {
        var arguments = ["workshop-launch", "--project", projectID, "--json"]
        if let model = model, !model.isEmpty {
            arguments.append(contentsOf: ["--model", model])
        }
        if let effort = effort, !effort.isEmpty {
            arguments.append(contentsOf: ["--effort", effort])
        }
        let output = try await runNat(arguments: arguments)
        return try decodeJSON(WorkshopLaunchResult.self, from: output)
    }

    /// File one new slice under a milestone, Todo and unassigned — the plus
    /// toolbar button's own action, mirroring `internal/cli/add.go`'s
    /// `sliceAdd`.
    ///
    /// The description goes over stdin (`--description -`) when given,
    /// exactly as `agentSend`'s prompt and `prComment`'s body do: a brief may
    /// run to several lines. A nil or empty description omits the flag
    /// entirely, which is how `slice-add` itself spells "no brief".
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - title: The new slice's title
    ///   - milestone: The milestone to file it under, by name
    ///   - description: Optional brief to write on the slice page
    /// - Returns: SliceAddResult with the created slice's fields
    /// - Throws: NatError if the command fails (no such milestone, empty title, etc.)
    public func sliceAdd(projectID: String, title: String, milestone: String, description: String?) async throws -> SliceAddResult {
        var arguments = ["slice-add", title, "--project", projectID, "--milestone", milestone, "--json"]
        var standardInput: Data?
        if let description = description, !description.isEmpty {
            arguments.append(contentsOf: ["--description", "-"])
            standardInput = description.data(using: .utf8)
        }
        let output = try await runNat(arguments: arguments, standardInput: standardInput)
        return try decodeJSON(SliceAddedEnvelope.self, from: output).slice
    }

    /// Replace a Todo slice's brief outright — the Brief tab's own Edit
    /// action, mirroring `internal/cli/sliceedit.go`'s `sliceEdit`. Refused by
    /// the CLI for a slice in progress or Done; this client passes that
    /// refusal straight through as `NatError.commandFailed`.
    ///
    /// The new brief goes over stdin (`--description -`), exactly as
    /// `sliceAdd`'s does: a brief may run to several lines.
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    ///   - description: The new brief to write, replacing whatever was there
    /// - Returns: SliceEditResult with the slice and the brief that landed
    /// - Throws: NatError if the slice is not Todo, or the command fails
    public func sliceEdit(projectID: String, sliceRef: String, description: String) async throws -> SliceEditResult {
        let output = try await runNat(
            arguments: ["slice-edit", "--project", projectID, "--json", "--description", "-", sliceRef],
            standardInput: description.data(using: .utf8)
        )
        return try decodeJSON(SliceEditResult.self, from: output)
    }

    /// Read local configuration: the fields the settings scene edits and
    /// nothing else, mirroring `internal/cli/configshow.go`.
    ///
    /// - Returns: ConfigDoc with the raw stored values — zero and empty mean unset
    /// - Throws: NatError if the command fails (no configuration yet, etc.)
    public func configShow() async throws -> ConfigDoc {
        let output = try await runNat(arguments: ["config-show", "--json"])
        return try decodeJSON(ConfigDoc.self, from: output)
    }

    /// Write one local config key, mirroring `internal/cli/configset.go`. An
    /// empty value is how a field is unset, matching the settings scene's own
    /// rule that a field cleared back to empty is "unset" rather than a value
    /// to keep.
    ///
    /// - Parameters:
    ///   - key: The config key, e.g. "agent_split_percent" or "project.<id>.working_dir"
    ///   - value: The value to write, or "" to unset
    /// - Throws: NatError if the key is unknown or the value is out of bounds
    public func configSet(key: String, value: String) async throws {
        _ = try await runNat(arguments: ["config-set", key, value])
    }

    // MARK: - Private Helpers

    private func runNat(arguments: [String], standardInput: Data? = nil) async throws -> String {
        let stdout = try await runNatRaw(arguments: arguments, standardInput: standardInput)

        guard !stdout.isEmpty else {
            throw NatError.missingOutput
        }

        guard let output = String(data: stdout, encoding: .utf8) else {
            throw NatError.missingOutput
        }

        return output
    }

    /// Runs `nat` and hands back its raw stdout, without requiring any —
    /// some commands (`agent-send`) say nothing at all on success.
    private func runNatRaw(arguments: [String], standardInput: Data? = nil) async throws -> Data {
        let (stdout, stderr, exitCode) = try await commandRunner.run(
            executable: "nat",
            arguments: arguments,
            workingDirectory: workingDirectory,
            standardInput: standardInput
        )

        guard exitCode == 0 else {
            let errorMessage = extractFirstLineOfError(from: stderr)
            throw NatError.commandFailed(errorMessage)
        }

        return stdout
    }

    private func decodeJSON<T: Decodable>(_ type: T.Type, from json: String) throws -> T {
        guard let data = json.data(using: .utf8) else {
            throw NatError.invalidJSON(json, details: "Could not encode JSON as UTF-8")
        }

        let decoder = JSONDecoder()
        do {
            return try decoder.decode(T.self, from: data)
        } catch {
            throw NatError.invalidJSON(json, details: error.localizedDescription)
        }
    }

    private func extractFirstLineOfError(from stderrData: Data) -> String {
        guard let stderr = String(data: stderrData, encoding: .utf8) else {
            return "Unknown error"
        }

        let lines = stderr.split(separator: "\n", omittingEmptySubsequences: true)
        return String(lines.first ?? "Unknown error")
    }
}
