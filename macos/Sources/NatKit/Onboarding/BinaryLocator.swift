import Foundation

/// Finds a command-line binary on PATH — the same resolution `AttachSpec`'s
/// `resolvedExecutable` uses for tmux, generalised to any of the binaries the
/// onboarding pane checks for (`nat`, `tmux`, `gh`, `ntn`). An app launched
/// from the Finder carries a PATH with no Homebrew on it, so the fallback
/// locations a Mac actually installs these to are checked too.
///
/// `nat` is the exception, and does not go near PATH in a packaged app:
/// `NatBinary` is what decides which nat the app runs, and this check reads
/// that very resolution, so the onboarding verdict and the runtime behaviour
/// can never disagree — a bundle carrying no nat reads as the damaged install
/// it is rather than as a found binary somewhere else on PATH.
public enum BinaryLocator {
    /// Where a Homebrew or system install puts a binary, checked after PATH
    /// itself comes up empty.
    private static let fallbackDirectories = ["/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"]

    /// What the onboarding checklist has to say about one binary: found,
    /// missing, or — for `nat` in a packaged app with none beside it — an
    /// install to repair, which is a different sentence from "install nat".
    public enum Status: Equatable, Sendable {
        case found(String)
        case missing
        case damagedInstall(expected: String)

        /// Whether there is a binary to run — the checklist's tick.
        public var isFound: Bool {
            if case .found = self { return true }
            return false
        }
    }

    /// The status of `binary`, which for `nat` is `NatBinary`'s resolution and
    /// for everything else is whether PATH or a fallback holds it.
    public static func status(
        of binary: String,
        environment: [String: String] = ["PATH": PathBootstrap.environmentValue("PATH") ?? ""],
        fileExists: (String) -> Bool = { FileManager.default.isExecutableFile(atPath: $0) },
        natResolution: () -> NatBinary.Resolution = { NatBinary.resolve() }
    ) -> Status {
        if binary == "nat" {
            switch natResolution() {
            // An override is taken as written here exactly as it is where nat
            // is spawned: NAT_BIN outranks everything, and a verdict that
            // second-guessed it would be answering for a different binary
            // than the one that will run.
            case .override(let path), .bundled(let path):
                return .found(path)
            case .damagedInstall(let expected):
                return .damagedInstall(expected: expected)
            case .searchPath:
                break
            }
        }
        guard let path = searchPath(for: binary, environment: environment, fileExists: fileExists)
        else { return .missing }
        return .found(path)
    }

    /// The absolute path to `binary`, or nil if there is none to run: for
    /// `nat`, whatever `NatBinary` resolved; for everything else, PATH's own
    /// entries first and then the fallback locations, so an override on PATH
    /// always wins. The default environment reads PATH live through getenv
    /// rather than ProcessInfo's snapshot, so PathBootstrap's setenv is seen —
    /// the onboarding check must answer for the same PATH the spawns will
    /// use.
    public static func resolvedPath(
        for binary: String,
        environment: [String: String] = ["PATH": PathBootstrap.environmentValue("PATH") ?? ""],
        fileExists: (String) -> Bool = { FileManager.default.isExecutableFile(atPath: $0) },
        natResolution: () -> NatBinary.Resolution = { NatBinary.resolve() }
    ) -> String? {
        switch status(
            of: binary, environment: environment, fileExists: fileExists,
            natResolution: natResolution
        ) {
        case .found(let path): return path
        case .missing, .damagedInstall: return nil
        }
    }

    /// PATH's own entries and then the fallback locations — the resolution
    /// every binary but `nat` is found by.
    private static func searchPath(
        for binary: String,
        environment: [String: String],
        fileExists: (String) -> Bool
    ) -> String? {
        let fromPath = (environment["PATH"] ?? "")
            .split(separator: ":")
            .map { "\($0)/\(binary)" }
        let fallbacks = fallbackDirectories.map { "\($0)/\(binary)" }
        return (fromPath + fallbacks).first(where: fileExists)
    }

    /// Whether `binary` can be found at all — the onboarding checklist's own
    /// question, which does not need to know where.
    public static func isFound(
        _ binary: String,
        environment: [String: String] = ["PATH": PathBootstrap.environmentValue("PATH") ?? ""],
        fileExists: (String) -> Bool = { FileManager.default.isExecutableFile(atPath: $0) },
        natResolution: () -> NatBinary.Resolution = { NatBinary.resolve() }
    ) -> Bool {
        resolvedPath(
            for: binary, environment: environment, fileExists: fileExists,
            natResolution: natResolution
        ) != nil
    }
}
