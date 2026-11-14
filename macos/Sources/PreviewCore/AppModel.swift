import Foundation
import Observation

@MainActor
@Observable
public final class AppModel {
    public private(set) var snapshot: HostSnapshot
    public private(set) var seenWaiting: Set<String> = []
    public var route: SettingsRoute = .pane(.general)
    public var sheet: PreviewSheet?
    public private(set) var note: String?
    public private(set) var lastRefusal: HostRefusal?
    private var noteToken = 0
    private let host: HostClient
    private var subscription: HostSubscription?

    public init(host: HostClient, initial: HostSnapshot) {
        self.host = host
        self.snapshot = initial
        subscription = host.subscribe { [weak self] in self?.receive($0) }
    }

    public var settingsPane: SettingsPane {
        get { route.pane }
        set { route = .pane(newValue) }
    }

    public func send(_ action: HostAction) {
        Task { [host] in
            switch await host.perform(action) {
            case let .done(next): self.receive(next)
            case let .refused(refusal):
                self.lastRefusal = refusal
                self.say(refusal.text)
            }
        }
    }

    public func say(_ text: String) {
        note = text
        noteToken += 1
        let token = noteToken
        Task { [weak self] in
            try? await Task.sleep(nanoseconds: 6_000_000_000)
            if self?.noteToken == token { self?.note = nil }
        }
    }

    public func resetPreviewState() {
        seenWaiting = []
        note = nil
        sheet = nil
        route = .pane(route.pane)
    }

    private func receive(_ next: HostSnapshot) {
        let previous = snapshot
        snapshot = next
        switch route {
        case let .folder(id) where next.folder(id) == nil:
            route = sameFolder(id, from: previous, in: next).map(SettingsRoute.folder) ?? .pane(.folders)
        case let .folderHistory(id) where next.folder(id) == nil:
            route = sameFolder(id, from: previous, in: next).map(SettingsRoute.folderHistory) ?? .pane(.folders)
        case let .program(id) where next.program(id) == nil: route = .pane(.programs)
        default: break
        }
    }

    /// The host gives a folder a new id when it starts being served, so an open
    /// folder page follows the folder at the same place on disk.
    private func sameFolder(_ id: String, from previous: HostSnapshot, in next: HostSnapshot) -> String? {
        guard let old = previous.folder(id), !old.fullPath.isEmpty else { return nil }
        return next.folders.first { $0.fullPath == old.fullPath }?.id
    }

    public var showsDot: Bool {
        if let counts = snapshot.hostCounts { return counts.dot }
        return snapshot.waiting.contains { $0.earnsDot && !seenWaiting.contains($0.id) }
    }

    public func markWaitingSeen() {
        seenWaiting.formUnion(snapshot.waiting.map(\.id))
        if !snapshot.waiting.isEmpty { send(.markSeen(waitingIDs: snapshot.waiting.map(\.id))) }
    }
}
