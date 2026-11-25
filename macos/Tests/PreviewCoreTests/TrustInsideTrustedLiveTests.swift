import XCTest
@testable import PreviewCore

/// The Trust sheet against the real host: the host asks for the acceptance the
/// sheet sends, and takes nothing less.
@MainActor
final class TrustInsideTrustedLiveTests: XCTestCase {
    override func tearDown() async throws {
        await LiveHost.stopAll()
    }

    private func makeFolder(_ url: URL) throws {
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        try Data("<!doctype html><title>\(url.lastPathComponent)</title><p>hello".utf8)
            .write(to: url.appendingPathComponent("index.html"))
    }

    func testTheHostTakesTheTrustOnlyWithTheWarningTheSheetShowed() async throws {
        let host = try await LiveHost.start()
        let notes = host.home.appendingPathComponent("notes")
        let drafts = notes.appendingPathComponent("drafts")
        let sibling = host.home.appendingPathComponent("notes-old")
        try makeFolder(notes)
        try makeFolder(drafts)
        try makeFolder(sibling)

        let model = AppModel(host: host.client, initial: try await host.client.load())
        guard case .done = await model.submit(.trustFolder(name: "notes", path: notes.path)) else {
            return XCTFail("trusting notes was refused")
        }
        let notesID = try XCTUnwrap(model.snapshot.folders.first { $0.fullPath == notes.path }?.id)

        guard case let .refused(refusal) = await host.client.perform(.trustFolder(name: "drafts", path: drafts.path)) else {
            return XCTFail("the host trusted a folder inside a trusted folder with nothing accepted")
        }
        XCTAssertEqual(refusal.code, "needs-confirmation")
        let untouched = try await host.client.load()
        XCTAssertNil(untouched.folders.first { $0.fullPath == drafts.path })

        let beside = TrustSheetModel(name: "notes-old", path: sibling.path, model: model)
        await beside.check()
        XCTAssertNil(beside.problem)
        XCTAssertNil(beside.insideTrusted, "a folder beside notes that shares its prefix was called inside it")

        let sheet = TrustSheetModel(name: "drafts", path: drafts.path, model: model)
        await sheet.check()
        XCTAssertEqual(sheet.insideTrusted, "notes")
        XCTAssertEqual(sheet.trustTitle, Strings.TrustSheet.trustNewAddress)

        let trusted = await sheet.trust()

        XCTAssertTrue(trusted, "the host refused the accepted trust: \(String(describing: sheet.problem))")
        let inner = try XCTUnwrap(model.snapshot.folders.first { $0.fullPath == drafts.path })
        XCTAssertEqual(inner.parentID, notesID, "drafts is not listed inside notes")
        XCTAssertEqual(inner.can?.merge, true, "drafts does not keep an address of its own that Merge could give up")
    }
}
