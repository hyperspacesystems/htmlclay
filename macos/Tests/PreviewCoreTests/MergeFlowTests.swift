import XCTest
@testable import PreviewCore

@MainActor
private final class MergeHost: HostClient {
    private(set) var recorded: [HostAction] = []
    var outcome: HostOutcome = .done(HostSnapshot())
    private var current: HostSnapshot

    init(_ snapshot: HostSnapshot = HostSnapshot()) { current = snapshot }

    func load() async throws -> HostSnapshot { current }

    func perform(_ action: HostAction) async -> HostOutcome {
        recorded.append(action)
        if case let .done(next) = outcome {
            current = next
            return .done(next)
        }
        return outcome
    }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .success(HostCheckAnswer()) }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription { HostSubscription {} }
}

@MainActor
final class MergeFlowTests: XCTestCase {
    override func tearDown() async throws {
        await LiveHost.stopAll()
    }

    private func parentAndChild() -> HostSnapshot {
        HostSnapshot(folders: [
            Folder(id: "notes", name: "Notes", path: "/tmp/notes", origin: .local, documentCount: 4),
            Folder(id: "journal", name: "Journal", path: "/tmp/notes/Journal", origin: .local,
                   parentID: "notes", documentCount: 6, can: FolderCan(merge: true)),
        ])
    }

    func testOpeningTheSheetAsksTheHostNothing() async throws {
        let host = MergeHost(parentAndChild())
        let model = AppModel(host: host, initial: parentAndChild())

        MergeFlow.open(childID: "journal", model: model)
        try await Task.sleep(nanoseconds: 50_000_000)

        XCTAssertEqual(model.sheet, .merge(childID: "journal"))
        XCTAssertEqual(host.recorded, [], "opening the sheet sent an action")
    }

    func testTheSheetStaysOpenAfterARefusedFlush() async {
        let host = MergeHost(parentAndChild())
        host.outcome = .refused(HostRefusal(code: "unsaved-edits",
                                            text: "Two documents in Journal have changes that could not be saved.",
                                            retryable: true))
        let model = AppModel(host: host, initial: parentAndChild())
        MergeFlow.open(childID: "journal", model: model)
        let sheet = MergeSheetModel(childID: "journal", model: model)

        let merged = await sheet.confirm()

        XCTAssertFalse(merged)
        XCTAssertEqual(model.sheet, .merge(childID: "journal"), "the sheet closed on a refusal")
        XCTAssertEqual(sheet.problem, "Two documents in Journal have changes that could not be saved.")
        XCTAssertFalse(sheet.saving)
        XCTAssertTrue(sheet.ready)
        XCTAssertEqual(host.recorded, [.mergeIntoParent(folderID: "journal")])
    }

    func testTheSheetClosesOnlyAfterTheHostMerged() async {
        let host = MergeHost(parentAndChild())
        host.outcome = .done(HostSnapshot(folders: [
            Folder(id: "notes", name: "Notes", path: "/tmp/notes", origin: .local, documentCount: 10),
            Folder(id: "journal", name: "Journal", path: "/tmp/notes/Journal", origin: .local,
                   documentCount: 6, can: FolderCan(merge: false)),
        ]))
        let model = AppModel(host: host, initial: parentAndChild())
        MergeFlow.open(childID: "journal", model: model)
        let sheet = MergeSheetModel(childID: "journal", model: model)

        XCTAssertEqual(host.recorded, [], "the action went before the sheet's confirm")
        let merged = await sheet.confirm()

        XCTAssertTrue(merged)
        XCTAssertNil(model.sheet)
        XCTAssertNil(sheet.problem)
        XCTAssertEqual(host.recorded, [.mergeIntoParent(folderID: "journal")])
        XCTAssertEqual(model.snapshot.folder("notes")?.documentCount, 10)
    }

    func testTheSheetSaysTheDocumentsMoveAndWhatTheBrowserStoredStays() {
        let message = Strings.FolderPage.mergeMessage(child: "Journal", parent: "Notes", documents: 6)
        XCTAssertTrue(message.contains("will move to Notes's address"), message)
        XCTAssertTrue(message.contains("saved in the browser stays behind at Journal's old address"), message)
        XCTAssertTrue(message.contains("files on disk stay right where they are"), message)
        XCTAssertFalse(message.lowercased().contains("copied"), message)
        XCTAssertFalse(message.lowercased().contains("copy"), message)
        XCTAssertEqual(Strings.FolderPage.mergeTitle(child: "Journal", parent: "Notes"),
                       "Merge Journal into Notes?")
        XCTAssertEqual(Strings.FolderPage.mergeAction(parent: "Notes"), "Merge into Notes")
    }

    private func makeFolder(_ url: URL) throws {
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        try Data("<!doctype html><title>\(url.lastPathComponent)</title><p>hello".utf8)
            .write(to: url.appendingPathComponent("index.html"))
    }

    private func done(_ outcome: HostOutcome, _ message: String) throws -> HostSnapshot {
        guard case let .done(snapshot) = outcome else {
            XCTFail(message)
            throw LiveHostFailure.start(message)
        }
        return snapshot
    }

    func testTheFolderListFollowsTheMergedFolderByItsID() async throws {
        let host = try await LiveHost.start()
        let notes = host.home.appendingPathComponent("notes")
        let journal = notes.appendingPathComponent("journal")
        try makeFolder(notes)
        try makeFolder(journal)

        let model = AppModel(host: host.client, initial: try await host.client.load())
        XCTAssertTrue(model.can(HostActionType.merge), "the host does not run folder.merge")

        _ = try done(await model.submit(.trustFolder(name: "journal", path: journal.path)),
                     "trusting journal was refused")
        let journalID = try XCTUnwrap(model.snapshot.folders.first { $0.fullPath == journal.path }?.id)
        _ = try done(await model.submit(.setServed(folderID: journalID, on: true)),
                     "serving journal was refused")
        _ = try done(await model.submit(.setServed(folderID: journalID, on: false)),
                     "stopping journal's address was refused")
        _ = try done(await model.submit(.trustFolder(name: "notes", path: notes.path)),
                     "trusting notes was refused")
        let nested = try XCTUnwrap(model.snapshot.folder(journalID))
        let notesID = try XCTUnwrap(model.snapshot.folders.first { $0.fullPath == notes.path }?.id)
        XCTAssertEqual(nested.parentID, notesID, "journal is not nested in notes")
        XCTAssertEqual(nested.can?.merge, true, "the child's merge = \(String(describing: nested.can?.merge))")
        XCTAssertEqual(model.snapshot.folder(notesID)?.documentCount, 2)

        MergeFlow.open(childID: journalID, model: model)
        let sheet = MergeSheetModel(childID: journalID, model: model)
        let merged = await sheet.confirm()

        XCTAssertTrue(merged, "the host refused the merge: \(String(describing: sheet.problem))")
        XCTAssertNil(model.sheet)
        let after = try XCTUnwrap(model.snapshot.folder(journalID), "the folder list lost the merged folder")
        XCTAssertNil(after.parentID)
        XCTAssertTrue(model.snapshot.children(of: notesID).isEmpty)
        XCTAssertEqual(model.snapshot.folder(notesID)?.documentCount, 2)
        XCTAssertEqual(after.name, "journal")
    }
}
