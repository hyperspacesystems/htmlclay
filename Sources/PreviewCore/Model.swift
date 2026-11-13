import Foundation

public enum Persona: String, CaseIterable, Codable, Sendable {
    case newPerson, fiveDocuments, trustedFolder, servedFolder, syncedWithTeams, localUser
}

public enum SettingsPane: String, CaseIterable, Codable, Sendable {
    case general, folders, programs, sync, yourName, updates, advanced
}

public struct RecentDocument: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var name: String
    public var place: String
    public var fileMissing: Bool
    public init(id: String, name: String, place: String, fileMissing: Bool = false) {
        self.id = id; self.name = name; self.place = place; self.fileMissing = fileMissing
    }
}

public struct Folder: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var name: String
    public var path: String
    public var served: Bool
    public var synced: Bool
    public var fromAccount: Bool
    public init(id: String, name: String, path: String, served: Bool = false, synced: Bool = false, fromAccount: Bool = false) {
        self.id = id; self.name = name; self.path = path; self.served = served; self.synced = synced; self.fromAccount = fromAccount
    }
}

public struct WaitingItem: Codable, Equatable, Identifiable, Sendable {
    public enum Kind: String, Codable, Sendable { case conflict, programRequest, programFileMissing, syncKeyRefused, folderMissing }
    public var id: String
    public var kind: Kind
    public var pane: SettingsPane
    public var title: String
    public var earnsDot: Bool
    public init(id: String, kind: Kind, pane: SettingsPane, title: String, earnsDot: Bool) {
        self.id = id; self.kind = kind; self.pane = pane; self.title = title; self.earnsDot = earnsDot
    }
}

public struct HostSnapshot: Codable, Equatable, Sendable {
    public var documents: [RecentDocument]
    public var folders: [Folder]
    public var waiting: [WaitingItem]
    public var syncConnected: Bool
    public init(documents: [RecentDocument] = [], folders: [Folder] = [], waiting: [WaitingItem] = [], syncConnected: Bool = false) {
        self.documents = documents; self.folders = folders; self.waiting = waiting; self.syncConnected = syncConnected
    }
    public func count(in pane: SettingsPane) -> Int { waiting.filter { $0.pane == pane }.count }
    public var totalCount: Int { waiting.count }
}

public enum HostAction: Equatable, Sendable {
    case setServed(folderID: String, on: Bool)
    case stopTrusting(folderID: String)
    case answerRequest(waitingID: String, allow: Bool)
}
