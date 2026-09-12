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
            activityStoreFactory: { ActivityStore(client: client, now: { Fixtures.now }) }
        )
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
