import XCTest
@testable import PreviewCore

private struct SplitMix64: RandomNumberGenerator {
    private var seed: UInt64

    init(seed: UInt64) {
        self.seed = seed
    }

    mutating func next() -> UInt64 {
        seed &+= 0x9E37_79B9_7F4A_7C15
        var z = seed
        z = (z ^ (z >> 30)) &* 0xBF58_476D_1CE4_E5B9
        z = (z ^ (z >> 27)) &* 0x94D0_49BB_1331_11EB
        return z ^ (z >> 31)
    }
}

final class HostStreamTests: XCTestCase {
    private static let fixtures = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .appendingPathComponent("Fixtures/v1")

    private let epoch = "0123456789abcdef"
    private let restartedEpoch = "fedcba9876543210"

    private func data(_ name: String) throws -> Data {
        try Data(contentsOf: Self.fixtures.appendingPathComponent(name))
    }

    private func fileLines(_ data: Data) -> [Data] {
        data.split(separator: 0x0A).map { part in
            var line = Data(part)
            if line.last == 0x0D { line.removeLast() }
            return line
        }
    }

    private func transcript() throws -> [Data] {
        fileLines(try data("events.v1.ndjson"))
    }

    private func event(_ line: Data) throws -> WireEvent {
        try HostWireCoding.decoder().decode(WireEvent.self, from: line)
    }

    private func snapshot(_ line: Data) throws -> WireSnapshot {
        try XCTUnwrap(try event(line).snapshot)
    }

    private func line(atRev rev: Int, in lines: [Data]) throws -> Data {
        let matches = try lines.filter { try event($0).type == "snapshot" && event($0).rev == rev }
        XCTAssertEqual(matches.count, 1, "rev \(rev)")
        return try XCTUnwrap(matches.first)
    }

    private func showRevs(_ changes: [HostStreamChange]) -> [Int] {
        changes.compactMap { change in
            if case .show(let snapshot) = change { return snapshot.rev }
            return nil
        }
    }

    private func feed(_ lines: [Data], into state: inout HostStreamState) throws -> [HostStreamChange] {
        var changes: [HostStreamChange] = []
        for line in lines {
            changes.append(contentsOf: try state.receive(line: line))
        }
        return changes
    }

    private func transcriptState() throws -> (HostStreamState, [Data]) {
        let lines = try transcript()
        var state = HostStreamState()
        _ = try feed(lines, into: &state)
        return (state, lines)
    }

    private func collect(_ data: Data, chunkedBy size: Int) -> [Data] {
        var buffer = HostLineBuffer()
        var lines: [Data] = []
        var index = data.startIndex
        while index < data.endIndex {
            let end = min(index + size, data.endIndex)
            lines.append(contentsOf: buffer.append(Data(data[index..<end])))
            index = end
        }
        if let last = buffer.finish() { lines.append(last) }
        return lines
    }

    func testTranscriptShowsEverySnapshotThenEnds() throws {
        let lines = try transcript()
        var state = HostStreamState()
        let changes = try feed(lines, into: &state)
        let shown = showRevs(changes)
        XCTAssertEqual(shown.count, 5)
        XCTAssertEqual(shown, shown.sorted())
        XCTAssertEqual(Set(shown).count, shown.count)
        let expected = try lines.filter { try event($0).type == "snapshot" }.map { try event($0).rev }
        XCTAssertEqual(shown, expected)
        XCTAssertEqual(shown, [0, 1, 3, 5, 7])
        XCTAssertEqual(changes.last, .ended(reason: "quit"))
        XCTAssertTrue(state.ended)
        XCTAssertEqual(state.rev, 7)
        XCTAssertEqual(state.since, 7)
    }

    func testStaleAndEqualAreIgnored() throws {
        let (state, lines) = try transcriptState()
        var live = state
        let stale = try line(atRev: 3, in: lines)
        let equal = try line(atRev: 7, in: lines)
        XCTAssertEqual(try live.receive(line: stale), [])
        XCTAssertEqual(live, state)
        XCTAssertEqual(try live.receive(line: equal), [])
        XCTAssertEqual(live, state)
    }

    func testReconnectReplayShowsNothingOld() throws {
        let lines = try transcript()
        var state = HostStreamState()
        var shown: [Int] = []
        shown.append(contentsOf: showRevs(try feed(Array(lines[0...3]), into: &state)))
        XCTAssertEqual(shown, [0, 1, 3, 5])
        shown.append(contentsOf: showRevs(try feed(Array(lines[2...5]), into: &state)))
        XCTAssertEqual(shown, [0, 1, 3, 5, 7])
        XCTAssertEqual(state.since, 7)
    }

