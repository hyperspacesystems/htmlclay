import XCTest
@testable import PreviewCore

/// A host that was interrupted: every answer is the retryable refusal a restart gives.
@MainActor
private final class InterruptedHost: HostClient {
    private(set) var performed: [HostAction] = []
    static let refusal = HostRefusal(code: "interrupted", text: "HTML Clay restarted before that finished. Try again.", retryable: true)

    func load() async throws -> HostSnapshot { HostSnapshot() }

    func perform(_ action: HostAction) async -> HostOutcome {
        performed.append(action)
        return .refused(Self.refusal)
    }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .failure(Self.refusal) }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        HostSubscription {}
    }
}

@MainActor
final class ServeSheetModelTests: XCTestCase {
    private func sample() -> (host: SampleHost, model: AppModel) {
        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: host.current)
        model.folderChooser = SampleFolderChooser()
        return (host, model)
    }

    func testANewSheetStartsEmptyAndTheCheckFillsTheDefaultName() async {
        let (host, model) = sample()
        let sheet = ServeSheetModel(folderID: nil, model: model)

        let expected = ServeRules.path(folderID: nil, pick: ServeRules.firstPick(in: host.current), in: host.current)
        XCTAssertEqual(sheet.path, expected)
        XCTAssertEqual(sheet.path, SampleData.servePaths[1])
        XCTAssertEqual(sheet.name, "")
        XCTAssertFalse(sheet.nameEdited)
        XCTAssertTrue(sheet.nameEditable)
        XCTAssertNil(sheet.pathProblem)
        XCTAssertNil(sheet.nameProblem)
        XCTAssertNil(sheet.otherProblem)
        XCTAssertTrue(sheet.ready)

        await sheet.check()

        XCTAssertEqual(sheet.name, "apps")
        XCTAssertFalse(sheet.nameEdited)
        XCTAssertTrue(sheet.address.contains("apps.htmlclay.localhost"))
    }

    func testATypedNameSurvivesTheCheck() async {
        let (_, model) = sample()
        let sheet = ServeSheetModel(folderID: nil, model: model)

        sheet.setName("my-site")
        await sheet.check()

        XCTAssertEqual(sheet.name, "my-site")
        XCTAssertTrue(sheet.nameEdited)
    }

    func testATakenNameIsTheNameProblem() async {
        let (_, model) = sample()
        let sheet = ServeSheetModel(folderID: nil, model: model)
        sheet.setName("prototypes")

        await sheet.check()

        XCTAssertEqual(sheet.nameProblem, Strings.Sheets.taken("prototypes"))
        XCTAssertNil(sheet.pathProblem)
        XCTAssertNil(sheet.otherProblem)
        XCTAssertFalse(sheet.ready)
    }

    func testChooseMovesToTheNextSamplePathAndClearsTheName() async {
        let (_, model) = sample()
        let sheet = ServeSheetModel(folderID: nil, model: model)
        sheet.setName("something-else")

        await sheet.choose()

        XCTAssertEqual(sheet.path, SampleData.servePaths[2])
        XCTAssertEqual(sheet.name, "")
        XCTAssertFalse(sheet.nameEdited)

        await sheet.check()

        XCTAssertEqual(sheet.name, ServeRules.defaultName(for: SampleData.servePaths[2]))
        XCTAssertEqual(sheet.name, "blog")
    }

    func testServeOnAGoodPairReturnsTrueAndServesTheFolder() async throws {
        let (host, model) = sample()
        let sheet = ServeSheetModel(folderID: nil, model: model)
        await sheet.check()
        XCTAssertFalse(sheet.address.isEmpty)

        let served = await sheet.serve()

        XCTAssertTrue(served)
        let folder = try XCTUnwrap(host.current.folders.first { $0.path == sheet.path })
        XCTAssertTrue(folder.served)
        XCTAssertTrue(folder.address.contains("apps.htmlclay.localhost"))
    }

    func testARetryableRefusalKeepsTheSheetReadyAndShowsTheText() async {
        let model = AppModel(host: InterruptedHost(), initial: HostSnapshot())
        model.folderChooser = SampleFolderChooser()
        let sheet = ServeSheetModel(folderID: nil, model: model)

        await sheet.check()

        XCTAssertEqual(sheet.otherProblem, InterruptedHost.refusal.text)
        XCTAssertTrue(sheet.ready)
        XCTAssertEqual(sheet.address, "")
    }

    func testARetryableRefusalFromServeKeepsTheSheetReady() async {
        let model = AppModel(host: InterruptedHost(), initial: HostSnapshot())
        model.folderChooser = SampleFolderChooser()
        let sheet = ServeSheetModel(folderID: nil, model: model)

        let served = await sheet.serve()

        XCTAssertFalse(served)
        XCTAssertEqual(sheet.otherProblem, InterruptedHost.refusal.text)
        XCTAssertTrue(sheet.ready)
    }

    func testAKnownFolderKeepsItsNameReadOnly() {
        let (_, model) = sample()
        let sheet = ServeSheetModel(folderID: "prototypes", model: model)

        XCTAssertFalse(sheet.nameEditable)
        XCTAssertEqual(sheet.name, "prototypes")
        XCTAssertEqual(sheet.path, "~/Sites/prototypes")
    }
}
