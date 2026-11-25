import Foundation

public struct MenuRow: Equatable, Sendable, Codable {
    public var id: String
    public var kind: String?
    public var enabled: Bool
    public var children: [MenuRow]?

    public init(id: String, kind: String? = nil, enabled: Bool = false, children: [MenuRow]? = nil) {
        self.id = id
        self.kind = kind
        self.enabled = enabled
        self.children = children
    }
}

public struct MenuUpdatePhase: Equatable, Sendable, Codable {
    public var checking: Bool
    public var ready: String
    /// The newer version downloading now, or empty.
    public var downloading: String

    public init(checking: Bool = false, ready: String = "", downloading: String = "") {
        self.checking = checking
        self.ready = ready
        self.downloading = downloading
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        checking = try container.decode(Bool.self, forKey: .checking)
        ready = try container.decode(String.self, forKey: .ready)
        downloading = try container.decodeIfPresent(String.self, forKey: .downloading) ?? ""
    }
}

/// What the one menu needs from a snapshot.
public struct MenuInput: Equatable, Sendable {
    public var syncConnected: Bool
    public var canPause: Bool
    public var waitingTotal: Int
    public var waitingIDs: [String]
    public var documentIDs: [String]
    public var recoveredChanges: Int

    public init(syncConnected: Bool, canPause: Bool, waitingTotal: Int, waitingIDs: [String], documentIDs: [String], recoveredChanges: Int) {
        self.syncConnected = syncConnected
        self.canPause = canPause
        self.waitingTotal = waitingTotal
        self.waitingIDs = waitingIDs
        self.documentIDs = documentIDs
        self.recoveredChanges = recoveredChanges
    }
}

public enum MenuModel {
    public static let separator = "-"
    static let listCap = 8

    public static func rows(_ input: MenuInput, phase: MenuUpdatePhase) -> [MenuRow] {
        var status = MenuRow(id: "status", kind: "version")
        if !phase.ready.isEmpty {
            status = MenuRow(id: "status", kind: "ready", enabled: true)
        } else if !phase.downloading.isEmpty {
            status.kind = "downloading"
        } else if phase.checking {
            status.kind = "checking"
        }
        var rows: [MenuRow] = [status, MenuRow(id: separator)]
        if input.syncConnected {
            rows.append(MenuRow(id: "syncLine"))
            if input.canPause { rows.append(MenuRow(id: "pauseSync", enabled: true)) }
        }
        if input.waitingTotal > 0, !input.waitingIDs.isEmpty {
            rows.append(MenuRow(id: "needsYou", enabled: true,
                                children: input.waitingIDs.prefix(listCap).map { MenuRow(id: $0, enabled: true) }))
        }
        rows.append(MenuRow(id: separator))
        rows.append(MenuRow(id: "openDashboard", enabled: true))
        rows.append(MenuRow(id: "openFile", enabled: true))
        let recent = input.documentIDs.prefix(listCap).map { MenuRow(id: $0, enabled: true) }
        rows.append(MenuRow(id: "recent", enabled: true, children: recent.isEmpty ? [MenuRow(id: "noRecent")] : Array(recent)))
        if input.recoveredChanges > 0 { rows.append(MenuRow(id: "recoveredChanges", enabled: true)) }
        rows.append(MenuRow(id: separator))
        rows.append(MenuRow(id: "settings", enabled: true))
        rows.append(MenuRow(id: "checkForUpdates", enabled: !phase.checking && phase.downloading.isEmpty))
        rows.append(MenuRow(id: separator))
        rows.append(MenuRow(id: "quit", enabled: true))
        var out: [MenuRow] = []
        for row in rows where !(row.id == separator && (out.isEmpty || out.last?.id == separator)) {
            out.append(row)
        }
        while out.last?.id == separator { out.removeLast() }
        return out
    }
}

extension MenuInput {
    /// The one menu as a snapshot describes it.
    public init(_ snapshot: HostSnapshot) {
        self.init(syncConnected: snapshot.sync.connected,
                  canPause: snapshot.hostCan(HostActionType.syncSetPaused),
                  waitingTotal: snapshot.totalCount,
                  waitingIDs: snapshot.waiting.map(\.id),
                  documentIDs: snapshot.documents.map(\.id),
                  recoveredChanges: snapshot.recoveredChanges)
    }
}
