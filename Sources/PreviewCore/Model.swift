import Foundation

public enum Persona: String, CaseIterable, Codable, Sendable {
    case newPerson, fiveDocuments, trustedFolder, servedFolder, syncedWithTeams, localUser
}

public enum SettingsPane: String, CaseIterable, Codable, Sendable {
    case general, folders, programs, sync, yourName, updates, advanced
}

public enum SettingsRoute: Equatable, Codable, Sendable {
    case pane(SettingsPane)
    case folder(String)
    case program(String)

    public var pane: SettingsPane {
        switch self {
        case let .pane(pane): return pane
        case .folder: return .folders
        case .program: return .programs
        }
    }
}

public enum Appearance: String, CaseIterable, Codable, Sendable {
    case system, light, dark
}

public enum PreviewSheet: Equatable, Identifiable, Sendable {
    case serveFolder(folderID: String?)
    case trustFolder(name: String, path: String)
    case connect
    case accountFolder(username: String)
    case teamSetup(folderID: String)
    case addProgram

    public var id: String {
        switch self {
        case let .serveFolder(folderID): return "serve." + (folderID ?? "new")
        case let .trustFolder(_, path): return "trust." + path
        case .connect: return "connect"
        case let .accountFolder(username): return "account." + username
        case let .teamSetup(folderID): return "team." + folderID
        case .addProgram: return "add-program"
        }
    }
}

public struct RecentDocument: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var name: String
    public var place: String
    public var fileMissing: Bool
    public var versionsKept: Int
    public var openNow: Bool
    public var opened: String
    public init(id: String, name: String, place: String, fileMissing: Bool = false, versionsKept: Int = 0, openNow: Bool = false, opened: String = "") {
        self.id = id; self.name = name; self.place = place; self.fileMissing = fileMissing
        self.versionsKept = versionsKept; self.openNow = openNow; self.opened = opened
    }
}

public enum FolderOrigin: Codable, Equatable, Sendable {
    case local
    case account(username: String)
    case team(name: String)

    public var canSync: Bool { self != .local }
}

public struct Folder: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var name: String
    public var path: String
    public var origin: FolderOrigin
    public var parentID: String?
    public var served: Bool
    public var synced: Bool
    public var address: String
    public var documentCount: Int
    public var status: String
    public var onThisMac: Bool
    public var missingOnDisk: Bool
    public var versionsSize: String
    public var lastSync: String
    public var historySummary: String

    public init(id: String, name: String, path: String, origin: FolderOrigin = .local, parentID: String? = nil, served: Bool = false, synced: Bool = false, address: String = "", documentCount: Int = 0, status: String = "", onThisMac: Bool = true, missingOnDisk: Bool = false, versionsSize: String = "", lastSync: String = "", historySummary: String = "") {
        self.id = id; self.name = name; self.path = path; self.origin = origin; self.parentID = parentID
        self.served = served; self.synced = synced; self.address = address; self.documentCount = documentCount
        self.status = status; self.onThisMac = onThisMac; self.missingOnDisk = missingOnDisk
        self.versionsSize = versionsSize; self.lastSync = lastSync; self.historySummary = historySummary
    }

    public var fromAccount: Bool { origin.canSync }
    public var trusted: Bool { onThisMac }
}

public struct TrustCandidate: Equatable, Sendable {
    public let name: String
    public let path: String
    public let documents: Int
}

public struct Program: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var name: String
    public var path: String
    public var fileMissing: Bool
    public var allowedDocuments: [String]
    public var allTrustedFolders: Bool
    public var removedDocuments: [String]
    public init(id: String, name: String, path: String, fileMissing: Bool = false, allowedDocuments: [String] = [], allTrustedFolders: Bool = false, removedDocuments: [String] = []) {
        self.id = id; self.name = name; self.path = path; self.fileMissing = fileMissing; self.allowedDocuments = allowedDocuments
        self.allTrustedFolders = allTrustedFolders; self.removedDocuments = removedDocuments
    }
}

public struct ProgramRequest: Codable, Equatable, Identifiable, Sendable {
    public enum Kind: String, Codable, CaseIterable, Sendable {
        case found, noFile, teamDocument, riskyFile, folderRead, folderReadAgain
    }
    public var id: String
    public var kind: Kind
    public var document: String
    public var program: String
    public var folder: String
    public var path: String
    public var detail: String
    public init(id: String, kind: Kind, document: String, program: String, folder: String = "", path: String = "", detail: String = "") {
        self.id = id; self.kind = kind; self.document = document; self.program = program; self.folder = folder
        self.path = path; self.detail = detail
    }
}

public struct WaitingItem: Codable, Equatable, Identifiable, Sendable {
    public enum Kind: String, Codable, CaseIterable, Sendable {
        case conflict, programRequest, syncKeyRefused, folderMissing, serviceFailed
        case programFileMissing, fileTooBig, teamToSetUp

