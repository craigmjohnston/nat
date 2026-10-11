import Foundation
import NatKit

extension Fixtures {
    /// October on a Pro plan: three projects holding storage in their
    /// colours, one holding none, a source project (no colour) and two
    /// repositories no project claims.
    public static let storageUsage = StorageUsage(
        login: "craigmjohnston", plan: "pro", allowanceGB: 1, year: 2026, month: 10, daysLeft: 22,
        totalGB: 0.71,
        projects: [
            .init(id: projectID, name: "notion-agent-tracker", color: "blue",
                  repos: ["craigmjohnston/nat"], gb: 0.38),
            .init(id: secondProjectID, name: "game-simple-brewery", color: "orange",
                  repos: ["craigmjohnston/game-simple-brewery"], gb: 0.14),
            .init(id: "f1x70000-0000-4000-8000-000000000005", name: "Shortcut", color: nil,
                  repos: ["craigmjohnston/cards"], gb: 0.06),
            .init(id: "f1x70000-0000-4000-8000-000000000006", name: "dotfiles", color: "green",
                  repos: ["craigmjohnston/dotfiles"], gb: 0),
        ],
        other: .init(gb: 0.13, repos: [
            .init(repo: "craigmjohnston/old-site", gb: 0.09),
            .init(repo: "craigmjohnston/scratchpad", gb: 0.04),
        ]))

    /// What nat answers where gh is signed in without the "user" scope.
    public static let storageNeedsScope = StorageUsageAnswer.needsScope(
        command: "gh auth refresh -h github.com -s user")

    /// What nat says where gh is not signed in at all.
    public static let storageUsageRefusal =
        "read GitHub's artifact storage: To get started with GitHub CLI, please run:  gh auth login"
}
