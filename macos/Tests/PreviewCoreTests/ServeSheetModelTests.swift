import XCTest
@testable import PreviewCore

@MainActor
final class ServeSheetModelTests: XCTestCase {
    private func sample() -> (host: SampleHost, model: AppModel) {
        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: host.current)
        model.folderChooser = SampleFolderChooser()
        return (host, model)
    }

    func testANewSheetStartsAtTheFirstSamplePathWithItsDefaultName() {
        let (host, model) = sample()
        let sheet = ServeSheetModel(folderID: nil, model: model)

        let expected = ServeRules.path(folderID: nil, pick: ServeRules.firstPick(in: host.current), in: host.current)
        XCTAssertEqual(sheet.path, expected)
        XCTAssertEqual(sheet.path, SampleData.servePaths[1])
        XCTAssertEqual(sheet.name, "apps")
        XCTAssertTrue(sheet.nameEditable)
        XCTAssertNil(sheet.pathProblem)
        XCTAssertNil(sheet.nameProblem)
        XCTAssertNil(sheet.otherProblem)
        XCTAssertTrue(sheet.ready)
    }

    func testATakenNameIsTheNameProblem() async {
        let (_, model) = sample()
        let sheet = ServeSheetModel(folderID: nil, model: model)
        sheet.name = "prototypes"

        await sheet.check()

        XCTAssertEqual(sheet.nameProblem, Strings.Sheets.taken("prototypes"))
        XCTAssertNil(sheet.pathProblem)
        XCTAssertNil(sheet.otherProblem)
        XCTAssertFalse(sheet.ready)
    }

    func testChooseMovesToTheNextSamplePathAndResetsTheName() async {
        let (_, model) = sample()
        let sheet = ServeSheetModel(folderID: nil, model: model)
        sheet.name = "something-else"

        await sheet.choose()

        XCTAssertEqual(sheet.path, SampleData.servePaths[2])
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

    func testAKnownFolderKeepsItsNameReadOnly() {
        let (_, model) = sample()
        let sheet = ServeSheetModel(folderID: "prototypes", model: model)

        XCTAssertFalse(sheet.nameEditable)
        XCTAssertEqual(sheet.name, "prototypes")
        XCTAssertEqual(sheet.path, "~/Sites/prototypes")
    }
}
