import Foundation

/// Result from `nat slice-launch`, containing session info and optional warning.
public struct LaunchResult: Codable, Equatable, Sendable {
    public let session: String
    public let workdir: String
    public let branch: String
    public let warning: String?

    enum CodingKeys: String, CodingKey {
        case session
        case workdir
        case branch
        case warning
    }

    public init(
        session: String,
        workdir: String,
        branch: String,
        warning: String? = nil
    ) {
        self.session = session
        self.workdir = workdir
        self.branch = branch
        self.warning = warning
    }
}
