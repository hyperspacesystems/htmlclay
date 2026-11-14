import XCTest
@testable import PreviewCore

/// A host that was interrupted: every answer is the retryable refusal a restart gives.
@MainActor
private final class InterruptedHost: HostClient {
    static let refusal = HostRefusal(code: "interrupted", text: "HTML Clay restarted before that finished. Try again.", retryable: true)

    func load() async throws -> HostSnapshot { HostSnapshot() }

    func perform(_ action: HostAction) async -> HostOutcome { .refused(Self.refusal) }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .failure(Self.refusal) }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        HostSubscription {}
    }
}

@MainActor
final class TrustSheetModelTests: XCTestCase {
    private func sample() -> (host: SampleHost, model: AppModel) {
        let host = SampleHost(persona: .trustedFolder)
        let model = AppModel(host: host, initial: host.current)
        model.folderChooser = SampleFolderChooser()
        return (host, model)
    }

    func testAMainFolderIsATrustedMainFolderAfterCheck() async {
        let (_, model) = sample()
        let sheet = TrustSheetModel(name: "Downloads", path: "~/Downloads", model: model)
        XCTAssertFalse(sheet.isMainFolder)

        await sheet.check()

        XCTAssertEqual(sheet.path, "~/Downloads")
        XCTAssertTrue(sheet.isMainFolder)
        XCTAssertTrue(sheet.insideNames.isEmpty)
        XCTAssertNil(sheet.problem)
        XCTAssertTrue(sheet.ready)
    }

    func testATrustedFolderInsideGivesItsNames() async {
        let (_, model) = sample()
        let sheet = TrustSheetModel(name: "Notes", path: "~/Documents/Notes", model: model)
        await sheet.check()

        XCTAssertEqual(sheet.insideNames, ["Journal", "Clips"])
        XCTAssertFalse(sheet.isMainFolder)
        XCTAssertNil(sheet.problem)
        XCTAssertTrue(sheet.ready)
    }

    func testChooseAnotherMovesToTheNextCandidate() async {
        let (_, model) = sample()
        let sheet = TrustSheetModel(name: "Downloads", path: "~/Downloads", model: model)
        await sheet.check()
        XCTAssertTrue(sheet.isMainFolder)

        let chose = await sheet.chooseAnother()

        XCTAssertTrue(chose)
        XCTAssertEqual(sheet.path, "~/Documents/Recipes")
        XCTAssertEqual(sheet.name, "Recipes")
        XCTAssertFalse(sheet.isMainFolder)
        XCTAssertTrue(sheet.insideNames.isEmpty)
    }

    func testARetryableRefusalKeepsTheSheetReadyAndShowsTheText() async {
        let model = AppModel(host: InterruptedHost(), initial: HostSnapshot())
        let sheet = TrustSheetModel(name: "Downloads", path: "~/Downloads", model: model)

        await sheet.check()

        XCTAssertEqual(sheet.problem, InterruptedHost.refusal.text)
        XCTAssertTrue(sheet.ready)
    }

    func testARetryableRefusalFromTrustKeepsTheSheetReady() async {
        let model = AppModel(host: InterruptedHost(), initial: HostSnapshot())
        let sheet = TrustSheetModel(name: "Downloads", path: "~/Downloads", model: model)

        let trusted = await sheet.trust()

        XCTAssertFalse(trusted)
        XCTAssertEqual(sheet.problem, InterruptedHost.refusal.text)
        XCTAssertTrue(sheet.ready)
    }

    func testTrustOnAGoodFolderReturnsTrueAndTrustsIt() async throws {
        let (host, model) = sample()
        XCTAssertFalse(host.current.localFolders.contains { $0.path == "~/Documents/Recipes" })
        let sheet = TrustSheetModel(name: "Recipes", path: "~/Documents/Recipes", model: model)
        await sheet.check()

        let trusted = await sheet.trust()

        XCTAssertTrue(trusted)
        XCTAssertNil(sheet.problem)
        let folder = try XCTUnwrap(host.current.folders.first { $0.path == "~/Documents/Recipes" })
        XCTAssertEqual(folder.name, "Recipes")
        XCTAssertTrue(model.snapshot.localFolders.contains { $0.path == "~/Documents/Recipes" })
    }
}
