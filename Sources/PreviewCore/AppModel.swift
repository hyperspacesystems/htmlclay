import Foundation
import Observation

@MainActor
@Observable
public final class AppModel {
    public private(set) var snapshot: HostSnapshot
    public private(set) var seenWaiting: Set<String> = []
    public var settingsPane: SettingsPane = .general
    private let host: HostClient

    public init(host: HostClient, initial: HostSnapshot) {
        self.host = host
        self.snapshot = initial
        host.subscribe { [weak self] in self?.snapshot = $0 }
    }

    public func send(_ action: HostAction) {
        Task { [host] in
            if let next = try? await host.perform(action) { self.snapshot = next }
        }
    }

    public var showsDot: Bool {
        snapshot.waiting.contains { $0.earnsDot && !seenWaiting.contains($0.id) }
    }

    public func markWaitingSeen() {
        seenWaiting.formUnion(snapshot.waiting.map(\.id))
    }
}
