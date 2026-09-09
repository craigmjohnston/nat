import Foundation

/// The state of a load operation.
public enum LoadState: Equatable {
    case idle
    case loading
    case loaded(ProjectInfo)
    case failed(String, previous: ProjectInfo?)

    public var projectInfo: ProjectInfo? {
        switch self {
        case .loaded(let info):
            return info
        case .failed(_, let previous):
            return previous
        case .idle, .loading:
            return nil
        }
    }

    public var errorMessage: String? {
        switch self {
        case .failed(let message, _):
            return message
        case .idle, .loading, .loaded:
            return nil
        }
    }

    public var isLoading: Bool {
        switch self {
        case .loading:
            return true
        case .idle, .loaded, .failed:
            return false
        }
    }
}
