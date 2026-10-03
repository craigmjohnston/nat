import Foundation

/// The plan a workshop session proposed — for an Untitled tab or a tracked
/// project's own workshop — what `nat plan-proposal --json` reads back from
/// the file `nat plan-propose` wrote. Held as the tree the workshop's Plan
/// section draws: milestones in plan order, each with its slices' titles.
/// A project's proposal may file slices under milestones the project already
/// has; those show as groups too, after the new ones, since a slice headed
/// for an existing milestone is still proposed work. Nothing here is a slice
/// yet; accepting is what files them (`nat plan-accept`).
public struct PlanProposal: Equatable, Sendable, Decodable {
    /// One milestone of the tree and the titles of the slices filed under
    /// it. `isNew` says the proposal creates the milestone itself; false is
    /// one the project already has, there only to hold its proposed slices.
    public struct Milestone: Equatable, Sendable {
        public let name: String
        public let slices: [String]
        public let isNew: Bool

        public init(name: String, slices: [String], isNew: Bool = true) {
            self.name = name
            self.slices = slices
            self.isNew = isNew
        }
    }

    /// The project name the planning agent suggested.
    public let name: String
    public let milestones: [Milestone]

    public init(name: String, milestones: [Milestone]) {
        self.name = name
        self.milestones = milestones
    }

    /// How many milestones accepting creates — an existing one a slice is
    /// filed under is not among them.
    public var milestoneCount: Int { milestones.filter(\.isNew).count }
    public var sliceCount: Int { milestones.reduce(0) { $0 + $1.slices.count } }

    /// The tree as milestone folders, as the workshop's Plan section draws
    /// it: every slice Todo, no milestone current, nothing done.
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
    // this does — validation already ran when the file was written. A
    // milestone a slice names that the document does not create is one the
    // project already has (plan-propose --project validated it against the
    // project's shape), and gets a group of its own after the new ones, in
    // the order the slices first name it.
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
        // A project's own workshop proposes into a project that already has
        // a name, and may suggest none.
        name = try container.decodeIfPresent(String.self, forKey: .name) ?? ""
        let plan = try container.decode(Plan.self, forKey: .plan)
        let slices = plan.slices ?? []
        func titles(under key: String) -> [String] {
            slices.filter { Self.key($0.milestone) == key }
                .map { $0.title.trimmingCharacters(in: .whitespacesAndNewlines) }
        }
        let new = (plan.milestones ?? []).map { milestone in
            Milestone(
                name: milestone.name.trimmingCharacters(in: .whitespacesAndNewlines),
                slices: titles(under: Self.key(milestone.name)))
        }
        var seen = Set(new.map { Self.key($0.name) })
        var existing: [Milestone] = []
        for slice in slices where seen.insert(Self.key(slice.milestone)).inserted {
            existing.append(Milestone(
                name: slice.milestone.trimmingCharacters(in: .whitespacesAndNewlines),
                slices: titles(under: Self.key(slice.milestone)),
                isNew: false))
        }
        milestones = new + existing
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
    public static let nameCaption = "Project name — suggested by the planning agent"
    public static let acceptLabel = "Accept plan"
    public static let keepLabel = "Keep workshopping"
    public static let emptyNameError = "Give the project a name to accept the plan."
    public static let acceptedTitle = "Plan accepted"

    /// "4 milestones · 14 slices", each pluralised on its own.
    public static func counts(milestones: Int, slices: Int) -> String {
        "\(milestones) \(milestones == 1 ? "milestone" : "milestones") · \(slices) \(slices == 1 ? "task" : "tasks")"
    }

    /// The caption under the buttons, tracking the name field.
    public static func acceptCaption(name: String) -> String {
        "Accepting writes the plan to local storage as “\(name)”."
    }

    /// The same caption on a project's own workshop, which has its name.
    public static func projectAcceptCaption(project: String) -> String {
        "Accepting files these milestones and tasks into “\(project)”."
    }

    public static func acceptedSubtitle(milestones: Int, slices: Int) -> String {
        "\(counts(milestones: milestones, slices: slices)) written locally. Select a task to begin."
    }
}
