import XCTest
@testable import PreviewCore

final class HostWireTests: XCTestCase {
    private static let fixtures = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .appendingPathComponent("Fixtures/v1")

    private static let snapshotFixtures = [
        "fiveDocuments",
        "localUser",
        "newPerson",
        "servedFolder",
        "syncedWithTeams",
        "trustedFolder",
    ]

    private func data(_ name: String) throws -> Data {
        try Data(contentsOf: Self.fixtures.appendingPathComponent(name))
    }

    func testEverySnapshotFixtureDecodes() throws {
        for name in Self.snapshotFixtures {
            let data = try data("\(name).snapshot.json")
            let snapshot = try HostWireCoding.decoder().decode(WireSnapshot.self, from: data)
            XCTAssertEqual(snapshot.v, 1, name)
            let raw = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any], name)
            let folders = try XCTUnwrap(raw["folders"] as? [[String: Any]], name)
            XCTAssertEqual(snapshot.folders.count, folders.count, name)
        }
    }

    func testNothingIsLost() throws {
        for name in Self.snapshotFixtures {
            let data = try data("\(name).snapshot.json")
            let snapshot = try HostWireCoding.decoder().decode(WireSnapshot.self, from: data)
            let encoded = try HostWireCoding.encoder().encode(snapshot)
            let original = try JSONSerialization.jsonObject(with: data)
            let round = try JSONSerialization.jsonObject(with: encoded)
            var originalPaths: Set<String> = []
            var roundPaths: Set<String> = []
            Self.collectKeyPaths(in: original, prefix: "", into: &originalPaths)
            Self.collectKeyPaths(in: round, prefix: "", into: &roundPaths)
            XCTAssertEqual(
                roundPaths.subtracting(originalPaths), [],
                "\(name) re-encodes keys Go does not send"
            )
            XCTAssertEqual(
                originalPaths.subtracting(roundPaths), [],
                "\(name) loses keys Go sends"
            )
        }
    }

    func testEventTranscriptDecodes() throws {
        let data = try data("events.v1.ndjson")
        let text = try XCTUnwrap(String(data: data, encoding: .utf8))
        let lines = text.split(separator: "\n").map(String.init).filter { !$0.isEmpty }
        let decoder = HostWireCoding.decoder()
        let events = try lines.map { try decoder.decode(WireEvent.self, from: Data($0.utf8)) }
        let revisions = events.compactMap { $0.type == "snapshot" ? $0.rev : nil }
        XCTAssertGreaterThanOrEqual(revisions.count, 3)
        XCTAssertEqual(revisions, revisions.sorted())
        XCTAssertEqual(Set(revisions).count, revisions.count)
        XCTAssertTrue(events.contains { $0.snapshot != nil })
        XCTAssertEqual(events.last?.type, "bye")
    }

    func testActionListDecodes() throws {
        let list = try HostWireCoding.decoder().decode(WireActionList.self, from: try data("actions.json"))
        XCTAssertEqual(list.v, 1)
        XCTAssertEqual(list.actions.count, 39)
        let classes: Set<String> = ["safe", "revoke", "grant"]
        for action in list.actions {
            XCTAssertTrue(classes.contains(action.class), action.type)
        }
    }

    func testFractionalSecondsDecode() throws {
        let json = """
        {"id":"index","name":"index.html","place":"panphora","folderID":null,"url":"http://panphora.htmlclay.localhost:1212/index.html","openNow":true,"openedAt":"2026-11-13T12:00:00.123456789Z","fileMissing":false,"versionsKept":0}
        """
        let document = try HostWireCoding.decoder().decode(WireDocument.self, from: Data(json.utf8))
        XCTAssertNotNil(document.openedAt)
    }

    private static func collectKeyPaths(in object: Any, prefix: String, into paths: inout Set<String>) {
        if let dictionary = object as? [String: Any] {
            for (key, value) in dictionary {
                let path = prefix.isEmpty ? key : "\(prefix).\(key)"
                paths.insert(path)
                collectKeyPaths(in: value, prefix: path, into: &paths)
            }
        } else if let array = object as? [Any] {
            for value in array {
                collectKeyPaths(in: value, prefix: "\(prefix)[]", into: &paths)
            }
        }
    }
}
