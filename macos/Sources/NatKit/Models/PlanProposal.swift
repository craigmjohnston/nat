import Foundation

/// The plan a workshop session proposed for an Untitled tab — what
/// `nat plan-proposal --json` reads back from the file `nat plan-propose`
/// wrote. Held as the tree the rail draws: milestones in plan order, each with
/// its slices' titles. Nothing here is a slice yet; accepting is what files
/// them (`nat plan-accept`).
public struct PlanProposal: Equatable, Sendable, Decodable {
    /// One proposed milestone and the titles of the slices filed under it.
    public struct Milestone: Equatable, Sendable {
        public let name: String
        public let slices: [String]

        public init(name: String, slices: [String]) {
            self.name = name
            self.slices = slices
        }
    }

    /// The project name the planning agent suggested.
    public let name: String
    public let milestones: [Milestone]

    public init(name: String, milestones: [Milestone]) {
        self.name = name
        self.milestones = milestones
    }

    public var milestoneCount: Int { milestones.count }
    public var sliceCount: Int { milestones.reduce(0) { $0 + $1.slices.count } }

    /// The tree as the rail's own folders, so the existing folder and slice
    /// rows draw it: every slice Todo, no milestone current, nothing done.
    public var folders: [MilestoneFolder] {
        milestones.enumerated().map { index, milestone in
            MilestoneFolder(
                milestoneID: milestone.name,
                title: milestone.name,
                done: 0,
                total: milestone.slices.count,
                isCurrent: false,
                slices: milestone.slices.enumerated().map { sliceIndex, title in
                    MilestoneSliceRow(
                        sliceID: "proposed-\(index)-\(sliceIndex)", name: title, glyph: .todo, isBlocked: false)
                }
            )
        }
    }

    // The wire shape is `plan-propose`'s proposal file: the plan document as
    // validated, slices naming their milestone. Grouping them under it is all
    // this does — validation already ran when the file was written.
    private enum CodingKeys: String, CodingKey { case name, plan }
    private struct Plan: Decodable {
        struct Named: Decodable { let name: String }
        struct Slice: Decodable {
            let title: String
            let milestone: String
        }
        let milestones: [Named]?
        let slices: [Slice]?
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        name = try container.decode(String.self, forKey: .name)
        let plan = try container.decode(Plan.self, forKey: .plan)
        let slices = plan.slices ?? []
        milestones = (plan.milestones ?? []).map { milestone in
            let key = Self.key(milestone.name)
            return Milestone(
                name: milestone.name.trimmingCharacters(in: .whitespacesAndNewlines),
                slices: slices.filter { Self.key($0.milestone) == key }
                    .map { $0.title.trimmingCharacters(in: .whitespacesAndNewlines) }
            )
        }
    }

    /// How the CLI matches a slice to its milestone: trimmed, case-folded.
    private static func key(_ name: String) -> String {
        name.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }
}

/// What `nat plan-accept --json` reports: the project it made and how much of
/// the plan went into it.
public struct PlanAccepted: Equatable, Sendable, Decodable {
    public let project: ProjectEntry
    public let milestones: Int
    public let slices: Int

    public init(project: ProjectEntry, milestones: Int, slices: Int) {
        self.project = project
        self.milestones = milestones
        self.slices = slices
    }
}

/// The words of the proposal's rail section and the accepted pane, written once
/// — the mock is `NFRail`/`NFShell` in `docs/design/nat-new-project/ui-npflow.jsx`.
public enum ProposalText {
    public static let heading = "PROPOSED"
    public static let nameCaption = "Project name — suggested by the planning agent"
    public static let acceptLabel = "Accept plan"
    public static let keepLabel = "Keep workshopping"
    public static let emptyNameError = "Give the project a name to accept the plan."
    public static let acceptedTitle = "Plan accepted"

    /// "4 milestones · 14 slices", each pluralised on its own.
    public static func counts(milestones: Int, slices: Int) -> String {
        "\(milestones) \(milestones == 1 ? "milestone" : "milestones") · \(slices) \(slices == 1 ? "slice" : "slices")"
    }

    /// The caption under the buttons, tracking the name field.
    public static func acceptCaption(name: String) -> String {
        "Accepting writes the plan to local storage as “\(name)”."
    }

    public static func acceptedSubtitle(milestones: Int, slices: Int) -> String {
        "\(counts(milestones: milestones, slices: slices)) written locally. Select a slice to begin."
    }
}
