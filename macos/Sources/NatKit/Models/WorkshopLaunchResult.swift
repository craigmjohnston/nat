import Foundation

/// Result from `nat workshop-launch`: the planning agent's session, the
/// working directory it started in, and whether it was launched on the
/// project's pending wishlist rather than a plain planning prompt.
public struct WorkshopLaunchResult: Codable, Equatable, Sendable {
    public let session: String
    public let workdir: String
    public let wishlist: Bool

    enum CodingKeys: String, CodingKey {
        case session
        case workdir
        case wishlist
    }

    public init(session: String, workdir: String, wishlist: Bool) {
        self.session = session
        self.workdir = workdir
        self.wishlist = wishlist
    }
}
