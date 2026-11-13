import Foundation

public enum HostWireCoding {
    public static func decoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let text = try decoder.singleValueContainer().decode(String.self)
            guard let date = date(from: text) else {
                throw DecodingError.dataCorruptedError(in: try decoder.singleValueContainer(), debugDescription: "not an RFC 3339 date")
            }
            return date
        }
        return decoder
    }

    public static func encoder() -> JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .custom { date, encoder in
            var container = encoder.singleValueContainer()
            try container.encode(string(from: date))
        }
        return encoder
    }

    private static func formatter(fractional: Bool) -> ISO8601DateFormatter {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = fractional ? [.withInternetDateTime, .withFractionalSeconds] : [.withInternetDateTime]
        return formatter
    }

    private static func date(from text: String) -> Date? {
        formatter(fractional: true).date(from: text) ?? formatter(fractional: false).date(from: text)
    }

    private static func string(from date: Date) -> String {
        formatter(fractional: true).string(from: date)
    }
}

public struct WireSnapshot: Codable, Equatable, Sendable {
    public var v: Int
    public var rev: Int
    public var epoch: String
    public var host: WireHostState
    public var documents: [WireDocument]
    public var folders: [WireFolder]
    public var programs: [WireProgram]
    public var requests: [WireRequest]
    public var waiting: [WireWaitingItem]
    public var counts: WireCounts
    public var sync: WireSyncState
    public var update: WireUpdateState
    public var preferences: WirePreferences
    public var move: WireMove?

    enum CodingKeys: String, CodingKey {
        case v, rev, epoch, host, documents, folders, programs, requests, waiting, counts, sync, update, preferences, move
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(v, forKey: .v)
        try container.encode(rev, forKey: .rev)
        try container.encode(epoch, forKey: .epoch)
        try container.encode(host, forKey: .host)
        try container.encode(documents, forKey: .documents)
        try container.encode(folders, forKey: .folders)
        try container.encode(programs, forKey: .programs)
        try container.encode(requests, forKey: .requests)
        try container.encode(waiting, forKey: .waiting)
        try container.encode(counts, forKey: .counts)
        try container.encode(sync, forKey: .sync)
        try container.encode(update, forKey: .update)
        try container.encode(preferences, forKey: .preferences)
        try container.encode(move, forKey: .move)
    }
}

public struct WireHostState: Codable, Equatable, Sendable {
    public var version: String
    public var os: String
    public var port: Int
    public var dashboardURL: String
    public var service: String
    public var stopped: Bool
    public var can: [String: Bool]
}

public struct WireDocument: Codable, Equatable, Sendable {
    public var id: String
    public var name: String
    public var place: String
    public var folderID: String?
    public var url: String
    public var openNow: Bool
    public var openedAt: Date?
    public var fileMissing: Bool
    public var versionsKept: Int

    enum CodingKeys: String, CodingKey {
        case id, name, place, folderID, url, openNow, openedAt, fileMissing, versionsKept
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(name, forKey: .name)
        try container.encode(place, forKey: .place)
        try container.encode(folderID, forKey: .folderID)
        try container.encode(url, forKey: .url)
        try container.encode(openNow, forKey: .openNow)
        try container.encode(openedAt, forKey: .openedAt)
        try container.encode(fileMissing, forKey: .fileMissing)
        try container.encode(versionsKept, forKey: .versionsKept)
    }
}

public struct WireOrigin: Codable, Equatable, Sendable {
    public var kind: String
    public var username: String?
    public var team: String?
}

public struct WireFolder: Codable, Equatable, Sendable {
    public var id: String
    public var name: String
    public var path: String
    public var displayPath: String
    public var origin: WireOrigin
    public var parentID: String?
    public var trusted: Bool
    public var served: Bool
    public var synced: Bool
    public var address: String
    public var url: String
    public var state: String
    public var documentCount: Int
    public var storageBytes: Int
    public var sync: WireFolderSync
    public var can: WireFolderCan
    public var why: WireFolderWhy

    enum CodingKeys: String, CodingKey {
        case id, name, path, displayPath, origin, parentID, trusted, served, synced, address, url, state, documentCount, storageBytes, sync, can, why
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(name, forKey: .name)
        try container.encode(path, forKey: .path)
        try container.encode(displayPath, forKey: .displayPath)
        try container.encode(origin, forKey: .origin)
        try container.encode(parentID, forKey: .parentID)
        try container.encode(trusted, forKey: .trusted)
        try container.encode(served, forKey: .served)
        try container.encode(synced, forKey: .synced)
        try container.encode(address, forKey: .address)
        try container.encode(url, forKey: .url)
        try container.encode(state, forKey: .state)
        try container.encode(documentCount, forKey: .documentCount)
        try container.encode(storageBytes, forKey: .storageBytes)
        try container.encode(sync, forKey: .sync)
        try container.encode(can, forKey: .can)
        try container.encode(why, forKey: .why)
    }
}

public struct WireFolderSync: Codable, Equatable, Sendable {
    public var state: String
    public var files: Int
    public var lastFinishedAt: Date?
    public var uploadedToday: Int
    public var downloadedToday: Int
    public var role: String?

