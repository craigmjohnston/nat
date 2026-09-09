import Foundation

/// The PATH a Finder-launched app actually needs, resolved once at startup.
///
/// An app launched from the Finder or the Dock inherits launchd's PATH —
/// `/usr/bin:/bin:/usr/sbin:/sbin` — which has neither Homebrew nor a Go
/// install on it, so every `nat`, `tmux` or `gh` the app spawns would be
/// resolved against directories none of them are in. The fix is the one
/// editors use: ask the user's own login shell what PATH it would give a
/// terminal, and take that. The app's bundled nat — carried inside gnat.app
/// so nobody go-installs the same tool twice — goes on the front, so the nat
/// the app was built with is the nat everything runs, the agent sessions
/// included (nat's launch carries PATH into tmux with `-e`).
///
/// `bootstrap()` runs first thing at startup, before anything reads the
/// environment or spawns a child: children inherit the process's real
/// environ, but `ProcessInfo` snapshots it, so a setenv made after a read
/// through that is a PATH some readers never see — which is also why
/// everything here, and every PATH lookup that must see the bootstrap's
/// work, reads through `environmentValue` (getenv, the live truth) rather
/// than `ProcessInfo`.
public enum PathBootstrap {

    /// The current value of an environment variable, read live through
    /// getenv rather than through `ProcessInfo`'s snapshot.
    public static func environmentValue(_ name: String) -> String? {
        getenv(name).map { String(cString: $0) }
    }

    /// The PATH in a login shell's environment listing. The shell is run
    /// `-l -c '/usr/bin/env'` — env's own output rather than an echoed
    /// `$PATH`, since fish would print a list's entries space-separated —
    /// and the last `PATH=` line wins: env runs after every profile, so its
    /// line is the final one, and a profile that happens to print something
    /// PATH-shaped is overtaken rather than believed.
    public static func loginPath(fromEnvListing listing: String) -> String? {
        let line = listing
            .split(separator: "\n", omittingEmptySubsequences: true)
            .last(where: { $0.hasPrefix("PATH=") })
        guard let line else { return nil }
        let value = String(line.dropFirst("PATH=".count))
        return value.isEmpty ? nil : value
    }

    /// One PATH out of the three there are: the bundled nat's directory
    /// first, so the nat the app shipped with outranks any other install;
    /// then the login shell's entries; then whatever the process already
    /// had, so nothing launchd gave us is lost. Deduplicated in that order,
    /// and nil when every source was empty — there is nothing to set.
    public static func composed(bundledDir: String?, loginPath: String?, current: String?) -> String? {
        var seen = Set<String>()
        var entries: [String] = []
        var parts: [String] = [bundledDir ?? ""]
        parts.append(contentsOf: (loginPath ?? "").split(separator: ":").map(String.init))
        parts.append(contentsOf: (current ?? "").split(separator: ":").map(String.init))
        for part in parts where !part.isEmpty && seen.insert(part).inserted {
            entries.append(part)
        }
        return entries.isEmpty ? nil : entries.joined(separator: ":")
    }

    /// The directory holding the app's own nat, or nil for a build that
    /// carries none: the dev run's bare executable sits beside no nat, and
    /// prepending its directory would say the bundle offers what it does
    /// not. `NAT_BIN` still outranks the bundle either way — ProcessRunner
    /// answers it before any PATH search.
    public static func bundledNatDir(
        executableURL: URL? = Bundle.main.executableURL,
        isExecutableFile: (String) -> Bool = { FileManager.default.isExecutableFile(atPath: $0) }
    ) -> String? {
        guard let dir = executableURL?.deletingLastPathComponent() else { return nil }
        return isExecutableFile(dir.appendingPathComponent("nat").path) ? dir.path : nil
    }

    /// The login shell's environment listing, or nil for a shell that could
    /// not run, said nothing, or was still going after two seconds — a
    /// profile that hangs must not hold the whole app's launch. The read is
    /// waited on rather than the exit: end of output is the child closing
    /// its pipe, which its exit does, and a listing bigger than the pipe
    /// (64KB) never deadlocks against a wait for a child blocked writing it.
    public static func loginShellEnvListing(shell: String) -> String? {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: shell)
        process.arguments = ["-l", "-c", "/usr/bin/env"]
        let stdout = Pipe()
        process.standardOutput = stdout
        // Profile noise goes to stderr in the best of homes; drop it.
        process.standardError = Pipe()
        do { try process.run() } catch { return nil }

        final class Box: @unchecked Sendable { var data = Data() }
        let box = Box()
        let eof = DispatchSemaphore(value: 0)
        DispatchQueue.global(qos: .userInitiated).async {
            box.data = stdout.fileHandleForReading.readDataToEndOfFile()
            eof.signal()
        }
        if eof.wait(timeout: .now() + 2) == .timedOut {
            process.terminate()
            return nil
        }
        return String(data: box.data, encoding: .utf8)
    }

    /// The once-at-startup entry point: compose the real PATH and set it,
    /// so every child this process spawns — nat, tmux, and through nat's
    /// launch the agents themselves — inherits it. Every piece is
    /// injectable and the pieces are tested; this is only their order. A
    /// composition with nothing to say sets nothing, which leaves the
    /// environment exactly as launchd handed it over.
    public static func bootstrap(
        bundledDir: String? = bundledNatDir(),
        shell: String? = environmentValue("SHELL"),
        current: String? = environmentValue("PATH"),
        loginListing: (String) -> String? = loginShellEnvListing,
        apply: (String) -> Void = { setenv("PATH", $0, 1) }
    ) {
        let login = shell.flatMap(loginListing).flatMap(loginPath(fromEnvListing:))
        guard let path = composed(bundledDir: bundledDir, loginPath: login, current: current) else { return }
        apply(path)
    }
}
