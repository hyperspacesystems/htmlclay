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
    private var inFlight: [String: HostAction] = [:]
    private var queued: [String: HostAction] = [:]
    private var generation = 0
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

    /// The setting an action changes, when it is one: the newest value wins.
    public static func slot(of action: HostAction) -> String? {
        switch action {
        case .setOpenAtLogin: return "prefs.openAtLogin"
        case .setAIEditing: return "prefs.aiEditing"
        case .setAppearance: return "prefs.appearance"
        case .setYourName: return "prefs.yourName"
        case .setUseName: return "prefs.useName"
        case .setAutoUpdate: return "prefs.autoUpdate"
        case .setSyncPaused: return "sync.paused"
        case let .setServed(id, _): return "folder.served." + id
        case let .setSynced(id, _): return "folder.synced." + id
        case let .setProgramAllTrusted(id, _): return "program.allTrusted." + id
        default: return nil
        }
    }

    public func send(_ action: HostAction) {
        Task { _ = await self.run(action, announce: true) }
    }

    /// The value a control should show while its setting is pending: the newest the person chose.
    public func pending(_ slot: String) -> HostAction? {
        queued[slot] ?? inFlight[slot]
    }

    /// True while this exact action (or, for a setting, any value of its slot) is in flight.
    public func isPending(_ action: HostAction) -> Bool {
        inFlight[key(action)] != nil
    }

    /// Sends an action and waits for the host's answer. Sheets call this, close on .done,
    /// and keep their input and show the refusal otherwise.
    public func submit(_ action: HostAction) async -> HostOutcome {
        await run(action, announce: false)
    }

    /// Ends every pending action as interrupted; nothing is resent. The live app calls this
    /// from LoopbackHostClient.onNewEpoch.
    public func hostRestarted() {
        generation += 1
        let wasPending = !inFlight.isEmpty || !queued.isEmpty
        inFlight = [:]
        queued = [:]
        if wasPending { say(Self.interruptedText) }
    }

    private static let interruptedText = "HTML Clay restarted before that finished. Try again."

    private func key(_ action: HostAction) -> String {
        Self.slot(of: action) ?? String(describing: action)
    }

    private func run(_ action: HostAction, announce: Bool) async -> HostOutcome {
        let k = key(action)
        if inFlight[k] != nil {
            if Self.slot(of: action) != nil {
                queued[k] = action
                return .refused(HostRefusal(code: "queued", text: ""))
            }
            return .refused(HostRefusal(code: "duplicate", text: ""))
        }
        inFlight[k] = action
        let gen = generation
        let outcome = await host.perform(action)
        if generation != gen {
            return .refused(HostRefusal(code: "interrupted", text: Self.interruptedText, retryable: true))
        }
        inFlight[k] = nil
        switch outcome {
        case let .done(next):
            receive(next)
        case let .refused(refusal):
            lastRefusal = refusal
            if announce { say(refusal.text) }
        }
        if let next = queued[k] {
            queued[k] = nil
            if next != action {
                Task { _ = await self.run(next, announce: announce) }
            }
        }
        return outcome
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
