import Foundation

public enum Persona: String, CaseIterable, Codable, Sendable {
    case newPerson, fiveDocuments, trustedFolder, servedFolder, syncedWithTeams, localUser

    /// True when the host can make this persona for real on a throwaway home.
    public var isReal: Bool {
        switch self {
        case .newPerson, .fiveDocuments, .trustedFolder, .servedFolder: return true
        case .syncedWithTeams, .localUser: return false
        }
    }

    /// The milestone that makes this persona real, or nil when it already is.
    public var milestone: Int? {
        switch self {
        case .newPerson, .fiveDocuments, .trustedFolder, .servedFolder: return nil
        case .syncedWithTeams: return 4
        case .localUser: return 5
        }
    }

    /// The personas that are still hand-written sample data, in declaration order.
    public static var sampleOnly: [Persona] { allCases.filter { !$0.isReal } }
}

public enum SettingsPane: String, CaseIterable, Codable, Sendable {
    case general
    case folders
    case programs
    case sync
    case aiEditing = "ai"
    case updates
    case advanced
}

public enum SettingsRoute: Equatable, Codable, Sendable {
    case pane(SettingsPane)
    case folder(String)
    case program(String)
    case folderHistory(String)

    public var pane: SettingsPane {
        switch self {
        case let .pane(pane): return pane
        case .folder: return .folders
        case .folderHistory: return .folders
        case .program: return .programs
        }
    }
}

public enum Appearance: String, CaseIterable, Codable, Sendable {
    case system, light, dark
}

public enum PreviewSheet: Equatable, Identifiable, Sendable {
    case trustFolder(name: String, path: String)
    case connect
    case accountFolder(username: String)
    case teamSetup(folderID: String)
    case addProgram
    case customCommand
    case changePort
    case relinkProgram(programID: String, path: String)
    case relinkFolder(folderID: String, path: String)
    case merge(childID: String)
    case moveFromLocal
    case movedFromLocal

