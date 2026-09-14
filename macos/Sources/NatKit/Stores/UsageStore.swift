import Foundation
import SwiftUI

/// Manages the Claude usage readout: the disk cache's last-known reading at
/// launch, then whatever `nat usage` reports on refresh — on launch and
/// every `refreshIntervalSeconds` after, matching the app's existing
/// cache-then-refresh shape (`ProjectStore` and `DiskPlanCache`) rather than
/// inventing a second one.
///
/// A probe already in flight is never overlapped: `refresh()` is a no-op
/// while one is running, so the launch call and the timer's own tick cannot
/// race each other into two probes at once — each of which is a live cost
/// against the very quota being measured.
@MainActor
@Observable
public final class UsageStore {
    /// The last reading — from the cache at launch, then the most recent
    /// successful probe. Nil only before the cache has been read even once,
    /// which is the app's first frame; a probe that ran and found nothing —
    /// unavailable, timed out, or failed outright — leaves whatever reading
    /// was already here standing, per the brief's "keeps rendering the last
    /// reading between probes."
    public private(set) var reading: UsageReading?

    private let client: NatClientProtocol
    private let cache: UsageCaching
    private let refreshIntervalSeconds: UInt64
    private var refreshTask: Task<Void, Never>?
    private var probing = false

    public init(
        client: NatClientProtocol = NatClient(),
        cache: UsageCaching = DiskUsageCache(),
        refreshIntervalSeconds: UInt64 = 15 * 60
    ) {
        self.client = client
        self.cache = cache
        self.refreshIntervalSeconds = refreshIntervalSeconds
    }

    /// Loads the cached reading so it is on screen at once, probes fresh,
    /// then arms the recurring timer. Called once, at app start.
    public func start() async {
        reading = await cache.read()
        await refresh()
        startTimer()
    }

    /// Runs one probe, skipped entirely when one is already under way. A
    /// probe that fails or comes back empty is not written over the cache —
    /// there is nothing fresher to remember, and the point of the cache is
    /// to survive exactly this.
    public func refresh() async {
        guard !probing else { return }
        probing = true
        defer { probing = false }

        guard let fresh = try? await client.usage(), !fresh.isEmpty else { return }
        reading = fresh
        await cache.write(fresh)
    }

    /// Stops the recurring timer — app teardown, mirroring `ActivityStore.stop()`.
    public func stop() {
        refreshTask?.cancel()
        refreshTask = nil
    }

    private func startTimer() {
        refreshTask?.cancel()
        refreshTask = Task {
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: refreshIntervalSeconds * 1_000_000_000)
                if Task.isCancelled { break }
                await refresh()
            }
        }
    }
}