    func testNewEpochIsShownAndAnnounced() throws {
        let (state, lines) = try transcriptState()
        var live = state
        var restarted = try snapshot(lines[0])
        restarted.epoch = restartedEpoch
        restarted.rev = 0
        XCTAssertEqual(live.accept(restarted), [
            .newEpoch(old: epoch, new: restartedEpoch),
            .show(restarted),
        ])
        XCTAssertEqual(live.epoch, restartedEpoch)
        XCTAssertEqual(live.rev, 0)
        XCTAssertFalse(live.ended)
        XCTAssertEqual(live.since, 0)

        let revOne = String(decoding: try line(atRev: 1, in: lines), as: UTF8.self)
        let newEpochLine = Data(revOne.replacingOccurrences(of: epoch, with: restartedEpoch).utf8)
        let changes = try live.receive(line: newEpochLine)
        XCTAssertEqual(showRevs(changes), [1])
        XCTAssertEqual(live.rev, 1)
    }

    func testNeverBackwards() throws {
        let lines = try transcript().filter { try event($0).type == "snapshot" }
        XCTAssertEqual(lines.count, 5)
        for seed in 1...50 {
            var generator = SplitMix64(seed: UInt64(seed))
            var shuffled = lines
            shuffled.shuffle(using: &generator)
            var state = HostStreamState()
            let shown = showRevs(try feed(shuffled, into: &state))
            XCTAssertFalse(shown.isEmpty, "seed \(seed)")
            XCTAssertEqual(shown, shown.sorted(), "seed \(seed)")
            XCTAssertEqual(Set(shown).count, shown.count, "seed \(seed)")
            XCTAssertEqual(state.rev, 7, "seed \(seed)")
        }
    }

    func testUnsupportedVersion() throws {
        let (state, lines) = try transcriptState()
        var live = state
        var future = try snapshot(try line(atRev: 5, in: lines))
        future.v = 2
        XCTAssertEqual(live.accept(future), [.unsupported(version: 2)])
        XCTAssertEqual(live, state)
        XCTAssertEqual(live.rev, 7)
    }

    func testRepliesAndStreamShareOneOrder() throws {
        let (_, lines) = try transcriptState()
        var state = HostStreamState()
        let reply = try snapshot(try line(atRev: 5, in: lines))
        XCTAssertEqual(showRevs(state.accept(reply)), [5])
        XCTAssertEqual(state.rev, 5)
        let changes = try feed(Array(lines[0...4]), into: &state)
        let shown = showRevs(changes)
        XCTAssertEqual(shown, [7])
        XCTAssertFalse(shown.contains(1))
        XCTAssertFalse(shown.contains(3))
        XCTAssertEqual(state.rev, 7)
    }

    func testLineBufferAnyChunking() throws {
        let data = try data("events.v1.ndjson")
        let expected = fileLines(data)
        XCTAssertEqual(expected.count, 6)
        XCTAssertEqual(String(decoding: expected.last ?? Data(), as: UTF8.self).contains("\"bye\""), true)
        for size in [1, 2, 3, 7, 64, 4096] {
            XCTAssertEqual(collect(data, chunkedBy: size), expected, "chunk size \(size)")
        }
        let text = String(decoding: data, as: UTF8.self)
        let crlf = Data(text.replacingOccurrences(of: "\n", with: "\r\n").utf8)
        XCTAssertEqual(collect(crlf, chunkedBy: 3), expected, "crlf")

        var buffer = HostLineBuffer()
        XCTAssertEqual(buffer.append(Data("a\n\nb".utf8)), [Data("a".utf8)])
        XCTAssertEqual(buffer.finish(), Data("b".utf8))
        XCTAssertNil(buffer.finish())
    }

    func testBadLineThrows() throws {
        var state = HostStreamState()
        XCTAssertThrowsError(try state.receive(line: Data("not json".utf8)))
        XCTAssertThrowsError(try state.receive(line: Data("{\"rev\":1}".utf8)))
        XCTAssertEqual(try state.receive(line: Data("{\"type\":\"ping\"}".utf8)), [])
        XCTAssertEqual(try state.receive(line: Data("{\"type\":\"later-type\"}".utf8)), [])
        XCTAssertThrowsError(try state.receive(line: Data("{\"type\":\"snapshot\",\"rev\":1}".utf8)))
    }
}