    public var id: String {
        switch self {
        case let .trustFolder(_, path): return "trust." + path
        case .connect: return "connect"
        case let .accountFolder(username): return "account." + username
        case let .teamSetup(folderID): return "team." + folderID
        case .addProgram: return "add-program"
        case .customCommand: return "custom-command"
        case .changePort: return "change-port"
        case let .relinkProgram(programID, _): return "relink." + programID
        case let .relinkFolder(folderID, _): return "relink-folder." + folderID
        case let .merge(childID): return "merge." + childID
        case .moveFromLocal: return "move"
        case .movedFromLocal: return "moved"
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
    public var url: String
    public var folderID: String?
    public var openedAt: Date?
    public init(id: String, name: String, place: String, fileMissing: Bool = false, versionsKept: Int = 0, openNow: Bool = false, opened: String = "", url: String = "", folderID: String? = nil, openedAt: Date? = nil) {
        self.id = id; self.name = name; self.place = place; self.fileMissing = fileMissing
        self.versionsKept = versionsKept; self.openNow = openNow; self.opened = opened
        self.url = url; self.folderID = folderID; self.openedAt = openedAt
    }
}

public struct FolderCan: Codable, Equatable, Sendable {
    public var serve: Bool
    public var stopServing: Bool
    public var setFraming: Bool
    public var sync: Bool
    public var stopSyncing: Bool
    public var stopTrusting: Bool
    public var merge: Bool
    public var openWorkspace: Bool
    public init(serve: Bool = false, stopServing: Bool = false, setFraming: Bool = false, sync: Bool = false, stopSyncing: Bool = false, stopTrusting: Bool = false, merge: Bool = false, openWorkspace: Bool = false) {
        self.serve = serve; self.stopServing = stopServing; self.setFraming = setFraming; self.sync = sync
        self.stopSyncing = stopSyncing
        self.stopTrusting = stopTrusting; self.merge = merge; self.openWorkspace = openWorkspace
    }
}

public struct HostReason: Codable, Equatable, Sendable {
    public var code: String
    public var text: String
    public init(code: String = "", text: String = "") {
        self.code = code; self.text = text
    }
}

public struct FolderWhy: Codable, Equatable, Sendable {
    public var serve: HostReason?
    public var sync: HostReason?
    public var stopTrusting: HostReason?
    public var merge: HostReason?
    public init(serve: HostReason? = nil, sync: HostReason? = nil, stopTrusting: HostReason? = nil, merge: HostReason? = nil) {
        self.serve = serve; self.sync = sync; self.stopTrusting = stopTrusting; self.merge = merge
    }
}

public enum FolderOrigin: Codable, Equatable, Sendable {
    case local
    case account(username: String)
    case team(name: String)

    public var canSync: Bool { self != .local }
}

/// The engine's own counts for a first download into a folder.
public struct FirstDownload: Codable, Equatable, Sendable {
    public var done: Int
    public var total: Int
    public var bytesDone: Int64
    public var bytesTotal: Int64
    public init(done: Int, total: Int, bytesDone: Int64 = 0, bytesTotal: Int64 = 0) {
        self.done = done; self.total = total; self.bytesDone = bytesDone; self.bytesTotal = bytesTotal
    }
    public var fraction: Double { total > 0 ? Double(done) / Double(total) : 0 }
}

/// A synced folder's state as the host sends it (appcore FolderSync).
public enum FolderSyncState {
    public static let off = "off"
    public static let upToDate = "up-to-date"
    public static let syncing = "syncing"
    public static let conflicts = "conflicts"
    public static let paused = "paused"
}

public struct Folder: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var name: String
    public var path: String
    public var origin: FolderOrigin
    public var parentID: String?
    public var served: Bool
    public var allowsFraming: Bool
    public var synced: Bool
    public var address: String
    public var documentCount: Int
    public var status: String
    public var onThisMac: Bool
    public var missingOnDisk: Bool
    public var versionsSize: String
    public var lastSync: String
    public var historySummary: String
    public var becameViewer: Bool
    public var fullPath: String
    public var url: String
    public var state: String
    public var can: FolderCan?
    public var why: FolderWhy
    public var storageBytes: Int
    /// A first download's progress while it runs (08-sync.md decision 20).
    public var download: FirstDownload? = nil
    /// The host's sync state for this folder: off, up-to-date, syncing,
    /// conflicts or paused (appcore FolderSync).
    public var syncState: String
    /// Why the last first download here failed, for a folder not on this Mac.
    public var setUpFailed: String? = nil

    public init(id: String, name: String, path: String, origin: FolderOrigin = .local, parentID: String? = nil, served: Bool = false, allowsFraming: Bool = false, synced: Bool = false, address: String = "", documentCount: Int = 0, status: String = "", onThisMac: Bool = true, missingOnDisk: Bool = false, versionsSize: String = "", lastSync: String = "", historySummary: String = "", becameViewer: Bool = false, fullPath: String = "", url: String = "", state: String = "ok", can: FolderCan? = nil, why: FolderWhy = FolderWhy(), storageBytes: Int = 0, syncState: String = FolderSyncState.off) {
        self.id = id; self.name = name; self.path = path; self.origin = origin; self.parentID = parentID
        self.served = served; self.allowsFraming = allowsFraming
        self.synced = synced; self.address = address; self.documentCount = documentCount
        self.status = status; self.onThisMac = onThisMac; self.missingOnDisk = missingOnDisk
        self.versionsSize = versionsSize; self.lastSync = lastSync; self.historySummary = historySummary
        self.becameViewer = becameViewer
        self.fullPath = fullPath; self.url = url; self.state = state; self.can = can; self.why = why
        self.storageBytes = storageBytes
        self.syncState = syncState
    }

    public var fromAccount: Bool { origin.canSync }
    /// An account or team folder that is not on this Mac and that the server will
    /// not sync here (a viewer's team, a lapsed plan): it offers no set-up.
    public var cannotSetUp: Bool { !onThisMac && syncState == FolderSyncState.paused }
    public var trusted: Bool { onThisMac }
}

public struct TrustCandidate: Equatable, Sendable {
    public let name: String
    public let path: String
    public let documents: Int
}

public struct MoveFolderInfo: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var name: String
    public var port: Int
    public var address: String
    /// True when Hyperclay Local syncs the folder.
    public var synced: Bool
    /// Why the last move left the folder with Hyperclay Local, or empty.
    public var skip: String
    public init(id: String = "", name: String = "", port: Int = 0, address: String = "", synced: Bool = false, skip: String = "") {
        self.id = id; self.name = name; self.port = port; self.address = address
        self.synced = synced; self.skip = skip
    }
}

