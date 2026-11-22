import Foundation

/// Splits bytes from the event stream into complete lines. Bytes after the last newline
/// wait for the next chunk. "\r\n" and "\n" both end a line; empty lines are dropped.
public struct HostLineBuffer: Sendable {
    private var buffer = Data()

    public init() {}

    public mutating func append(_ chunk: Data) -> [Data] {
        buffer.append(chunk)
        let parts = buffer.split(separator: 0x0A, omittingEmptySubsequences: false)
        let rest: Data.SubSequence = parts.last ?? Data()
        buffer = Data(rest)
        var lines: [Data] = []
        for part in parts.dropLast() {
            let line = Self.trimmed(part)
            if !line.isEmpty { lines.append(line) }
        }
        return lines
    }

    /// What is left when the stream closes, if it is not empty (a last line with no newline).
    public mutating func finish() -> Data? {
        let line = Self.trimmed(buffer)
        buffer.removeAll()
        return line.isEmpty ? nil : line
    }

    private static func trimmed(_ slice: Data) -> Data {
        var line = Data(slice)
        if line.last == 0x0D { line.removeLast() }
        return line
    }
}

public enum HostStreamChange: Equatable, Sendable {
    /// Show this snapshot.
    case show(WireSnapshot)
    /// The host restarted (a new epoch). Comes before the .show of its first snapshot.
    case newEpoch(old: String, new: String)
    /// The host said goodbye; reason as sent ("quit").
    case ended(reason: String)
    /// A snapshot in a format this app does not read (v != 1). Nothing is shown.
    case unsupported(version: Int)
    /// Something to tell the person once, as a notification (08-sync.md decision 19).
    case notice(HostNotice)
    /// Update was pressed on a browser page: restart into the downloaded update.
    case restartToUpdate
    case openSettings(SettingsPane)
}

/// A conflict, a key the server stopped accepting, or a team the person was
/// added to. The host sends each once; id is stable across restarts.
public struct HostNotice: Equatable, Sendable {
    public enum Kind: String, Sendable { case conflict, keyStopped, addedToTeam }
    public var id: String
    public var kind: Kind
    public var folderID: String?
    /// The file's path for a conflict, the team's name for addedToTeam.
    public var name: String
    public var detail: String
    public init(id: String, kind: Kind, folderID: String? = nil, name: String = "", detail: String = "") {
        self.id = id; self.kind = kind; self.folderID = folderID; self.name = name; self.detail = detail
    }

    /// Where the card's button takes the person: a conflict to the dashboard's
    /// Needs you page, a stopped key to Settings › Sync to replace it, a new team
    /// to Settings › Folders to set it up.
    public var opens: HostNoticeTarget {
        switch kind {
        case .conflict: return .dashboard(route: "/needs-you")
        case .keyStopped: return .settings(.sync)
        case .addedToTeam: return .settings(.folders)
        }
    }
}

public enum HostNoticeTarget: Equatable, Sendable {
    case dashboard(route: String)
    case settings(SettingsPane)
}

/// What the app has shown so far. Pure value; the reducer is the only writer.
public struct HostStreamState: Equatable, Sendable {
    public private(set) var epoch: String = ""
    /// -1 before the first snapshot.
    public private(set) var rev: Int = -1
    public private(set) var ended = false

    public init() {}

    /// The since= value for reconnecting to the same host process; 0 before the first snapshot.
    public var since: Int { max(rev, 0) }

    /// One line from the stream. Throws only when the line is not JSON or has no type.
    public mutating func receive(line: Data) throws -> [HostStreamChange] {
        let event = try HostWireCoding.decoder().decode(WireEvent.self, from: line)
        switch event.type {
        case "snapshot":
            guard let snapshot = event.snapshot else {
                throw DecodingError.dataCorrupted(.init(
                    codingPath: [],
                    debugDescription: "a snapshot event has no snapshot"
                ))
            }
            return accept(snapshot)
        case "ping":
            return []
        case "notice":
            // A kind this app does not know yet is left for a newer app to show.
            guard let id = event.id, let kind = event.kind.flatMap(HostNotice.Kind.init(rawValue:)) else { return [] }
            return [.notice(HostNotice(id: id, kind: kind, folderID: event.folderID, name: event.name ?? "", detail: event.detail ?? ""))]
        case "restartToUpdate":
            return [.restartToUpdate]
        case "openSettings":
            let pane = event.detail.flatMap(SettingsPane.init(rawValue:)) ?? .general
            return [.openSettings(pane)]
        case "bye":
            ended = true
            return [.ended(reason: event.reason ?? "")]
        default:
            return []
        }
    }

    /// A snapshot from any source (stream line or action reply).
    public mutating func accept(_ snapshot: WireSnapshot) -> [HostStreamChange] {
        if snapshot.v != 1 {
            return [.unsupported(version: snapshot.v)]
        }
        if epoch.isEmpty {
            epoch = snapshot.epoch
            rev = snapshot.rev
            ended = false
            return [.show(snapshot)]
        }
        if snapshot.epoch != epoch {
            let old = epoch
            epoch = snapshot.epoch
            rev = snapshot.rev
            ended = false
            return [.newEpoch(old: old, new: snapshot.epoch), .show(snapshot)]
        }
        if snapshot.rev > rev {
            rev = snapshot.rev
            return [.show(snapshot)]
        }
        return []
    }
}
