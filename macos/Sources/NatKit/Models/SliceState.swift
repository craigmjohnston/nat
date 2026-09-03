import Foundation

/// The state of a slice in flight: where it has got to in its workflow.
public enum SliceState: Codable, Equatable {
    case working
    case waiting
    case blocked
    case readyToPush
    case awaitingReview
    case readyToMerge
    /// A state string that was not recognized. Stores the original string for debugging.
    case unknown(String)

    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let value = try container.decode(String.self)

        switch value {
        case "working":
            self = .working
        case "waiting":
            self = .waiting
        case "blocked":
            self = .blocked
        case "ready to push":
            self = .readyToPush
        case "awaiting review":
            self = .awaitingReview
        case "ready to merge":
            self = .readyToMerge
        default:
            self = .unknown(value)
        }
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .working:
            try container.encode("working")
        case .waiting:
            try container.encode("waiting")
        case .blocked:
            try container.encode("blocked")
        case .readyToPush:
            try container.encode("ready to push")
        case .awaitingReview:
            try container.encode("awaiting review")
        case .readyToMerge:
            try container.encode("ready to merge")
        case .unknown(let value):
            try container.encode(value)
        }
    }
}

extension SliceState: Hashable {
    public func hash(into hasher: inout Hasher) {
        switch self {
        case .working:
            hasher.combine("working")
        case .waiting:
            hasher.combine("waiting")
        case .blocked:
            hasher.combine("blocked")
        case .readyToPush:
            hasher.combine("ready to push")
        case .awaitingReview:
            hasher.combine("awaiting review")
        case .readyToMerge:
            hasher.combine("ready to merge")
        case .unknown(let value):
            hasher.combine(value)
        }
    }
}