/// What stands between the person and the move from Hyperclay Local.
public struct MoveBlockers: Codable, Equatable, Sendable {
    public var localRunning: Bool
    public var needsKey: Bool
    public init(localRunning: Bool = false, needsKey: Bool = false) {
        self.localRunning = localRunning; self.needsKey = needsKey
    }
}

/// One folder a page may read because the person allowed it on the read prompt.
/// place is the folder as a person reads it; pages are the documents sharing it.
public struct ReadAccess: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var place: String
    public var pages: [String]

    public init(id: String, place: String, pages: [String] = []) {
        self.id = id; self.place = place; self.pages = pages
    }
}

public struct Program: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var name: String
    public var path: String
    public var fileMissing: Bool
    public var allowedDocuments: [String]
    public var allTrustedFolders: Bool
    public var removedDocuments: [String]
    /// One of the programs HTML Clay ships rather than one somebody added.
    public var isBuiltin: Bool
    /// Whether a built-in program is switched on. Built-in programs start off.
    public var isOn: Bool
    public init(id: String, name: String, path: String, fileMissing: Bool = false, allowedDocuments: [String] = [], allTrustedFolders: Bool = false, removedDocuments: [String] = [], isBuiltin: Bool = false, isOn: Bool = false) {
        self.id = id; self.name = name; self.path = path; self.fileMissing = fileMissing; self.allowedDocuments = allowedDocuments
        self.allTrustedFolders = allTrustedFolders; self.removedDocuments = removedDocuments
        self.isBuiltin = isBuiltin; self.isOn = isOn
    }
}

public struct ProgramRequest: Codable, Equatable, Identifiable, Sendable {
    public enum Kind: String, Codable, CaseIterable, Sendable {
        case found, noFile, teamDocument, riskyFile, folderRead, folderReadAgain, builtinProgram, unknownDocument, aiTools
    }
    public var id: String
    public var kind: Kind
    public var document: String
    public var program: String
    public var folder: String
    public var path: String
    public var detail: String
    public var revision: Int
    public var answers: [String]
    public var documentID: String?
    /// How many times somebody asked for this question to be shown. The Mac app
    /// raises its window when the count goes up; nothing else about the
    /// question changes. Older hosts send none, which reads as never raised.
    public var attention: Int?
    public init(id: String, kind: Kind, document: String, program: String, folder: String = "", path: String = "", detail: String = "", revision: Int = 0, answers: [String] = [], documentID: String? = nil, attention: Int? = nil) {
        self.id = id; self.kind = kind; self.document = document; self.program = program; self.folder = folder
        self.path = path; self.detail = detail; self.revision = revision; self.answers = answers
        self.documentID = documentID; self.attention = attention
    }
}

public struct WaitingItem: Codable, Equatable, Identifiable, Sendable {
    public enum Kind: String, Codable, CaseIterable, Sendable {
        case conflict, programRequest, syncKeyRefused, folderMissing, serviceFailed
        case programFileMissing, fileTooBig, teamToSetUp, removalHeld, fileNotText, nameClash

        public var earnsDot: Bool {
            switch self {
            case .conflict, .programRequest, .syncKeyRefused, .folderMissing, .serviceFailed, .removalHeld: return true
            case .programFileMissing, .fileTooBig, .teamToSetUp, .fileNotText, .nameClash: return false
            }
        }

