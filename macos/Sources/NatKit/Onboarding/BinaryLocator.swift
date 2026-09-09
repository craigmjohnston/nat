import Foundation

/// Finds a command-line binary on PATH — the same resolution `AttachSpec`'s
/// `resolvedExecutable` uses for tmux, generalised to any of the binaries the
/// onboarding pane checks for (`nat`, `tmux`, `gh`, `ntn`). An app launched
/// from the Finder carries a PATH with no Homebrew on it, so the fallback
/// locations a Mac actually installs these to are checked too.
public enum BinaryLocator {
    /// Where a Homebrew or system install puts a binary, checked after PATH
    /// itself comes up empty.
    private static let fallbackDirectories = ["/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"]

    /// The absolute path to `binary`, or nil if it was found nowhere: PATH's
    /// own entries first, then the fallback locations, so an override on
    /// PATH always wins. The default environment reads PATH live through
    /// getenv rather than ProcessInfo's snapshot, so PathBootstrap's setenv
    /// is seen — the onboarding check must answer for the same PATH the
    /// spawns will use.
    public static func resolvedPath(
        for binary: String,
        environment: [String: String] = ["PATH": PathBootstrap.environmentValue("PATH") ?? ""],
        fileExists: (String) -> Bool = { FileManager.default.isExecutableFile(atPath: $0) }
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
        fileExists: (String) -> Bool = { FileManager.default.isExecutableFile(atPath: $0) }
    ) -> Bool {
        resolvedPath(for: binary, environment: environment, fileExists: fileExists) != nil
    }
}
