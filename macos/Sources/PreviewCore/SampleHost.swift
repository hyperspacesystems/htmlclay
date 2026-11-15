import Foundation

public enum SampleReducer {
    public static let addressSuffix = ".htmlclay.localhost:1212"

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
        case let .setFraming(folderID, on):
            guard let i = next.folders.firstIndex(where: { $0.id == folderID }), next.folders[i].served else { return snapshot }
            next.folders[i].allowsFraming = on
        case let .setSynced(folderID, on):
            guard let i = next.folders.firstIndex(where: { $0.id == folderID }) else { return snapshot }
            if on {
                guard next.folders[i].served, next.folders[i].origin.canSync, next.sync.connected else { return snapshot }
                guard !(next.folders[i].becameViewer || next.folders[i].missingOnDisk) else { return snapshot }
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
        case let .answer(requestID, revision, answer, _):
            guard let request = next.requests.first(where: { $0.id == requestID }),
                  request.revision == revision else { return snapshot }
            let allow = answer != RequestAnswer.deny
            next.requests.removeAll { $0.id == request.id }
            next.waiting.removeAll { $0.subjectID == request.id }
            if allow, let p = next.programs.firstIndex(where: { $0.name == request.program }) {
                if !next.programs[p].allowedDocuments.contains(request.document) {
                    next.programs[p].allowedDocuments.append(request.document)
                }
                next.programs[p].removedDocuments.removeAll { $0 == request.document }
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
            if document.isEmpty {
                for allowed in next.programs[p].allowedDocuments where !next.programs[p].removedDocuments.contains(allowed) {
                    next.programs[p].removedDocuments.append(allowed)
                }
                next.programs[p].allowedDocuments = []
            } else {
                next.programs[p].allowedDocuments.removeAll { $0 == document }
                if !next.programs[p].removedDocuments.contains(document) { next.programs[p].removedDocuments.append(document) }
            }
        case .connectSample:
            next.sync.connected = true
            next.sync.username = "panphora"
            next.sync.keySuffix = "7f3a"
            next.sync.lastFinished = "just now"
            next.sync.fullName = "David Miranda"
            next.waiting.removeAll { $0.kind == .syncKeyRefused }
            if next.folder("panphora") == nil {
                next.folders.append(Folder(id: "panphora", name: "panphora", path: "~/hyperclay/panphora", origin: .account(username: "panphora"), served: true, synced: true, address: "panphora.htmlclay.localhost:1212", documentCount: 31, status: "up to date"))
            }
        case .disconnect:
            next.sync = SyncState()
            for i in next.folders.indices { next.folders[i].synced = false }
            next.waiting.removeAll { $0.kind == .syncKeyRefused }
        case let .setProgramAllTrusted(programID, on):
            guard let p = next.programs.firstIndex(where: { $0.id == programID }) else { return snapshot }
            next.programs[p].allTrustedFolders = on
        case let .setUseName(on): next.preferences.useName = on
        case let .markSeen(waitingIDs):
            for i in next.waiting.indices where waitingIDs.contains(next.waiting[i].id) { next.waiting[i].seen = true }
        case let .setAutoUpdate(on): next.update.automatic = on
        case let .trustFolder(name, path, _):
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
        case .resolveConflict:
            // The sample keeps no held conflicts; the real host's list is the
            // waiting items it sends.
            return snapshot
        case let .setUpTeamFolder(folderID):
            guard let i = next.folders.firstIndex(where: { $0.id == folderID }), !next.folders[i].onThisMac else { return snapshot }
            next.folders[i].onThisMac = true
            next.folders[i].served = true
            next.folders[i].synced = next.sync.connected
            next.folders[i].path = "~/hyperclay/" + next.folders[i].name
            next.folders[i].status = "up to date"
        case .startUpdate:
            guard next.update.ready != nil else { return snapshot }
            next.update.finishing = true
        case .finishUpdate:
            guard let ready = next.update.ready, next.update.finishing else { return snapshot }
            next.update.current = ready
            next.update.ready = nil
            next.update.finishing = false
        case .restartHost:
            next.hostStopped = false
        case .restartService:
            return snapshot
        case let .moveFromLocal(folderIDs):
            guard next.localMoveOffer else { return snapshot }
            let source = SampleData.syncedWithTeams
            next.localMoveOffer = false
            for id in folderIDs where next.folder(id) == nil {
                if let folder = source.folder(id) { next.folders.append(folder) }
            }
            for item in source.waiting where item.kind == .conflict && folderIDs.contains(item.subjectID ?? "") && !next.waiting.contains(where: { $0.id == item.id }) {
                next.waiting.append(item)
            }
            if !folderIDs.isEmpty && !next.sync.connected {
                next.sync = SyncState(connected: true, username: "panphora", keySuffix: "7f3a", lastFinished: "just now", fullName: "David Miranda")
            }
        case let .addProgram(name, path, _):
            guard !next.programs.contains(where: { $0.name == name }) else { return snapshot }
            var id = name
            var n = 2
            while next.program(id) != nil { id = name + "-" + String(n); n += 1 }
            next.programs.append(Program(id: id, name: name, path: path))
        case let .relinkProgram(programID, path, _):
            guard let p = next.programs.firstIndex(where: { $0.id == programID }) else { return snapshot }
            next.programs[p].path = path
            next.programs[p].fileMissing = false
        case let .relinkFolder(folderID, path, _):
            guard let i = next.folders.firstIndex(where: { $0.id == folderID }) else { return snapshot }
            next.folders[i].path = path
            next.folders[i].missingOnDisk = false
        case let .askProgramAgain(programID, documentID):
            guard let p = next.programs.firstIndex(where: { $0.id == programID }) else { return snapshot }
            next.programs[p].removedDocuments.removeAll { $0 == documentID }
            if !next.programs[p].allowedDocuments.contains(documentID) {
                next.programs[p].allowedDocuments.append(documentID)
            }
        case let .removeProgram(programID):
            guard next.program(programID) != nil else { return snapshot }
            next.programs.removeAll { $0.id == programID }
        case let .forgetDocument(id):
            guard next.documents.contains(where: { $0.id == id }) else { return snapshot }
            next.documents.removeAll { $0.id == id }
        case .openDocument, .revealDocument, .openFile, .openDashboard,
             .revealFolder, .copyAddress, .connect, .quit, .revealProgram,
             .exportDocument, .recoverDocument, .openVersions, .openKeptPage, .showRequest:
            return snapshot
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
    private var listeners: [Int: @MainActor (HostSnapshot) -> Void] = [:]
    private var listenerCount = 0

    public init(persona: Persona) {
        self.persona = persona
        self.current = Self.snapshot(for: persona)
    }

    public func pretend(_ persona: Persona) {
        let appearance = current.preferences.appearance
        self.persona = persona
        current = Self.snapshot(for: persona)
        current.preferences.appearance = appearance
        listeners.values.forEach { $0(current) }
    }

    /// Every snapshot this host publishes is sample data; a real host's snapshot never is.
    private static func snapshot(for persona: Persona) -> HostSnapshot {
        var snapshot = SampleData.snapshot(for: persona)
        snapshot.isSample = true
        return snapshot
    }

    public func load() async throws -> HostSnapshot { current }

    public func perform(_ action: HostAction) async -> HostOutcome {
        current = SampleReducer.apply(action, to: current)
        listeners.values.forEach { $0(current) }
        return .done(current)
    }

    public func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> {
        switch action {
        case let .serveFolder(path, name):
            if let problem = ServeRules.pathProblem(path, folderID: nil, in: current) {
                return .failure(HostRefusal(code: "path", text: problem, field: "path"))
            }
            let wanted = name.isEmpty ? ServeRules.defaultName(for: path) : name
            if let problem = ServeRules.nameProblem(wanted, folderID: nil, in: current) {
                return .failure(HostRefusal(code: "name-taken", text: problem, field: "name"))
            }
            return .success(HostCheckAnswer(address: ServeRules.address(for: wanted), name: wanted))
        case let .setServed(folderID, on) where on:
            let name = ServeRules.startName(folderID: folderID, in: current)
            return .success(HostCheckAnswer(address: ServeRules.address(for: name), name: name))
        case let .trustFolder(name, path, _):
            var warnings: [HostWarning] = []
            if TrustRules.isMainFolder(path) {
                warnings.append(HostWarning(code: "personal-folder", text: ""))
            }
            let inside = current.localFolders
                .filter { $0.path.hasPrefix(path + "/") }
                .map(\.name)
            if !inside.isEmpty {
                warnings.append(HostWarning(code: "contains-trusted", text: "", names: inside))
            }
            if !current.folders.contains(where: { $0.path == path }),
               let around = SampleReducer.nearestParent(of: path, in: current).flatMap({ current.folder($0) }),
               !around.served {
                warnings.append(HostWarning(code: TrustRules.insideTrusted, text: "", names: [around.name]))
            }
            return .success(HostCheckAnswer(name: name, path: path, warnings: warnings))
        default:
            return .success(HostCheckAnswer())
        }
    }

    @discardableResult
    public func addSampleRequest(_ kind: ProgramRequest.Kind) -> WaitingItem {
        let sample = SampleData.request(kind)
        if !current.requests.contains(where: { $0.id == sample.request.id }) {
            current.requests.append(sample.request)
            current.waiting.append(sample.item)
            listeners.values.forEach { $0(current) }
        }
        return sample.item
    }

    public func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        listenerCount += 1
        let key = listenerCount
        listeners[key] = onChange
        return HostSubscription { [weak self] in self?.listeners[key] = nil }
    }

    public func canToggle(_ overlay: PreviewOverlay) -> Bool {
        switch overlay {
        case .keyRefused, .offline: return current.sync.connected
        case .teamViewer, .teamFolderMissing: return current.folder(SampleData.overlayTeamID)?.onThisMac == true
        case .updateReady, .hostStopped: return true
        }
    }

    public func isOn(_ overlay: PreviewOverlay) -> Bool {
        switch overlay {
        case .keyRefused: return current.waiting.contains { $0.kind == .syncKeyRefused }
        case .offline: return current.sync.offlineChanges > 0
        case .updateReady: return current.update.ready != nil
        case .hostStopped: return current.hostStopped
        case .teamViewer: return current.folder(SampleData.overlayTeamID)?.becameViewer == true
        case .teamFolderMissing: return current.folder(SampleData.overlayTeamID)?.missingOnDisk == true
        }
    }

    public func toggle(_ overlay: PreviewOverlay) {
        guard canToggle(overlay) else { return }
        let on = !isOn(overlay)
        let team = current.folders.firstIndex { $0.id == SampleData.overlayTeamID }
        switch overlay {
        case .keyRefused:
            current.waiting.removeAll { $0.kind == .syncKeyRefused }
            if on { current.waiting.append(WaitingItem(id: "w-key-refused", kind: .syncKeyRefused, title: "Your sync key no longer works. Syncing stopped.")) }
        case .offline:
            current.sync.offlineChanges = on ? SampleData.offlineChanges : 0
        case .updateReady:
            current.update.ready = on ? "2.1.0" : nil
            current.update.finishing = false
        case .hostStopped:
            current.hostStopped = on
        case .teamViewer:
            guard let i = team else { return }
            current.folders[i].becameViewer = on
            current.folders[i].synced = !on && current.sync.connected && current.folders[i].served && !current.folders[i].missingOnDisk
        case .teamFolderMissing:
            guard let i = team else { return }
            current.folders[i].missingOnDisk = on
            current.folders[i].synced = !on && current.sync.connected && current.folders[i].served && !current.folders[i].becameViewer
            current.waiting.removeAll { $0.id == "w-missing-" + SampleData.overlayTeamID }
            if on { current.waiting.append(WaitingItem(id: "w-missing-" + SampleData.overlayTeamID, kind: .folderMissing, title: current.folders[i].path + " isn't there any more", subjectID: SampleData.overlayTeamID)) }
        }
        listeners.values.forEach { $0(current) }
    }
}
