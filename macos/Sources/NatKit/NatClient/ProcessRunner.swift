import Foundation

/// The real implementation of CommandRunning using Foundation.Process.
public final class ProcessRunner: CommandRunning {
    public init() {}

    public func run(
        executable: String,
        arguments: [String],
        workingDirectory: String?,
        standardInput: Data?
    ) async throws -> (stdout: Data, stderr: Data, exitCode: Int32) {
        let process = Process()

        // Resolve the executable path by searching PATH if it's not absolute
        let executablePath = try resolveExecutable(executable)
        process.executableURL = URL(fileURLWithPath: executablePath)

        process.arguments = arguments

        if let workingDirectory = workingDirectory {
            process.currentDirectoryURL = URL(fileURLWithPath: workingDirectory)
        }

        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()
        let stdinPipe = Pipe()

        process.standardOutput = stdoutPipe
        process.standardError = stderrPipe
        process.standardInput = stdinPipe

        try process.run()

        // Write stdin if provided
        if let standardInput = standardInput {
            stdinPipe.fileHandleForWriting.write(standardInput)
            try stdinPipe.fileHandleForWriting.close()
        } else {
            try stdinPipe.fileHandleForWriting.close()
        }

        process.waitUntilExit()

        let stdoutData = stdoutPipe.fileHandleForReading.readDataToEndOfFile()
        let stderrData = stderrPipe.fileHandleForReading.readDataToEndOfFile()

        return (stdoutData, stderrData, process.terminationStatus)
    }

    private func resolveExecutable(_ executable: String) throws -> String {
        // If it's an absolute path, use it directly
        if executable.hasPrefix("/") {
            return executable
        }

        // Check NAT_BIN environment variable
        if let natBin = ProcessInfo.processInfo.environment["NAT_BIN"] {
            return natBin
        }

        // Search PATH for the executable
        if let pathEnv = ProcessInfo.processInfo.environment["PATH"] {
            let pathDirs = pathEnv.split(separator: ":").map(String.init)
            for dir in pathDirs {
                let fullPath = (dir as NSString).appendingPathComponent(executable)
                if FileManager.default.fileExists(atPath: fullPath) {
                    return fullPath
                }
            }
        }

        // If not found in PATH, assume it's available in the current environment
        return executable
    }
}
