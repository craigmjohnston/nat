import Foundation

/// Result from `nat workshop-launch`: the planning agent's session and the
/// working directory it started in.
public struct WorkshopLaunchResult: Codable, Equatable, Sendable {
    public let session: String
    public let workdir: String

    enum CodingKeys: String, CodingKey {
        case session
        case workdir
    }

    public init(session: String, workdir: String) {
        self.session = session
        self.workdir = workdir
    }
}
