import Foundation

/// Watches a file's modification time and fires a callback when it changes.
/// A missing file is treated as "no reading yet", and file creation counts as a change.
public final class NudgeWatcher: @unchecked Sendable {
    private var timer: Timer?
    private var lastModificationTime: Date?
    private var path: String?
    private var onChange: (() -> Void)?

    public init() {}

    /// Start watching the nudge file at the given path.
    ///
    /// The first reading establishes a baseline; subsequent mtime changes fire the callback.
    /// A missing file is ignored (no error), and file creation counts as a change.
    ///
    /// - Parameters:
    ///   - path: Path to the nudge file to watch
    ///   - onChange: Callback to invoke when the file's mtime changes
    public func start(path: String, onChange: @escaping () -> Void) {
        stop()

        self.path = path
        self.onChange = onChange
        self.lastModificationTime = getModificationTime(for: path)

        // Poll every 1 second, matching the Go TUI's behavior
        timer = Timer.scheduledTimer(withTimeInterval: 1.0, repeats: true) { [weak self] _ in
            self?.checkForChanges()
        }
    }

    /// Stop watching the file.
    public func stop() {
        timer?.invalidate()
        timer = nil
        path = nil
        onChange = nil
        lastModificationTime = nil
    }

    deinit {
        stop()
    }

    // MARK: - Private Helpers

    private func checkForChanges() {
        guard let path = path, let onChange = onChange else {
            return
        }

        let currentModificationTime = getModificationTime(for: path)

        // If the modification time has changed (including the file appearing for the first time),
        // fire the callback
        if currentModificationTime != lastModificationTime {
            lastModificationTime = currentModificationTime
            onChange()
        }
    }

    private func getModificationTime(for path: String) -> Date? {
        let fileManager = FileManager.default
        do {
            let attributes = try fileManager.attributesOfItem(atPath: path)
            if let modificationDate = attributes[.modificationDate] as? Date {
                return modificationDate
            }
            return nil
        } catch {
            // File doesn't exist or can't be stat'd; return nil
            return nil
        }
    }
}
