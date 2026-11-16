import XCTest
@testable import PreviewCore

/// A host that answers folder.setUpTeam from a list, and records what it got.
@MainActor
private final class SetUpHost: HostClient {
    var answers: [HostOutcome] = []
    private(set) var performed: [HostAction] = []

    func load() async throws -> HostSnapshot { HostSnapshot() }
    func perform(_ action: HostAction) async -> HostOutcome {
        performed.append(action)
        return answers.isEmpty ? .done(HostSnapshot()) : answers.removeFirst()
    }
    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .failure(HostRefusal(code: "unsupported", text: "")) }
    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription { HostSubscription {} }
}

@MainActor
private final class OneFolderChooser: FolderChooser {
    let folder: String
    init(_ folder: String) { self.folder = folder }
    func first(in snapshot: HostSnapshot) -> String? { nil }
    func choose(after current: String?, in snapshot: HostSnapshot) async -> String? { folder }
    func chooseToTrust(after current: String?, in snapshot: HostSnapshot) async -> String? { nil }
    func chooseFile() async -> String? { nil }
}

@MainActor
final class SetUpSheetsTests: XCTestCase {
    private func sheet() -> (SetUpHost, SetUpSheetModel) {
        let host = SetUpHost()
        let model = AppModel(host: host, initial: HostSnapshot())
        model.folderChooser = OneFolderChooser("/Users/maya/Documents/maya")
        return (host, SetUpSheetModel(folderID: "0dd3adfe2b553e75", path: "~/hyperclay/maya", model: model))
    }

    func testANewFolderSetsUpInOneSendAtTheFullPath() async {
        let (host, form) = sheet()
        let done = await form.setUp()
        XCTAssertTrue(done)
        let home = NSHomeDirectory()
        XCTAssertEqual(host.performed, [.setUpTeamFolder(folderID: "0dd3adfe2b553e75", path: home + "/hyperclay/maya")])
        let wire = HostActionWire.wire(host.performed[0])
        XCTAssertEqual(wire?.type, "folder.setUpTeam")
        XCTAssertEqual(wire?.args["path"], .string(home + "/hyperclay/maya"))
        XCTAssertNil(wire?.args["existing"])
    }

    func testAFolderAlreadyThereShowsTheCountsBeforeTheSecondSend() async {
        let (host, form) = sheet()
        let counts = SyncCounts(toUpload: 3, toDownload: 12, differ: 1)
        host.answers = [.refused(HostRefusal(code: "confirm-needed", text: "Check what will sync, then confirm.", counts: counts))]
        await form.chooseExisting()
        XCTAssertTrue(form.existing)
        XCTAssertEqual(form.path, "/Users/maya/Documents/maya")

        let first = await form.setUp()
        XCTAssertFalse(first)
        XCTAssertEqual(form.counts, counts)
        XCTAssertNil(form.refusal)
        XCTAssertEqual(host.performed.last, .setUpTeamFolder(folderID: "0dd3adfe2b553e75", path: "/Users/maya/Documents/maya", existing: true, confirmed: false))

        let second = await form.setUp()
        XCTAssertTrue(second)
        XCTAssertEqual(host.performed.last, .setUpTeamFolder(folderID: "0dd3adfe2b553e75", path: "/Users/maya/Documents/maya", existing: true, confirmed: true))
        XCTAssertEqual(HostActionWire.wire(host.performed[1])?.args["confirmed"], .bool(true))
    }

    func testChoosingAgainAsksForTheCountsAgain() async {
        let (host, form) = sheet()
        host.answers = [.refused(HostRefusal(code: "confirm-needed", text: "", counts: SyncCounts(toUpload: 1, toDownload: 0, differ: 0)))]
        await form.chooseExisting()
        _ = await form.setUp()
        await form.chooseExisting()
        XCTAssertNil(form.counts)
        form.chooseNew("~/hyperclay/maya")
        XCTAssertFalse(form.existing)
    }

    func testARefusalShowsItsLine() async {
        let (host, form) = sheet()
        host.answers = [.refused(HostRefusal(code: "sync-folder-not-empty", text: "A team folder starts empty."))]
        let done = await form.setUp()
        XCTAssertFalse(done)
        XCTAssertEqual(form.refusal, "A team folder starts empty.")
        XCTAssertNil(form.counts)
    }

    func testAFinishedSetUpFollowsTheDownloadAndARefusalDoesNot() async {
        let (host, form) = sheet()
        host.answers = [.refused(HostRefusal(code: "sync-folder-not-empty", text: "no"))]
        _ = await form.setUp()
        XCTAssertFalse(form.following)
        _ = await form.setUp()
        XCTAssertTrue(form.following)
    }

    func testTheSheetFollowsTheFirstDownloadUntilItAnswers() {
        let running = FirstDownload(done: 3, total: 31)
        var folder = Folder(id: "f1", name: "Studio", path: "~/Studio", origin: .team(name: "Studio"), syncState: FolderSyncState.syncing)
        XCTAssertEqual(SetUpSheetModel.follow(folder, waited: 60), .running, "a download with no report yet keeps the sheet")
        folder.download = running
        XCTAssertEqual(SetUpSheetModel.follow(folder, waited: 60), .running, "a running download keeps the sheet")
        folder.download = nil
        folder.syncState = FolderSyncState.upToDate
        XCTAssertEqual(SetUpSheetModel.follow(folder, waited: 0.5), .finished, "a download that ended closes it")
        folder.onThisMac = false
        XCTAssertEqual(SetUpSheetModel.follow(folder, waited: 0.5), .running, "the host's next snapshot may still be coming")
        XCTAssertEqual(SetUpSheetModel.follow(folder, waited: 3), .failed, "a folder given back says the download failed")
        XCTAssertEqual(SetUpSheetModel.follow(nil, waited: 3), .failed)
        XCTAssertEqual(running.fraction, 3.0 / 31.0, accuracy: 0.0001)
        XCTAssertTrue(Strings.TeamSetup.downloading(FirstDownload(done: 3, total: 31, bytesDone: 1_500_000, bytesTotal: 12_000_000)).contains("3 of 31"))
    }

    func testTheHostsDownloadReachesTheFolder() throws {
        let url = URL(fileURLWithPath: #filePath).deletingLastPathComponent().appendingPathComponent("Fixtures/v1/syncedWithTeams.snapshot.json")
        var wire = try HostWireCoding.decoder().decode(WireSnapshot.self, from: try Data(contentsOf: url))
        let index = try XCTUnwrap(wire.folders.firstIndex { $0.state != "not-on-this-computer" && $0.synced })
        wire.folders[index].sync.download = FirstDownload(done: 3, total: 31, bytesDone: 1, bytesTotal: 2)
        let encoded = try JSONEncoder().encode(wire.folders[index].sync)
        XCTAssertTrue(String(decoding: encoded, as: UTF8.self).contains("\"download\""))
        let snapshot = HostSnapshotAdapter.snapshot(from: wire)
        XCTAssertEqual(snapshot.folder(wire.folders[index].id)?.download, FirstDownload(done: 3, total: 31, bytesDone: 1, bytesTotal: 2))
        XCTAssertNil(HostSnapshotAdapter.snapshot(from: try HostWireCoding.decoder().decode(WireSnapshot.self, from: try Data(contentsOf: url))).folder(wire.folders[index].id)?.download)
    }
}
