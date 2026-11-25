import XCTest
@testable import PreviewCore

/// 08-sync.md 8.43: what the live Sync pane reads from the host and asks of it.
final class SyncPaneLiveTests: XCTestCase {
    private static let fixtures = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .appendingPathComponent("Fixtures/v1")

    private func synced(keyStore: String?) throws -> SyncState {
        let data = try Data(contentsOf: Self.fixtures.appendingPathComponent("syncedWithTeams.snapshot.json"))
        var wire = try HostWireCoding.decoder().decode(WireSnapshot.self, from: data)
        wire.sync.keyStore = keyStore
        return HostSnapshotAdapter.snapshot(from: wire).sync
    }

    func testTheKeyInAFileIsSaidAndTheSystemStoreIsNot() throws {
        XCTAssertTrue(try synced(keyStore: "file").keyInFile)
        XCTAssertFalse(try synced(keyStore: "system").keyInFile)
        XCTAssertFalse(try synced(keyStore: nil).keyInFile, "an older host says nothing about where the key is")
    }

    func testTheHostSendsWhereTheKeyIs() throws {
        let data = try Data(contentsOf: Self.fixtures.appendingPathComponent("syncedWithTeams.snapshot.json"))
        let raw = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        let sync = try XCTUnwrap(raw["sync"] as? [String: Any])
        XCTAssertNotNil(sync["keyStore"], "the fixture from the Go host carries sync.keyStore")
    }

    func testTheSyncLogRowRevealsThroughTheHost() throws {
        XCTAssertEqual(HostActionWire.wire(.revealSyncLog)?.type, "sync.revealLog")
        let actions = try String(contentsOf: Self.fixtures.appendingPathComponent("actions.json"), encoding: .utf8)
        XCTAssertTrue(actions.contains("\"sync.revealLog\""), "the host's action list has sync.revealLog")
        XCTAssertTrue(SettingsControls.showsSyncLog(isSample: false, can: true))
        XCTAssertFalse(SettingsControls.showsSyncLog(isSample: false, can: false), "a host that cannot reveal it shows no row")
        XCTAssertTrue(SettingsControls.showsSyncLog(isSample: true, can: false))
    }
}
