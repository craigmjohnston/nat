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

    /// The Shortcut plugin's one setup field, as its describe lists it with
    /// no token in the Keychain yet.
    public static let shortcutTokenField = PluginSetupField(
        id: "token", label: "API token", input: "secret", hint: "Shortcut ▸ Settings ▸ API Tokens", set: false)

    /// A machine with Shortcut installed and no token set — its field reads
    /// `set: false` — beside a plugin put in by hand whose describe failed,
    /// the describe-error warning. nat fills `setup` only from a describe
    /// that worked, so no one row carries both; and Shortcut's describe needs
    /// no token, so a missing one is its `set`, never its describe failing.
    public static let pluginListingShortcut = PluginListing(
        sources: [PluginSourceStatus(repo: "craigmjohnston/nat", version: "1.0.57", isDefault: true)],
        installed: [
            InstalledPlugin(
                name: "shortcut",
                path: "/Users/craig/.config/notion-agent-tracker/plugins/shortcut/nat-source-shortcut",
                kind: .managed, source: "craigmjohnston/nat", version: "1.0.57",
                setup: [shortcutTokenField]
            ),
            InstalledPlugin(
                name: "jira", path: "/Users/craig/.config/notion-agent-tracker/plugins/jira/nat-source-jira",
                kind: .manual,
                describeError: "source plugin jira speaks protocol 2; this nat speaks protocol 1"
            ),
        ],
        available: [
            AvailablePlugin(
                name: "demo", title: "Demo source", description: "Canned cards to try a source project on.",
                source: "craigmjohnston/nat", version: "1.0.57", installed: true
            ),
            AvailablePlugin(
                name: "shortcut", title: "Shortcut", description: "Shortcut stories as the cards a project's tasks hang off.",
                source: "craigmjohnston/nat", version: "1.0.57", installed: true
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
