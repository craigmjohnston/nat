import Foundation
import NatKit

/// A `PlanCaching` that remembers nothing — so a fixture board never reads a
/// real project's cached plan off Application Support, and never writes the
/// fixture plan over one.
public struct NullPlanCache: PlanCaching {
    public init() {}
    public func read(projectID: String) async -> ProjectInfo? { nil }
    public func write(_ info: ProjectInfo, projectID: String) async {}
}

/// The fixture config, handed back without touching the disk.
public struct FixtureConfigReader: ConfigReaderProtocol {
    private let config: NatProjectConfig

    public init(config: NatProjectConfig = Fixtures.config) {
        self.config = config
    }

    public func readConfig(from path: String) async throws -> NatProjectConfig {
        config
    }
}

extension Fixtures {
    /// A whole app over the fixtures: every store it makes reads the canned
    /// client, its config comes from memory and its plan cache remembers
    /// nothing, so nothing here spawns a `nat`, reads a real project's cache
    /// or reaches tmux.
    ///
    /// It comes back unstarted, since starting is `async` and a `#Preview`
    /// builds its value synchronously — `start(_:)` below is what a preview's
    /// own `.task` calls to fill it in.
    @MainActor
    public static func appModel(
        client: FixtureNatClient = FixtureNatClient(),
        config: NatProjectConfig = Fixtures.config
    ) -> AppModel {
        AppModel(
            configReader: FixtureConfigReader(config: config),
            planCache: NullPlanCache(),
            // Far longer than any preview or test lives, so the poll never
            // fires under one; the fixtures do not change, so a poll would
            // only be work nobody reads.
            pollIntervalSeconds: 3600,
            pathsProvider: { Fixtures.paths },
            workshopLauncher: { projectID, model, effort, request in
                try await client.workshopLaunch(
                    projectID: projectID, model: model, effort: effort, request: request)
            },
            clientFactory: { client },
            // The live clock rather than the pinned one, alone among the
            // fixtures: this clock stamps `firstSeen`, and the rail draws an
            // agent's elapsed time by measuring that against the clock the
            // Mac is actually on. Pinned, a rail drawn today would read
            // "5764h 7m" — the distance to the fixtures' own January — where
            // the live clock has every fixture agent read as just started,
            // which is a state the board really has.
            activityStoreFactory: { ActivityStore(client: client) }
        )
    }

    /// A fixture app whose first load is still in flight and stays that way:
    /// the client never answers, and the start is kicked off rather than
    /// awaited, so what comes back is a model with a project store loading.
    /// It is how a view's own loading state — the rail's skeleton — is drawn,
    /// since a client that answers has no moment to catch one in.
    @MainActor
    public static func loadingAppModel() -> AppModel {
        let model = appModel(client: FixtureNatClient(behaviour: .hanging))
        Task { await start(model) }
        return model
    }

    /// Starts a fixture app on the fixture paths — the project activated, its
    /// plan loaded, its review stats and PR readiness taken.
    @MainActor
    public static func start(_ model: AppModel) async {
        await model.start(configPath: paths.config, nudgePath: paths.nudge)
    }

    /// The app over the fixtures, already loaded — what a test wants, and
    /// what a preview reaches through `start(_:)`.
    @MainActor
    public static func startedAppModel(
        client: FixtureNatClient = FixtureNatClient(),
        config: NatProjectConfig = Fixtures.config
    ) async -> AppModel {
        let model = appModel(client: client, config: config)
        await start(model)
        return model
    }
}