        public var earnsDot: Bool {
            switch self {
            case .conflict, .programRequest, .syncKeyRefused, .folderMissing, .serviceFailed: return true
            case .programFileMissing, .fileTooBig, .teamToSetUp: return false
            }
        }

        public var pane: SettingsPane {
            switch self {
            case .conflict, .folderMissing, .fileTooBig, .teamToSetUp: return .folders
            case .programRequest, .programFileMissing: return .programs
            case .syncKeyRefused: return .sync
            case .serviceFailed: return .advanced
            }
        }
    }
    public var id: String
    public var kind: Kind
    public var title: String
    public var detail: String
    public var subjectID: String?
    public init(id: String, kind: Kind, title: String, detail: String = "", subjectID: String? = nil) {
        self.id = id; self.kind = kind; self.title = title; self.detail = detail; self.subjectID = subjectID
    }
    public var pane: SettingsPane { kind.pane }
    public var earnsDot: Bool { kind.earnsDot }
}

public struct SyncState: Codable, Equatable, Sendable {
    public var connected: Bool
    public var username: String
    public var keySuffix: String
    public var syncingFiles: Int
    public var paused: Bool
    public var viewOnlyTeams: Int
    public var lastFinished: String
    public var fullName: String
    public init(connected: Bool = false, username: String = "", keySuffix: String = "", syncingFiles: Int = 0, paused: Bool = false, viewOnlyTeams: Int = 0, lastFinished: String = "", fullName: String = "") {
        self.lastFinished = lastFinished; self.fullName = fullName
        self.connected = connected; self.username = username; self.keySuffix = keySuffix
        self.syncingFiles = syncingFiles; self.paused = paused; self.viewOnlyTeams = viewOnlyTeams
    }
}

public struct UpdateState: Codable, Equatable, Sendable {
    public var current: String
    public var ready: String?
    public var checked: String
    public var automatic: Bool
    public init(current: String = "2.0.3", ready: String? = nil, checked: String = "Today at 9:12", automatic: Bool = true) {
        self.automatic = automatic
        self.current = current; self.ready = ready; self.checked = checked
    }
}

public struct PreferenceState: Codable, Equatable, Sendable {
    public var openAtLogin: Bool
    public var aiEditing: Bool
    public var appearance: Appearance
    public var yourName: String
    public var useName: Bool
    public init(openAtLogin: Bool = true, aiEditing: Bool = true, appearance: Appearance = .system, yourName: String = "", useName: Bool = false) {
        self.useName = useName
        self.openAtLogin = openAtLogin; self.aiEditing = aiEditing; self.appearance = appearance; self.yourName = yourName
    }
}

public struct HostSnapshot: Codable, Equatable, Sendable {
    public var documents: [RecentDocument]
    public var folders: [Folder]
    public var programs: [Program]
    public var requests: [ProgramRequest]
    public var waiting: [WaitingItem]
    public var sync: SyncState
    public var update: UpdateState
    public var preferences: PreferenceState
    public var localMoveOffer: Bool

    public init(documents: [RecentDocument] = [], folders: [Folder] = [], programs: [Program] = [], requests: [ProgramRequest] = [], waiting: [WaitingItem] = [], sync: SyncState = SyncState(), update: UpdateState = UpdateState(), preferences: PreferenceState = PreferenceState(), localMoveOffer: Bool = false) {
        self.documents = documents; self.folders = folders; self.programs = programs; self.requests = requests
        self.waiting = waiting; self.sync = sync; self.update = update; self.preferences = preferences
        self.localMoveOffer = localMoveOffer
    }

    public var syncConnected: Bool { sync.connected }
    public func count(in pane: SettingsPane) -> Int { waiting.filter { $0.pane == pane }.count }
    public var totalCount: Int { waiting.count }

    public func folder(_ id: String) -> Folder? { folders.first { $0.id == id } }
    public func program(_ id: String) -> Program? { programs.first { $0.id == id } }
    public func children(of id: String) -> [Folder] { folders.filter { $0.parentID == id } }
    public var localFolders: [Folder] { folders.filter { $0.origin == .local } }
    public var accountFolders: [Folder] { folders.filter { $0.origin != .local } }
}

public enum HostAction: Equatable, Sendable {
    case setServed(folderID: String, on: Bool)
    case setSynced(folderID: String, on: Bool)
    case stopTrusting(folderID: String)
    case mergeIntoParent(folderID: String)
    case answerRequest(waitingID: String, allow: Bool)
    case setSyncPaused(Bool)
    case setOpenAtLogin(Bool)
    case setAIEditing(Bool)
    case setAppearance(Appearance)
    case setYourName(String)
    case removeProgramAccess(programID: String, document: String)
    case connectSample
    case disconnect
    case setUpTeamFolder(folderID: String)
    case setProgramAllTrusted(programID: String, on: Bool)
    case setUseName(Bool)
    case setAutoUpdate(Bool)
    case trustFolder(name: String, path: String)
    case serveFolder(path: String, name: String)
    case addProgram(name: String, path: String)
}
