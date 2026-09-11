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

    /// The directories the tools nat spawns are actually installed to —
    /// Homebrew's two prefixes for tmux and gh, `~/.local/bin` for ntn,
    /// `~/go/bin` for a go-installed nat — taken as a floor under the
    /// composed PATH. They go last, after everything the shell and launchd
    /// said, so a real PATH entry always outranks them: they only decide
    /// for a binary found nowhere else, the same bargain BinaryLocator
    /// strikes for the onboarding checks. They are what a login shell that
    /// failed to answer (see `loginShellEnvListing`'s two-second cap) no
    /// longer costs the whole session: without them, one slow profile at
    /// launch left every nat this process ever spawned unable to find ntn.
    public static func wellKnownDirs(home: String = NSHomeDirectory()) -> [String] {
        ["/opt/homebrew/bin", "/usr/local/bin", home + "/.local/bin", home + "/go/bin"]
    }

    /// One PATH out of the four there are: the bundled nat's directory
    /// first, so the nat the app shipped with outranks any other install;
    /// then the login shell's entries; then whatever the process already
    /// had, so nothing launchd gave us is lost; then the well-known
    /// fallbacks, so a shell that said nothing still leaves the tools
    /// findable. Deduplicated in that order, and nil when every source was
    /// empty — there is nothing to set.
    public static func composed(
        bundledDir: String?, loginPath: String?, current: String?, fallbacks: [String] = []
    ) -> String? {
        var seen = Set<String>()
        var entries: [String] = []
        var parts: [String] = [bundledDir ?? ""]
        parts.append(contentsOf: (loginPath ?? "").split(separator: ":").map(String.init))
        parts.append(contentsOf: (current ?? "").split(separator: ":").map(String.init))
        parts.append(contentsOf: fallbacks)
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
    /// not run, said nothing, or was still going when the timeout ran out —
    /// two seconds at launch, since a profile that hangs must not hold the
    /// whole app's launch; the background retry waits longer, having
    /// nothing to hold. The read is
    /// waited on rather than the exit: end of output is the child closing
    /// its pipe, which its exit does, and a listing bigger than the pipe
    /// (64KB) never deadlocks against a wait for a child blocked writing it.
    public static func loginShellEnvListing(shell: String, timeout: TimeInterval = 2) -> String? {
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
        if eof.wait(timeout: .now() + timeout) == .timedOut {
            process.terminate()
            return nil
        }
        return String(data: box.data, encoding: .utf8)
    }

    /// The retry's default home: a global queue, the work boxed the way
    /// `loginShellEnvListing` boxes its read, since it is made of injected
    /// closures the compiler cannot see are safe to send. Public only
    /// because a public default argument may name nothing less so.
    public static func runInBackground(_ work: @escaping () -> Void) {
        final class Box: @unchecked Sendable {
            let work: () -> Void
            init(_ work: @escaping () -> Void) { self.work = work }
        }
        let box = Box(work)
        DispatchQueue.global(qos: .utility).async { box.work() }
    }

    /// The once-at-startup entry point: compose the real PATH and set it,
    /// so every child this process spawns — nat, tmux, and through nat's
    /// launch the agents themselves — inherits it. Every piece is
    /// injectable and the pieces are tested; this is only their order. A
    /// composition with nothing to say sets nothing, which leaves the
    /// environment exactly as launchd handed it over.
    ///
    /// A shell that missed its two-second window gets one patient second
    /// attempt, off the launch path: children inherit the live environ at
    /// spawn time, so a retry that lands re-composes and re-applies for
    /// everything spawned after it — the floor covers the seconds between.
    /// One attempt and not a loop, because the miss this recovers is
    /// launch-time load, and a shell that cannot answer an unhurried
    /// fifteen seconds is not one a third try would hear from. Only the
    /// shell's silence retries: with the listing in hand the first
    /// composition already said everything there is to say.
    public static func bootstrap(
        bundledDir: String? = bundledNatDir(),
        shell: String? = environmentValue("SHELL"),
        current: String? = environmentValue("PATH"),
        fallbacks: [String] = wellKnownDirs(),
        loginListing: (String) -> String? = { loginShellEnvListing(shell: $0) },
        retryListing: @escaping (String) -> String? = { loginShellEnvListing(shell: $0, timeout: 15) },
        inBackground: (@escaping () -> Void) -> Void = runInBackground,
        apply: @escaping (String) -> Void = { setenv("PATH", $0, 1) }
    ) {
        let listing = shell.flatMap(loginListing)
        let login = listing.flatMap(loginPath(fromEnvListing:))
        if let path = composed(
            bundledDir: bundledDir, loginPath: login, current: current, fallbacks: fallbacks
        ) {
            apply(path)
        }
        // A listing that arrived without a PATH line is the shell's answer,
        // and asking again would only hear it again — only no listing at
        // all is worth the second ask.
        guard listing == nil, let shell else { return }
        inBackground {
            guard
                let late = retryListing(shell).flatMap(loginPath(fromEnvListing:)),
                // The original `current`, not a re-read: the first apply
                // wrote the composition into the environment, and composing
                // over one's own output would only re-say it — the sources
                // are the same either way, and this stays one deduplication
                // over them in the documented order.
                let path = composed(
                    bundledDir: bundledDir, loginPath: late, current: current, fallbacks: fallbacks
                )
            else { return }
            apply(path)
        }
    }
}
