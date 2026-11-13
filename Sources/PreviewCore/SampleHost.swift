import Foundation

public enum SampleReducer {
    public static let addressSuffix = ".hyperclay.localhost:1212"

    public static func apply(_ action: HostAction, to snapshot: HostSnapshot) -> HostSnapshot {
        var next = snapshot
        switch action {
        case let .setServed(folderID, on):
            guard let i = next.folders.firstIndex(where: { $0.id == folderID }) else { return snapshot }
            next.folders[i].served = on
            if on, next.folders[i].address.isEmpty {
                next.folders[i].address = next.folders[i].name.lowercased().replacingOccurrences(of: " ", with: "-") + addressSuffix
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
                if allow, let p = next.programs.firstIndex(where: { $0.name == request.program }) {
                    next.programs[p].removedDocuments.removeAll { $0 == request.document }
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
            if !next.programs[p].removedDocuments.contains(document) { next.programs[p].removedDocuments.append(document) }
        case .connectSample:
            next.sync.connected = true
            next.sync.username = "panphora"
            next.sync.keySuffix = "7f3a"
            next.sync.lastFinished = "just now"
            next.sync.fullName = "David Miranda"
            next.waiting.removeAll { $0.kind == .syncKeyRefused }
            if next.folder("panphora") == nil {
                next.folders.append(Folder(id: "panphora", name: "panphora", path: "~/hyperclay/panphora", origin: .account(username: "panphora"), served: true, synced: true, address: "panphora.hyperclay.localhost:1212", documentCount: 31, status: "up to date"))
            }
        case .disconnect:
            next.sync = SyncState()
            for i in next.folders.indices { next.folders[i].synced = false }
            next.waiting.removeAll { $0.kind == .syncKeyRefused }
        case let .setProgramAllTrusted(programID, on):
            guard let p = next.programs.firstIndex(where: { $0.id == programID }) else { return snapshot }
            next.programs[p].allTrustedFolders = on
        case let .setUseName(on): next.preferences.useName = on
        case let .setAutoUpdate(on): next.update.automatic = on
        case let .trustFolder(name, path):
            guard !next.folders.contains(where: { $0.path == path }) else { return snapshot }
            let id = newFolderID(name, in: next)
            let documents = SampleData.trustCandidates.first { $0.path == path }?.documents ?? 0
            next.folders.append(Folder(id: id, name: name, path: path, parentID: nearestParent(of: path, in: next), documentCount: documents, status: path))
            adopt(id, path: path, in: &next)
        case let .serveFolder(path, name):
            if let i = next.folders.firstIndex(where: { $0.path == path }) {
                next.folders[i].served = true
                next.folders[i].address = name + addressSuffix
            } else {
                let folderName = (path as NSString).lastPathComponent
                let id = newFolderID(folderName, in: next)
                next.folders.append(Folder(id: id, name: folderName, path: path, parentID: nearestParent(of: path, in: next), served: true, address: name + addressSuffix, status: Strings.Folders.onlyOnThisMac, versionsSize: "0 MB"))
                adopt(id, path: path, in: &next)
            }
        case let .setUpTeamFolder(folderID):
            guard let i = next.folders.firstIndex(where: { $0.id == folderID }), !next.folders[i].onThisMac else { return snapshot }
            next.folders[i].onThisMac = true
            next.folders[i].served = true
            next.folders[i].synced = next.sync.connected
            next.folders[i].path = "~/hyperclay/" + next.folders[i].name
            next.folders[i].status = "up to date"
        case let .addProgram(name, path):
            guard !next.programs.contains(where: { $0.name == name }) else { return snapshot }
            var id = name
            var n = 2
            while next.program(id) != nil { id = name + "-" + String(n); n += 1 }
            next.programs.append(Program(id: id, name: name, path: path))
        }
        return next
    }

    static func newFolderID(_ name: String, in snapshot: HostSnapshot) -> String {
        let base = name.lowercased().replacingOccurrences(of: " ", with: "-")
        var id = base
        var n = 2
        while snapshot.folder(id) != nil { id = base + "-" + String(n); n += 1 }
        return id
    }

    static func nearestParent(of path: String, in snapshot: HostSnapshot) -> String? {
        snapshot.localFolders.filter { path.hasPrefix($0.path + "/") }.max { $0.path.count < $1.path.count }?.id
    }

    static func adopt(_ id: String, path: String, in snapshot: inout HostSnapshot) {
        for i in snapshot.folders.indices where snapshot.folders[i].origin == .local && snapshot.folders[i].id != id && snapshot.folders[i].path.hasPrefix(path + "/") {
            let current = snapshot.folders[i].parentID.flatMap { snapshot.folder($0) }?.path ?? ""
            if current.count < path.count { snapshot.folders[i].parentID = id }
        }
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

    @discardableResult
    public func addSampleRequest(_ kind: ProgramRequest.Kind) -> WaitingItem {
        let sample = SampleData.request(kind)
        if !current.requests.contains(where: { $0.id == sample.request.id }) {
            current.requests.append(sample.request)
            current.waiting.append(sample.item)
            listeners.forEach { $0(current) }
        }
        return sample.item
    }

    public func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) {
        listeners.append(onChange)
    }
}
