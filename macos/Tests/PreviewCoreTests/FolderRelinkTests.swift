import XCTest
@testable import PreviewCore

@MainActor
private final class RelinkHost: HostClient {
    private(set) var recorded: [HostAction] = []
    var checkAnswer: Result<HostCheckAnswer, HostRefusal> = .success(HostCheckAnswer())
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

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { checkAnswer }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription { HostSubscription {} }
}

@MainActor
private final class PickOnce: FolderChooser {
    var path: String

    init(path: String) { self.path = path }

    func first(in snapshot: HostSnapshot) -> String? { path }
    func choose(after current: String?, in snapshot: HostSnapshot) async -> String? { path }
    func chooseToTrust(after current: String?, in snapshot: HostSnapshot) async -> String? { path }
    func chooseFile() async -> String? { path }
    func chooseProgram() async -> String? { path }
}

@MainActor
final class FolderPageRelinkTests: XCTestCase {
    private let folder = Folder(id: "f1", name: "docs", path: "/Users/sam/docs",
                                origin: .local, onThisMac: true, missingOnDisk: true)

    private func model(_ host: RelinkHost) -> AppModel {
        AppModel(host: host, initial: HostSnapshot(folders: [folder]))
    }

    func testFindItOnlyWhereTheHostCanRelink() {
        XCTAssertTrue(FolderControls.canRelink(HostSnapshot(host: HostInfo(can: [HostActionType.relinkFolder: true]))))
        XCTAssertFalse(FolderControls.canRelink(HostSnapshot(host: HostInfo(can: [HostActionType.relinkFolder: false]))))
    }

    func testTheSheetSendsTheCodesItShowed() async {
        let host = RelinkHost()
        host.checkAnswer = .success(HostCheckAnswer(path: "/Users/sam/Projects/docs", warnings: [
            HostWarning(code: "different-folder", text: "This is a different folder."),
            HostWarning(code: "contains-trusted", text: "These folders inside it are already trusted."),
        ]))
        let sheet = FolderRelinkModel(folderID: "f1", path: "/Users/sam/docs", model: model(host))

        await sheet.check()

        XCTAssertEqual(sheet.path, "/Users/sam/Projects/docs")
        XCTAssertEqual(sheet.acknowledged, ["different-folder", "contains-trusted"])
        XCTAssertEqual(sheet.warningLines, ["This is a different folder.",
                                           "These folders inside it are already trusted."])
        XCTAssertTrue(sheet.ready)

        let done = await sheet.confirm()
        XCTAssertTrue(done)
        XCTAssertEqual(host.recorded, [.relinkFolder(folderID: "f1", path: "/Users/sam/Projects/docs",
                                                     acknowledged: ["different-folder", "contains-trusted"])])
    }

    func testTheSameFolderMovedAcknowledgesNothing() async {
        let host = RelinkHost()
        host.checkAnswer = .success(HostCheckAnswer(path: "/Users/sam/Projects/docs"))
        let sheet = FolderRelinkModel(folderID: "f1", path: "/Users/sam/Projects/docs", model: model(host))

        await sheet.check()

        XCTAssertEqual(sheet.warningLines, [])
        XCTAssertEqual(sheet.acknowledged, [])
        let done = await sheet.confirm()
        XCTAssertTrue(done)
        XCTAssertEqual(host.recorded, [.relinkFolder(folderID: "f1", path: "/Users/sam/Projects/docs", acknowledged: [])])
    }

    func testARefusalKeepsTheSheetAndShowsWhy() async {
        let host = RelinkHost()
        host.outcome = .refused(HostRefusal(code: "needs-confirmation", text: "This action needs your approval."))
        let sheet = FolderRelinkModel(folderID: "f1", path: "/Users/sam/Projects", model: model(host))

        let done = await sheet.confirm()

        XCTAssertFalse(done)
        XCTAssertEqual(sheet.path, "/Users/sam/Projects")
        XCTAssertEqual(sheet.problem, "This action needs your approval.")
        XCTAssertFalse(sheet.saving)
    }

    func testFindItPicksWhereTheFolderWent() async {
        let host = RelinkHost()
        let model = model(host)
        model.folderChooser = PickOnce(path: "/Users/sam/Projects/docs")

        FolderFlow.findIt(folder, model: model)

        let deadline = Date().addingTimeInterval(2)
        while model.sheet == nil && Date() < deadline {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(model.sheet, .relinkFolder(folderID: "f1", path: "/Users/sam/Projects/docs"))
    }
}
