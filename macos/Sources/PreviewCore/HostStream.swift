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