        public var pane: SettingsPane {
            switch self {
            case .conflict, .folderMissing, .fileTooBig, .teamToSetUp, .removalHeld, .fileNotText, .nameClash: return .folders
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
    public var seen: Bool
    public var hostPane: SettingsPane?
    public var hostEarnsDot: Bool?
    public init(id: String, kind: Kind, title: String, detail: String = "", subjectID: String? = nil, seen: Bool = false, hostPane: SettingsPane? = nil, hostEarnsDot: Bool? = nil) {
        self.id = id; self.kind = kind; self.title = title; self.detail = detail; self.subjectID = subjectID
        self.seen = seen; self.hostPane = hostPane; self.hostEarnsDot = hostEarnsDot
    }
    public var pane: SettingsPane { hostPane ?? kind.pane }
    public var earnsDot: Bool { hostEarnsDot ?? kind.earnsDot }
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
    public var offlineChanges: Int
    /// The key is in a file rather than the system's secret store (Linux without
    /// a Secret Service); the Sync pane says so.
    public var keyInFile: Bool
    /// True once this Mac has had sync on, so the sync log stays offered after
    /// a disconnect.
    public var everOn: Bool
    public init(connected: Bool = false, username: String = "", keySuffix: String = "", syncingFiles: Int = 0, paused: Bool = false, viewOnlyTeams: Int = 0, lastFinished: String = "", fullName: String = "", offlineChanges: Int = 0, keyInFile: Bool = false, everOn: Bool = false) {
        self.lastFinished = lastFinished; self.fullName = fullName
        self.offlineChanges = offlineChanges; self.keyInFile = keyInFile
        self.everOn = everOn
        self.connected = connected; self.username = username; self.keySuffix = keySuffix
        self.syncingFiles = syncingFiles; self.paused = paused; self.viewOnlyTeams = viewOnlyTeams
    }
}

public struct UpdateState: Codable, Equatable, Sendable {
    public var current: String
    public var ready: String?
    public var checked: String
    public var automatic: Bool
    public var finishing: Bool
    /// A check is running now. The Mac reads its own updater instead.
    public var checking: Bool
    public init(current: String = "2.0.3", ready: String? = nil, checked: String = "Today at 9:12", automatic: Bool = true, finishing: Bool = false, checking: Bool = false) {
        self.automatic = automatic
        self.finishing = finishing
        self.current = current; self.ready = ready; self.checked = checked
        self.checking = checking
    }

    /// A version without a trailing .0 third part: 2.0.0 shows 2.0, 2.0.1 stays 2.0.1.
    public static func short(_ version: String) -> String {
        let parts = version.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count == 3, parts[2] == "0" else { return version }
        return parts[0] + "." + parts[1]
    }
}

public struct AIEditTool: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var custom: Bool
    public var found: Bool
    public var model: String
    public var defaultModel: String
    public var models: [String]
    public var effort: String
    public var defaultEffort: String
    public var efforts: [String]
    public var modelEfforts: [String: [String]]?
    public var command: String
    public init(id: String, custom: Bool = false, found: Bool = false, model: String = "", defaultModel: String = "", models: [String] = [],
                effort: String = "", defaultEffort: String = "", efforts: [String] = [], modelEfforts: [String: [String]]? = nil, command: String = "") {
        self.id = id; self.custom = custom; self.found = found
        self.model = model; self.defaultModel = defaultModel; self.models = models
        self.effort = effort; self.defaultEffort = defaultEffort; self.efforts = efforts
        self.modelEfforts = modelEfforts; self.command = command
    }

    public func efforts(for model: String) -> [String] {
        let key = model.isEmpty ? defaultModel : model
        if let specific = modelEfforts?[key], !specific.isEmpty { return specific }
        return efforts
    }
}

public enum OfflineMode: String, Codable, Equatable, Hashable, Sendable, CaseIterable {
    case `default` = "default"
    case full = "full"
}

public enum OfflineSpace: String, Codable, Equatable, Hashable, Sendable, CaseIterable {
    case mb500 = "500mb"
    case gb2 = "2gb"
    case gb10 = "10gb"
}

public struct OfflinePolicy: Codable, Equatable, Sendable {
    public static let minLimitMB = 50
    public static let maxLimitMB = 1 << 24

