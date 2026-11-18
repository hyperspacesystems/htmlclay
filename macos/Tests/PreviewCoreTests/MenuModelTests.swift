import XCTest
@testable import PreviewCore

final class MenuModelTests: XCTestCase {
    private static let fixtures = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .appendingPathComponent("Fixtures/v1")

    private struct Case: Decodable {
        var name: String
        var snapshot: String
        var update: MenuUpdatePhase
        var recoveredChanges: Int
        var rows: [MenuRow]
    }

    private struct Fixture: Decodable {
        var v: Int
        var cases: [Case]
    }

    func testMatchesTheSharedFixture() throws {
        let data = try Data(contentsOf: Self.fixtures.appendingPathComponent("menu.json"))
        let fixture = try JSONDecoder().decode(Fixture.self, from: data)
        XCTAssertFalse(fixture.cases.isEmpty)
        for one in fixture.cases {
            let path = Self.fixtures.appendingPathComponent(one.snapshot + ".snapshot.json")
            let wire = try HostWireCoding.decoder().decode(WireSnapshot.self, from: Data(contentsOf: path))
            var snapshot = HostSnapshotAdapter.snapshot(from: wire)
            snapshot.recoveredChanges = one.recoveredChanges
            XCTAssertEqual(MenuModel.rows(MenuInput(snapshot), phase: one.update), one.rows, one.name)
        }
    }

    func testRecentIsAlwaysThere() {
        let input = MenuInput(syncConnected: false, canPause: false, waitingTotal: 0,
                              waitingIDs: [], documentIDs: [], recoveredChanges: 0)
        let rows = MenuModel.rows(input, phase: MenuUpdatePhase())
        let recent = rows.first { $0.id == "recent" }
        XCTAssertEqual(recent?.children, [MenuRow(id: "noRecent")])
        XCTAssertEqual(recent?.children?.first?.enabled, false)
    }
}
