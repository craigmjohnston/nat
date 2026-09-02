import Foundation

/// Envelope for status response.
private struct StatusEnvelope: Codable {
    let agents: [AgentStatus]
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
    /// from, already split into files.
    ///
    /// - Parameters:
    ///   - projectID: The project's Notion page ID
    ///   - sliceRef: The slice's URL or Notion page ID
    /// - Returns: SliceDiff with the base, branch, and per-file diff sections
    /// - Throws: NatError if the command fails or output is invalid
    public func sliceDiff(projectID: String, sliceRef: String) async throws -> SliceDiff {
        let output = try await runNat(arguments: ["slice-diff", "--project", projectID, "--json", sliceRef])
        return try decodeJSON(SliceDiff.self, from: output)
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

    // MARK: - Private Helpers

    private func runNat(arguments: [String]) async throws -> String {
        let (stdout, stderr, exitCode) = try await commandRunner.run(
            executable: "nat",
            arguments: arguments,
            workingDirectory: workingDirectory,
            standardInput: nil
        )

        guard exitCode == 0 else {
            let errorMessage = extractFirstLineOfError(from: stderr)
            throw NatError.commandFailed(errorMessage)
        }

        guard !stdout.isEmpty else {
            throw NatError.missingOutput
        }

        guard let output = String(data: stdout, encoding: .utf8) else {
            throw NatError.missingOutput
        }

        return output
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