    public var mode: OfflineMode
    public var space: OfflineSpace
    public var limited: Bool
    public var images: Bool
    public var video: Bool
    public var audio: Bool
    public var limitMB: Int

    public init(
        mode: OfflineMode = .default,
        space: OfflineSpace = .mb500,
        limited: Bool = false,
        images: Bool = true,
        video: Bool = true,
        audio: Bool = true,
        limitMB: Int = 0
    ) {
        self.mode = mode
        self.space = space
        self.limited = limited
        self.images = images
        self.video = video
        self.audio = audio
        self.limitMB = limitMB
    }

    private enum CodingKeys: String, CodingKey {
        case mode, space, limited, images, video, audio, limitMB
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        let rawMode = try container.decodeIfPresent(String.self, forKey: .mode) ?? ""
        let rawSpace = try container.decodeIfPresent(String.self, forKey: .space) ?? ""
        let limited = try container.decodeIfPresent(Bool.self, forKey: .limited) ?? false
        let images = try container.decodeIfPresent(Bool.self, forKey: .images) ?? true
        let video = try container.decodeIfPresent(Bool.self, forKey: .video) ?? true
        let audio = try container.decodeIfPresent(Bool.self, forKey: .audio) ?? true
        let limitMB = try container.decodeIfPresent(Int.self, forKey: .limitMB) ?? 0
        if rawMode.isEmpty && rawSpace.isEmpty && !limited && !images && !video && !audio && limitMB == 0 {
            self.init()
            return
        }
        self.init(
            mode: OfflineMode(rawValue: rawMode) ?? .default,
            space: OfflineSpace(rawValue: rawSpace) ?? .mb500,
            limited: limited,
            images: images,
            video: video,
            audio: audio,
            limitMB: max(0, limitMB)
        )
    }

    /// Parses the custom MB field: commas ignored, empty or 0 means unlimited (0),
    /// whole numbers from 50 up to 1<<24 accepted, anything else nil.
    public static func parseLimitMB(_ raw: String) -> Int? {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { return 0 }
        let cleaned = trimmed.replacingOccurrences(of: ",", with: "")
        guard !cleaned.isEmpty, cleaned.allSatisfy({ $0.isASCII && $0.isNumber }),
              let value = Int(cleaned) else { return nil }
        if value == 0 { return 0 }
        guard value >= minLimitMB, value <= maxLimitMB else { return nil }
        return value
    }

    /// Formats limitMB for the custom MB text field (empty when unlimited).
    public static func formatLimitMB(_ limitMB: Int) -> String {
        limitMB > 0 ? String(limitMB) : ""
    }
}

public struct OfflineState: Codable, Equatable, Sendable {
    public var available: Bool
    public var bytes: Int64
    public var files: Int
    public init(available: Bool = false, bytes: Int64 = 0, files: Int = 0) {
        self.available = available; self.bytes = bytes; self.files = files
    }
}

public struct PreferenceState: Codable, Equatable, Sendable {
    public var openAtLogin: Bool
    public var aiEditing: Bool
    public var appearance: Appearance
    public var yourName: String
    public var useName: Bool
    public var openAtLoginChosen: Bool
    /// The AI tool AI editing uses: a built-in tool's id, or "custom".
    public var aiEditTool: String
    /// The AI tools this Mac has, as the host found them.
    public var aiEditTools: [AIEditTool]
    /// Whether HTML Clay keeps offline copies of the files HTML files load.
    public var offlineCopies: Bool
    /// What offline copies keep while they are on.
    public var offlinePolicy: OfflinePolicy
    public init(openAtLogin: Bool = true, aiEditing: Bool = true, appearance: Appearance = .system, yourName: String = "", useName: Bool = false, openAtLoginChosen: Bool = true, aiEditTool: String = "", aiEditTools: [AIEditTool] = [], offlineCopies: Bool = true, offlinePolicy: OfflinePolicy = OfflinePolicy()) {
        self.useName = useName
        self.openAtLogin = openAtLogin; self.aiEditing = aiEditing; self.appearance = appearance; self.yourName = yourName
        self.openAtLoginChosen = openAtLoginChosen
        self.aiEditTool = aiEditTool; self.aiEditTools = aiEditTools
        self.offlineCopies = offlineCopies; self.offlinePolicy = offlinePolicy
    }

