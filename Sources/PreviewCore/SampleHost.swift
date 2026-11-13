import Foundation

public enum SampleReducer {
    public static func apply(_ action: HostAction, to snapshot: HostSnapshot) -> HostSnapshot {
        var next = snapshot
        switch action {
        case let .setServed(folderID, on):
            guard let i = next.folders.firstIndex(where: { $0.id == folderID }) else { return snapshot }
            next.folders[i].served = on
            if on, next.folders[i].address.isEmpty {
                next.folders[i].address = next.folders[i].name.lowercased().replacingOccurrences(of: " ", with: "-") + ".hyperclay.localhost:1212"
            }
            if !on { next.folders[i].synced = false }
        case let .setSynced(folderID, on):
            guard let i = next.folders.firstIndex(where: { $0.id == folderID }) else { return snapshot }
            if on {
                guard next.folders[i].served, next.folders[i].origin.canSync, next.sync.connected else { return snapshot }
            }
            next.folders[i].synced = on
        case let .stopTrusting(folderID):
            guard next.folder(folderID) != nil else { return snapshot }
            next.folders.removeAll { $0.id == folderID }
            for i in next.folders.indices where next.folders[i].parentID == folderID { next.folders[i].parentID = nil }
            next.waiting.removeAll { $0.subjectID == folderID }
        case let .mergeIntoParent(folderID):
            guard let child = next.folder(folderID), let parentID = child.parentID,
                  let p = next.folders.firstIndex(where: { $0.id == parentID }) else { return snapshot }
            next.folders[p].documentCount += child.documentCount
            next.folders.removeAll { $0.id == folderID }
            for i in next.waiting.indices where next.waiting[i].subjectID == folderID { next.waiting[i].subjectID = parentID }
        case let .answerRequest(waitingID, allow):
            guard let item = next.waiting.first(where: { $0.id == waitingID }) else { return snapshot }
            next.waiting.removeAll { $0.id == waitingID }
            if let request = next.requests.first(where: { $0.id == item.subjectID }) {
                next.requests.removeAll { $0.id == request.id }
                if allow, let p = next.programs.firstIndex(where: { $0.name == request.program }),
                   !next.programs[p].allowedDocuments.contains(request.document) {
                    next.programs[p].allowedDocuments.append(request.document)
                }
            }
        case let .setSyncPaused(paused):
            guard next.sync.connected else { return snapshot }
            next.sync.paused = paused
        case let .setOpenAtLogin(on): next.preferences.openAtLogin = on
        case let .setAIEditing(on): next.preferences.aiEditing = on
        case let .setAppearance(appearance): next.preferences.appearance = appearance
        case let .setYourName(name): next.preferences.yourName = name
        case let .removeProgramAccess(programID, document):
            guard let p = next.programs.firstIndex(where: { $0.id == programID }) else { return snapshot }
            next.programs[p].allowedDocuments.removeAll { $0 == document }
        case .connectSample:
            next.sync.connected = true
            next.sync.username = "panphora"
            next.sync.keySuffix = "7f3a"
            if next.folder("panphora") == nil {
                next.folders.append(Folder(id: "panphora", name: "panphora", path: "~/hyperclay/panphora", origin: .account(username: "panphora"), served: true, synced: true, address: "panphora.hyperclay.localhost:1212", documentCount: 31, status: "up to date"))
            }
        case .disconnect:
            next.sync = SyncState()
            for i in next.folders.indices { next.folders[i].synced = false }
            next.waiting.removeAll { $0.kind == .syncKeyRefused }
        case let .setUpTeamFolder(folderID):
            guard let i = next.folders.firstIndex(where: { $0.id == folderID }), !next.folders[i].onThisMac else { return snapshot }
            next.folders[i].onThisMac = true
            next.folders[i].served = true
            next.folders[i].synced = next.sync.connected
            next.folders[i].path = "~/hyperclay/" + next.folders[i].name
            next.folders[i].status = "up to date"
        }
        return next
    }
}

@MainActor
public final class SampleHost: HostClient {
    public private(set) var persona: Persona
    public private(set) var current: HostSnapshot
    private var listeners: [@MainActor (HostSnapshot) -> Void] = []

    public init(persona: Persona) {
        self.persona = persona
        self.current = SampleData.snapshot(for: persona)
    }

    public func pretend(_ persona: Persona) {
        let appearance = current.preferences.appearance
        self.persona = persona
        current = SampleData.snapshot(for: persona)
        current.preferences.appearance = appearance
        listeners.forEach { $0(current) }
    }

    public func load() async throws -> HostSnapshot { current }

    public func perform(_ action: HostAction) async throws -> HostSnapshot {
        current = SampleReducer.apply(action, to: current)
        listeners.forEach { $0(current) }
        return current
    }

    public func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) {
        listeners.append(onChange)
    }
}