    enum CodingKeys: String, CodingKey {
        case state, files, lastFinishedAt, uploadedToday, downloadedToday, role
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(state, forKey: .state)
        try container.encode(files, forKey: .files)
        try container.encode(lastFinishedAt, forKey: .lastFinishedAt)
        try container.encode(uploadedToday, forKey: .uploadedToday)
        try container.encode(downloadedToday, forKey: .downloadedToday)
        try container.encodeIfPresent(role, forKey: .role)
    }
}

public struct WireFolderCan: Codable, Equatable, Sendable {
    public var serve: Bool
    public var stopServing: Bool
    public var sync: Bool
    public var stopSyncing: Bool
    public var stopTrusting: Bool
    public var merge: Bool
    public var openWorkspace: Bool
}

public struct WireFolderWhy: Codable, Equatable, Sendable {
    public var serve: WireReason?
    public var sync: WireReason?
    public var stopTrusting: WireReason?
    public var merge: WireReason?
}

public struct WireReason: Codable, Equatable, Sendable {
    public var code: String
    public var text: String
}

public struct WireProgram: Codable, Equatable, Sendable {
    public var id: String
    public var name: String
    public var path: String
    public var fileMissing: Bool
    public var allowedDocuments: [String]
    public var allTrustedFolders: Bool
    public var removedDocuments: [String]
}

public struct WireRequest: Codable, Equatable, Sendable {
    public var id: String
    public var kind: String
    public var revision: Int
    public var documentID: String?
    public var document: String
    public var program: String
    public var folder: String
    public var path: String
    public var detail: String
    public var answers: [String]

    enum CodingKeys: String, CodingKey {
        case id, kind, revision, documentID, document, program, folder, path, detail, answers
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(kind, forKey: .kind)
        try container.encode(revision, forKey: .revision)
        try container.encode(documentID, forKey: .documentID)
        try container.encode(document, forKey: .document)
        try container.encode(program, forKey: .program)
        try container.encode(folder, forKey: .folder)
        try container.encode(path, forKey: .path)
        try container.encode(detail, forKey: .detail)
        try container.encode(answers, forKey: .answers)
    }
}

public struct WireWaitingItem: Codable, Equatable, Sendable {
    public var id: String
    public var kind: String
    public var pane: String
    public var earnsDot: Bool
    public var seen: Bool
    public var title: String
    public var detail: String
    public var subjectID: String?

    enum CodingKeys: String, CodingKey {
        case id, kind, pane, earnsDot, seen, title, detail, subjectID
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(kind, forKey: .kind)
        try container.encode(pane, forKey: .pane)
        try container.encode(earnsDot, forKey: .earnsDot)
        try container.encode(seen, forKey: .seen)
        try container.encode(title, forKey: .title)
        try container.encode(detail, forKey: .detail)
        try container.encode(subjectID, forKey: .subjectID)
    }
}

public struct WireCounts: Codable, Equatable, Sendable {
    public var total: Int
    public var byPane: [String: Int]
    public var dot: Bool
}

public struct WireSyncState: Codable, Equatable, Sendable {
    public var connected: Bool
    public var username: String
    public var keySuffix: String
    public var paused: Bool
    public var syncingFiles: Int
    public var offlineChanges: Int
    public var viewOnlyTeams: Int
    public var lastFinishedAt: Date?
    public var fullName: String

    enum CodingKeys: String, CodingKey {
        case connected, username, keySuffix, paused, syncingFiles, offlineChanges, viewOnlyTeams, lastFinishedAt, fullName
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(connected, forKey: .connected)
        try container.encode(username, forKey: .username)
        try container.encode(keySuffix, forKey: .keySuffix)
        try container.encode(paused, forKey: .paused)
        try container.encode(syncingFiles, forKey: .syncingFiles)
        try container.encode(offlineChanges, forKey: .offlineChanges)
        try container.encode(viewOnlyTeams, forKey: .viewOnlyTeams)
        try container.encode(lastFinishedAt, forKey: .lastFinishedAt)
        try container.encode(fullName, forKey: .fullName)
    }
}

public struct WireUpdateState: Codable, Equatable, Sendable {
    public var current: String
    public var ready: String?
    public var checkedAt: Date?
    public var automatic: Bool
    public var finishing: Bool

    enum CodingKeys: String, CodingKey {
        case current, ready, checkedAt, automatic, finishing
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(current, forKey: .current)
        try container.encode(ready, forKey: .ready)
        try container.encode(checkedAt, forKey: .checkedAt)
        try container.encode(automatic, forKey: .automatic)
        try container.encode(finishing, forKey: .finishing)
    }
}

public struct WirePreferences: Codable, Equatable, Sendable {
    public var openAtLogin: Bool
    public var openAtLoginChosen: Bool
    public var aiEditing: Bool
    public var appearance: String
    public var yourName: String
    public var useName: Bool
}

public struct WireMove: Codable, Equatable, Sendable {
    public var state: String
    public var folders: [WireMoveFolder]
}

public struct WireMoveFolder: Codable, Equatable, Sendable {
    public var id: String
    public var name: String
    public var port: Int
    public var address: String
}

public struct WireEvent: Decodable, Sendable {
    public var type: String
    public var rev: Int?
    public var snapshot: WireSnapshot?
    public var reason: String?
}

public struct WireActionList: Codable {
    public var v: Int
    public var actions: [WireAction]
}

public struct WireAction: Codable, Equatable, Sendable {
    public var type: String
    public var `class`: String
}