    private enum CodingKeys: String, CodingKey {
        case openAtLogin, aiEditing, appearance, yourName, useName, openAtLoginChosen, aiEditTool, aiEditTools,
             offlineCopies, offlinePolicy
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        openAtLogin = try container.decode(Bool.self, forKey: .openAtLogin)
        aiEditing = try container.decode(Bool.self, forKey: .aiEditing)
        appearance = try container.decode(Appearance.self, forKey: .appearance)
        yourName = try container.decode(String.self, forKey: .yourName)
        useName = try container.decode(Bool.self, forKey: .useName)
        openAtLoginChosen = try container.decode(Bool.self, forKey: .openAtLoginChosen)
        aiEditTool = try container.decode(String.self, forKey: .aiEditTool)
        aiEditTools = try container.decode([AIEditTool].self, forKey: .aiEditTools)
        offlineCopies = try container.decodeIfPresent(Bool.self, forKey: .offlineCopies) ?? true
        offlinePolicy = try container.decodeIfPresent(OfflinePolicy.self, forKey: .offlinePolicy) ?? OfflinePolicy()
    }
}

public struct HostInfo: Codable, Equatable, Sendable {
    public var version: String
    public var os: String
    public var port: Int
    public var dashboardURL: String
    public var service: String
    public var can: [String: Bool]
    public init(version: String = "", os: String = "", port: Int = 0, dashboardURL: String = "", service: String = "", can: [String: Bool] = [:]) {
        self.version = version; self.os = os; self.port = port; self.dashboardURL = dashboardURL
        self.service = service; self.can = can
    }
}

public struct WaitCounts: Codable, Equatable, Sendable {
    public var total: Int
    public var byPane: [String: Int]
    public var dot: Bool
    public init(total: Int = 0, byPane: [String: Int] = [:], dot: Bool = false) {
        self.total = total; self.byPane = byPane; self.dot = dot
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
    public var hostStopped: Bool
    public var rev: Int
    public var epoch: String
    public var host: HostInfo
    public var hostCounts: WaitCounts?
    public var moveFolders: [MoveFolderInfo]
    public var moveBlockers: MoveBlockers
    /// Only the preview's SampleHost sets this: a host snapshot is never sample data.
    public var isSample: Bool
    /// How many edits HTML Clay saved aside; the menu and the panel hide the row at 0.
    public var recoveredChanges: Int
    /// Settings, Folders: the read permissions list, hidden when empty.
    public var readAccess: [ReadAccess]
    /// Documents with a stored Don't allow answer, offered Ask again on Programs.
    public var deniedDocuments: [String]
    /// What the offline copies take on disk, and whether they are available at all.
    public var offline: OfflineState

    private enum CodingKeys: String, CodingKey {
        case documents, folders, programs, requests, waiting, sync, update, preferences,
             localMoveOffer, hostStopped, rev, epoch, host, hostCounts, moveFolders, moveBlockers,
             recoveredChanges, readAccess, deniedDocuments, offline
    }

    public init(documents: [RecentDocument] = [], folders: [Folder] = [], programs: [Program] = [], requests: [ProgramRequest] = [], waiting: [WaitingItem] = [], sync: SyncState = SyncState(), update: UpdateState = UpdateState(), preferences: PreferenceState = PreferenceState(), localMoveOffer: Bool = false, hostStopped: Bool = false, rev: Int = 0, epoch: String = "", host: HostInfo = HostInfo(), hostCounts: WaitCounts? = nil, moveFolders: [MoveFolderInfo] = [], moveBlockers: MoveBlockers = MoveBlockers(), isSample: Bool = false, recoveredChanges: Int = 0, readAccess: [ReadAccess] = [], deniedDocuments: [String] = [], offline: OfflineState = OfflineState()) {
        self.documents = documents; self.folders = folders; self.programs = programs; self.requests = requests
        self.waiting = waiting; self.sync = sync; self.update = update; self.preferences = preferences
        self.localMoveOffer = localMoveOffer
        self.hostStopped = hostStopped
        self.rev = rev; self.epoch = epoch; self.host = host; self.hostCounts = hostCounts
        self.moveFolders = moveFolders
        self.moveBlockers = moveBlockers
        self.isSample = isSample
        self.recoveredChanges = recoveredChanges
        self.readAccess = readAccess
        self.deniedDocuments = deniedDocuments
        self.offline = offline
    }

    /// A decoded snapshot is never sample data: isSample stays out of the JSON.
    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        documents = try container.decode([RecentDocument].self, forKey: .documents)
        folders = try container.decode([Folder].self, forKey: .folders)
        programs = try container.decode([Program].self, forKey: .programs)
        requests = try container.decode([ProgramRequest].self, forKey: .requests)
        waiting = try container.decode([WaitingItem].self, forKey: .waiting)
        sync = try container.decode(SyncState.self, forKey: .sync)
        update = try container.decode(UpdateState.self, forKey: .update)
        preferences = try container.decode(PreferenceState.self, forKey: .preferences)
        localMoveOffer = try container.decode(Bool.self, forKey: .localMoveOffer)
        hostStopped = try container.decode(Bool.self, forKey: .hostStopped)
        rev = try container.decode(Int.self, forKey: .rev)
        epoch = try container.decode(String.self, forKey: .epoch)
        host = try container.decode(HostInfo.self, forKey: .host)
        hostCounts = try container.decodeIfPresent(WaitCounts.self, forKey: .hostCounts)
        moveFolders = try container.decode([MoveFolderInfo].self, forKey: .moveFolders)
        moveBlockers = try container.decodeIfPresent(MoveBlockers.self, forKey: .moveBlockers) ?? MoveBlockers()
        recoveredChanges = try container.decodeIfPresent(Int.self, forKey: .recoveredChanges) ?? 0
        readAccess = try container.decodeIfPresent([ReadAccess].self, forKey: .readAccess) ?? []
        deniedDocuments = try container.decodeIfPresent([String].self, forKey: .deniedDocuments) ?? []
        offline = try container.decodeIfPresent(OfflineState.self, forKey: .offline) ?? OfflineState()
        isSample = false
    }

