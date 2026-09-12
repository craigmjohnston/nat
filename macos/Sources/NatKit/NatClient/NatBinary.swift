import Foundation

/// Which `nat` the app runs, decided explicitly rather than left to PATH.
///
/// A packaged gnat.app carries a nat built from the same checkout, beside the
/// app's own executable in `Contents/MacOS/`, so the two are never out of
/// step. Finding it by searching PATH made that match an artefact of ordering:
/// `PathBootstrap` puts the bundle's directory on the front, but a bundle
/// whose nat is missing would quietly fall through to whatever older install
/// the login shell offers, and a mismatched nat is the one thing the bundling
/// exists to prevent.
///
/// So the order is said out loud, and it is the same order for everything that
/// asks — `ProcessRunner`, which spawns nat, and `BinaryLocator`, which tells
/// onboarding whether there is one to spawn:
///
/// 1. `NAT_BIN`, the dev override, which outranks everything.
/// 2. The binary beside the app's executable, invoked by its absolute path.
/// 3. Nothing at all, for a real `.app` whose nat is missing: that is a broken
///    install to report, not a reason to run another nat.
/// 4. PATH, and only for a bare executable outside any `.app` — the dev run,
///    which sits beside no nat and never did.
///
/// `PathBootstrap`'s PATH prepend is untouched by any of this: the agent
/// sessions nat launches resolve `nat` off PATH inside tmux, and need the
/// bundled directory on the front to find the same one.
public enum NatBinary {

    /// The four answers, one per step of the order above.
    public enum Resolution: Equatable {
        /// `NAT_BIN` named it.
        case override(String)
        /// The binary the bundle carries, as an absolute path.
        case bundled(String)
        /// A real `.app` carrying no nat, and the path it should have been
        /// at — a damaged install, which is an error rather than a fallback.
        case damagedInstall(expected: String)
        /// A bare executable outside any bundle: resolve off PATH as ever.
        case searchPath
    }

    /// The resolution for this process. Every source is injected, so the order
    /// is testable without a bundle to run inside.
    ///
    /// `NAT_BIN` is taken as written and never checked for existence: the
    /// override is the dev saying which binary to run, and a fallback past an
    /// override that turned out to be missing would run something else under
    /// the name of the thing that was asked for.
    public static func resolve(
        override: String? = PathBootstrap.environmentValue("NAT_BIN"),
        executableURL: URL? = Bundle.main.executableURL,
        bundleURL: URL? = Bundle.main.bundleURL,
        isExecutableFile: (String) -> Bool = { FileManager.default.isExecutableFile(atPath: $0) }
    ) -> Resolution {
        if let override, !override.isEmpty {
            return .override(override)
        }
        guard let dir = executableURL?.deletingLastPathComponent() else {
            // No executable to sit beside is no bundle to be damaged: a
            // process that cannot say where it runs from is read as the dev
            // run it can only be.
            return .searchPath
        }
        let candidate = dir.appendingPathComponent("nat").path
        if isExecutableFile(candidate) {
            return .bundled(candidate)
        }
        return isAppBundle(bundleURL) ? .damagedInstall(expected: candidate) : .searchPath
    }

    /// Whether this process is running as a real `.app` — the bundle's own
    /// extension, which a SwiftPM executable's "bundle" (the directory the
    /// binary sits in) does not have.
    private static func isAppBundle(_ bundleURL: URL?) -> Bool {
        bundleURL?.pathExtension == "app"
    }
}
