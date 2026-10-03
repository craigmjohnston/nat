import Foundation
import NatKit

/// The plugin installer's canned answers, as `nat plugin-list --json` and
/// friends write them.
extension Fixtures {
    /// A machine with nat's own source and one extra that could not be read;
    /// the demo plugin installed by nat with an update waiting, one put in
    /// the plugins directory by hand and one on PATH; and on offer the demo
    /// (installed) and Shortcut (not).
    public static let pluginListing = PluginListing(
        sources: [
            PluginSourceStatus(repo: "craigmjohnston/nat", version: "1.0.57", isDefault: true),
            PluginSourceStatus(
                repo: "someone/nat-plugins",
                error: "read plugin source someone/nat-plugins: GET https://github.com/someone/nat-plugins/releases/latest/download/nat-plugins.json: 404 Not Found"
            ),
        ],
        installed: [
            InstalledPlugin(
                name: "demo", path: "/Users/craig/.config/notion-agent-tracker/plugins/demo/nat-source-demo",
                kind: .managed, source: "craigmjohnston/nat", version: "1.0.52", update: "1.0.57"
            ),
            InstalledPlugin(
                name: "jira", path: "/Users/craig/.config/notion-agent-tracker/plugins/jira/nat-source-jira",
                kind: .manual
            ),
            InstalledPlugin(name: "linear", path: "/opt/homebrew/bin/nat-source-linear", kind: .path),
        ],
        available: [
            AvailablePlugin(
                name: "demo", title: "Demo source", description: "Canned cards to try a source project on.",
                source: "craigmjohnston/nat", version: "1.0.57", installed: true
            ),
            AvailablePlugin(
                name: "shortcut", title: "Shortcut", description: "Shortcut stories as the cards a project's tasks hang off.",
                source: "craigmjohnston/nat", version: "1.0.57", installed: false
            ),
        ]
    )

    /// A machine with nothing installed, whose one source offers nothing yet.
    public static let pluginListingEmpty = PluginListing(
        sources: [PluginSourceStatus(repo: "craigmjohnston/nat", version: "1.0.57", isDefault: true)],
        installed: [],
        available: []
    )
}