    public var syncConnected: Bool { sync.connected }
    public var filesStillUploading: Int { sync.connected && !sync.paused && sync.offlineChanges == 0 ? sync.syncingFiles : 0 }
    public func count(in pane: SettingsPane) -> Int { hostCounts?.byPane[pane.rawValue] ?? waiting.filter { $0.pane == pane }.count }
    public var totalCount: Int { hostCounts?.total ?? waiting.count }
    public func hostCan(_ type: String) -> Bool { host.can[type] ?? true }

    public func folder(_ id: String) -> Folder? { folders.first { $0.id == id } }
    public func program(_ id: String) -> Program? { programs.first { $0.id == id } }
    public func children(of id: String) -> [Folder] { folders.filter { $0.parentID == id } }
    public var localFolders: [Folder] { folders.filter { $0.origin == .local } }
    public var accountFolders: [Folder] { folders.filter { $0.origin != .local } }
}

public enum HostAction: Equatable, Sendable {
    case setServed(folderID: String, on: Bool)
    case setSynced(folderID: String, on: Bool)
    case setFraming(folderID: String, on: Bool)
    case stopTrusting(folderID: String, confirmed: Bool = false, force: Bool = false)
    case mergeIntoParent(folderID: String)
    case setSyncPaused(Bool)
    case setOpenAtLogin(Bool)
    case setAIEditing(Bool)
    case setOfflineCopies(Bool)
    case setOfflinePolicy(OfflinePolicy)
    case clearOfflineCopies
    case setAppearance(Appearance)
    case setYourName(String)
    case removeProgramAccess(programID: String, document: String)
    case removeAllProgramAccess(programID: String)
    case removeReadAccess(id: String)
    case connectSample
    case disconnect
    /// Sets up an account or team folder at path. existing syncs into files
    /// already there (own account only); confirmed is the person having seen
    /// the counts the host answered for it.
    case setUpTeamFolder(folderID: String, path: String = "", existing: Bool = false, confirmed: Bool = false)
    /// A held conflict's choice: the conflict's id and path as the host listed
    /// it, and "mine", "theirs" or "both".
    case resolveConflict(id: String, path: String, keep: String)
    /// A held removal of many attachments in a folder: "restore" puts them back
    /// here, "delete" removes them on the server too.
    case resolveRemoval(id: String, choice: String)
    case setProgramAllTrusted(programID: String, on: Bool)
    case setUseName(Bool)
    case setAutoUpdate(Bool)
    case trustFolder(name: String, path: String, acknowledged: [String] = [String]())
    case serveFolder(path: String, name: String)
    case addProgram(name: String, path: String, acknowledged: [String] = [String]())
    case relinkProgram(programID: String, path: String, acknowledged: [String] = [String]())
    case relinkFolder(folderID: String, path: String, acknowledged: [String] = [String]())
    case askProgramAgain(programID: String, documentID: String)
    case removeProgram(programID: String)
    case revealProgram(programID: String)
    case startUpdate
    /// The version the updater has downloaded ("" for none), so the host's
    /// browser pages can offer Update.
    case updatePrepared(String)
    case finishUpdate
    case restartHost
    case restartService
    case moveFromLocal(folderIDs: [String])
    case quitLocal
    case markSeen(waitingIDs: [String])
    case openDocument(id: String)
    case revealDocument(id: String)
    case forgetDocument(id: String)
    case exportDocument(id: String)
    case recoverDocument(id: String, destination: String)
    case openVersions(id: String)
    case openRecoveredChanges
    case openFile(path: String)
    /// A Home card when no folder is served: the host writes the template into
    /// ~/htmlclay under a free name and opens it.
    case createFromTemplate(templateID: String)
    case openDashboard(route: String)
    case openSettings(pane: SettingsPane? = nil)
    case revealFolder(id: String)
    case copyAddress(folderID: String)
    case answer(requestID: String, revision: Int, answer: String, path: String = String())
    case showRequest(requestID: String)
    case connect(username: String, key: String)
    case revealSyncLog
    case revealLog
    /// confirmed is the person choosing Restart anyway after restart-stops-work.
    case setPort(Int, confirmed: Bool = false)
    case setProgramOn(programID: String, on: Bool)
    case openProgramExample(programID: String)
    case setAIEditTool(String)
    case setAIEditModel(tool: String, model: String)
    case setAIEditEffort(tool: String, effort: String)
    case setAIEditCustom(command: String)
    /// The host actions Windows and Linux send for themselves: the Mac sets
    /// file types and checks for updates in Swift, and never sends these.
    case checkForUpdates
    case makeDefaultFileType
    case setHTMLOpenWith(Bool)
    case stopOpeningFiles
    case quit(force: Bool)
}

public enum PreviewOverlay: String, CaseIterable, Sendable {
    case keyRefused, offline, updateReady, hostStopped, teamViewer, teamFolderMissing
}

public enum SampleNotification: String, CaseIterable, Sendable {
    case conflict, keyStopped, addedToTeam, wentBack, cantOpen
}

public struct HistoryEntry: Equatable, Identifiable, Sendable {
    public enum Kind: String, Sendable { case uploaded, downloaded, renamed, removed, conflict, settled }
    public let id: String
    public let kind: Kind
    public let title: String
    public let detail: String
    public let time: String
    /// The day the page groups the entry under ("Today", or a date).
    public var day: String = Strings.History.today
}

/// One page of a folder's history, newest first. next is the cursor for older
/// entries, nil when there are none (08-sync.md 8.39).
public struct HistoryPage: Equatable, Sendable {
    public var entries: [HistoryEntry]
    public var next: String?
    public init(entries: [HistoryEntry], next: String?) {
        self.entries = entries
        self.next = next
    }
}
