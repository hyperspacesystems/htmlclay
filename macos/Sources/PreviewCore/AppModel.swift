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
    private var noteToken = 0
    private let host: HostClient

    public init(host: HostClient, initial: HostSnapshot) {
        self.host = host
        self.snapshot = initial
        host.subscribe { [weak self] in self?.receive($0) }
    }

    public var settingsPane: SettingsPane {
        get { route.pane }
        set { route = .pane(newValue) }
    }

    public func send(_ action: HostAction) {
        Task { [host] in
            if let next = try? await host.perform(action) { self.receive(next) }
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
        snapshot = next
        switch route {
        case let .folder(id) where next.folder(id) == nil: route = .pane(.folders)
        case let .folderHistory(id) where next.folder(id) == nil: route = .pane(.folders)
        case let .program(id) where next.program(id) == nil: route = .pane(.programs)
        default: break
        }
    }

    public var showsDot: Bool {
        if let counts = snapshot.hostCounts { return counts.dot }
        return snapshot.waiting.contains { $0.earnsDot && !seenWaiting.contains($0.id) }
    }

    public func markWaitingSeen() {
        seenWaiting.formUnion(snapshot.waiting.map(\.id))
    }
}
