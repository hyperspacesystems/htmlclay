import XCTest
import PreviewCore
@testable import PreviewUI

@MainActor
final class M3RequestNameTests: XCTestCase {
    private func asking() throws -> HostSnapshot {
        let transcript = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent("PreviewCoreTests/Fixtures/v1/events.v1.ndjson")
        let snapshots = try Data(contentsOf: transcript).split(separator: 0x0A).compactMap { line in
            try HostWireCoding.decoder().decode(WireEvent.self, from: Data(line)).snapshot
        }
        let asked = try XCTUnwrap(snapshots.first { !$0.requests.isEmpty })
        return HostSnapshotAdapter.snapshot(from: asked)
    }

    func testTheProgramsPaneNamesTheDocumentThatAsks() throws {
        let snapshot = try asking()
        let request = try XCTUnwrap(snapshot.requests.first)
        let item = try XCTUnwrap(snapshot.waiting.first)
        XCTAssertTrue(request.document.hasPrefix("/"))
        XCTAssertEqual(ProgramsPane.waitingTitle(item, snapshot: snapshot),
                       Strings.Programs.wants(document: "essay.htmlclay", program: "search"))
    }

    func testATeamRequestNamesTheDocumentThatAsks() throws {
        var request = try XCTUnwrap(try asking().requests.first)
        XCTAssertEqual(RequestBody.teamTitle(request),
                       Strings.Request.runNamed(document: "essay.htmlclay", program: "search"))
        request.program = ProgramControls.aiEditID
        XCTAssertEqual(RequestBody.teamTitle(request),
                       Strings.Request.useAIEditing(document: "essay.htmlclay"))
    }
}
