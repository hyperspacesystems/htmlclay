import Foundation

private enum RelayText {
    // TODO-COPY
    static let notRunning = "HTML Clay isn't running."
}

@MainActor
final class EpochCallback {
    private let run: @MainActor () -> Void
    init(_ run: @escaping @MainActor () -> Void) { self.run = run }
    func call() { run() }
}

/// The one HostClient the app's AppModel keeps, forwarding to the LoopbackHostClient of
/// whichever host process is current.
@MainActor
public final class RelayHostClient: HostClient {
    public init() {}

    /// Called when a new host replaces an earlier one (AppModel.hostRestarted).
    public var onRestart: (@MainActor () -> Void)?
    /// Called when the person presses Restart in "host stopped".
    public var onRestartRequested: (@MainActor () -> Void)?
    /// The latest snapshot listeners saw.
    public private(set) var last = HostSnapshot()

    private var client: HostClient?
    private var subscription: HostSubscription?
    private var listeners: [Int: @MainActor (HostSnapshot) -> Void] = [:]
    private var listenerCount = 0
    /// Raised by every attachment, epoch change and loss, so one loss is announced once.
    private var attachment = 0
    private var announcedAttachment = 0

    /// A ready host: forward to it, subscribe, load its snapshot. A second attach (a restarted host) calls onRestart first.
    public func attach(_ client: LoopbackHostClient) async {
        await attach(client) { hook in
            client.onNewEpoch = { _, _ in hook.call() }
        }
    }

    func attach(_ client: any HostClient, onNewEpoch install: (EpochCallback) -> Void) async {
        subscription?.cancel()
        subscription = nil
        attachment += 1
        if self.client != nil { announceRestart() }
        self.client = client
        install(EpochCallback { [weak self] in
            guard let self else { return }
            self.attachment += 1
            self.announceRestart()
        })
        subscription = client.subscribe { [weak self] snapshot in self?.receive(snapshot) }
        if let loaded = try? await client.load() { receive(loaded) }
    }

    /// The host is gone (stopped or failed): pending work is interrupted once, listeners get `last`
    /// with hostStopped = true, and actions answer unreachable.
    public func hostGone() {
        subscription?.cancel()
        subscription = nil
        client = nil
        last.hostStopped = true
        attachment += 1
        announceRestart()
        listeners.values.forEach { $0(last) }
    }

    public func load() async throws -> HostSnapshot {
        guard let client else { return last }
        return try await client.load()
    }

    public func perform(_ action: HostAction) async -> HostOutcome {
        if case .restartHost = action {
            onRestartRequested?()
            return .done(last)
        }
        guard let client else { return .refused(unreachable) }
        return await client.perform(action)
    }

    public func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> {
        guard let client else { return .failure(unreachable) }
        return await client.check(action)
    }

    public func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        listenerCount += 1
        let key = listenerCount
        listeners[key] = onChange
        return HostSubscription { [weak self] in self?.listeners[key] = nil }
    }

    /// Interrupts AppModel's pending work once per loss; a later attach does not repeat it.
    private func announceRestart() {
        guard announcedAttachment != attachment else { return }
        announcedAttachment = attachment
        onRestart?()
    }

    private var unreachable: HostRefusal {
        HostRefusal(code: "unreachable", text: RelayText.notRunning, retryable: true)
    }

    private func receive(_ snapshot: HostSnapshot) {
        guard snapshot.epoch != last.epoch || snapshot.rev >= last.rev else { return }
        last = snapshot
        listeners.values.forEach { $0(snapshot) }
    }
}
