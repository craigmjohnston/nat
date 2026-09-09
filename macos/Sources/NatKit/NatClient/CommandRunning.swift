import Foundation

/// A protocol for running external commands asynchronously.
public protocol CommandRunning: Sendable {
    /// Execute a command with the given arguments.
    ///
    /// - Parameters:
    ///   - executable: The path or name of the executable to run
    ///   - arguments: Command line arguments
    ///   - workingDirectory: Optional working directory; if nil, inherits current working directory
    ///   - standardInput: Optional data to send to the command's stdin
    /// - Returns: A tuple of stdout data, stderr data, and exit code
    func run(
        executable: String,
        arguments: [String],
        workingDirectory: String?,
        standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32)
}
