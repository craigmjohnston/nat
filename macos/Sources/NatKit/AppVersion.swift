import Foundation

/// gnat's own version and build, as Settings ▸ About shows them: read off
/// an Info.plist's `CFBundleShortVersionString` and `CFBundleVersion`
/// (`make-app.sh` writes both), each `dev` where it is unset or empty — as
/// it is for a bare dev executable, which has no Info.plist at all.
public struct AppVersion: Equatable, Sendable {
    public static let unset = "dev"

    public let version: String
    public let build: String

    public init(infoDictionary: [String: Any]?) {
        version = Self.value(infoDictionary?["CFBundleShortVersionString"])
        build = Self.value(infoDictionary?["CFBundleVersion"])
    }

    /// "Version 1.4.0 (212)", the build dropped where it would only say the
    /// version again — a dev build's "Version dev", not "Version dev (dev)".
    public var label: String {
        build == version ? "Version \(version)" : "Version \(version) (\(build))"
    }

    private static func value(_ raw: Any?) -> String {
        guard let text = (raw as? String)?.trimmingCharacters(in: .whitespaces), !text.isEmpty else { return unset }
        return text
    }
}
