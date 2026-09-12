import Foundation

/// The real implementation of CommandRunning using Foundation.Process.
public final class ProcessRunner: CommandRunning {
    /// Which `nat` to run, asked afresh per spawn: `NAT_BIN` can be set after
    /// launch, and a seam here is what lets the resolution be driven from a
    /// test without a bundle to run inside.
    private let natResolution: @Sendable () -> NatBinary.Resolution

    public init(
        natResolution: @escaping @Sendable () -> NatBinary.Resolution = { NatBinary.resolve() }
    ) {
        self.natResolution = natResolution
    }

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

        // Both pipes are drained while the child runs, never after it: a pipe
        // holds 64KB, and a child with more to say than that blocks writing
        // it, so a parent that waited first would deadlock against exactly
        // the outputs worth reading (`nat info --json` is one).
        async let stdoutData = drain(stdoutPipe)
        async let stderrData = drain(stderrPipe)

        let exitCode: Int32 = try await withCheckedThrowingContinuation { continuation in
            process.terminationHandler = { continuation.resume(returning: $0.terminationStatus) }
            do {
                try process.run()
            } catch {
                process.terminationHandler = nil
                // A process that never ran writes nothing and closes
                // nothing: the drains above are already blocked reading
                // pipes whose write ends only a launch would have handed
                // over, and leaving this scope awaits them (an abandoned
                // async let is awaited, not abandoned) — so the write ends
                // are closed here, which is the EOF that lets the drains
                // finish and the error actually surface. Without it, a
                // machine with no nat at all hangs instead of erroring.
                try? stdoutPipe.fileHandleForWriting.close()
                try? stderrPipe.fileHandleForWriting.close()
                continuation.resume(throwing: error)
                return
            }
            if let standardInput = standardInput {
                stdinPipe.fileHandleForWriting.write(standardInput)
            }
            try? stdinPipe.fileHandleForWriting.close()
        }

        return (await stdoutData, await stderrData, exitCode)
    }

    /// drain reads a pipe to its end off the cooperative pool, so an async
    /// caller neither blocks a pool thread nor races the child's writes.
    private func drain(_ pipe: Pipe) async -> Data {
        await withCheckedContinuation { continuation in
            DispatchQueue.global(qos: .userInitiated).async {
                continuation.resume(returning: pipe.fileHandleForReading.readDataToEndOfFile())
            }
        }
    }

    private func resolveExecutable(_ executable: String) throws -> String {
        // If it's an absolute path, use it directly
        if executable.hasPrefix("/") {
            return executable
        }

        // `nat` is resolved rather than searched for — NAT_BIN, then the
        // binary the bundle carries, by absolute path — so a packaged app
        // runs the nat it was built with whatever PATH holds, and says its
        // install is damaged rather than running another. See `NatBinary`.
        // Only nat: the override and the bundle are about nat alone, and must
        // not hijack every other tool this runner spawns.
        if executable == "nat" {
            switch natResolution() {
            case .override(let path), .bundled(let path):
                return path
            case .damagedInstall(let expected):
                throw NatError.bundledBinaryMissing(expected)
            case .searchPath:
                break
            }
        }

        // Search PATH for the executable — read live through getenv rather
        // than ProcessInfo's snapshot, so PathBootstrap's setenv (the
        // bundled nat's directory, the login shell's entries) is seen here
        // whenever it ran.
        if let pathEnv = PathBootstrap.environmentValue("PATH") {
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
