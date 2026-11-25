import XCTest
@testable import PreviewCore

/// A host that records what the Trust sheet sends, answers every check the same
/// way, and answers each trust with the next scripted outcome.
@MainActor
private final class TrustHost: HostClient {
    private(set) var recorded: [HostAction] = []
    var checkAnswer: Result<HostCheckAnswer, HostRefusal> = .success(HostCheckAnswer())
    var outcomes: [HostOutcome] = []

    func load() async throws -> HostSnapshot { HostSnapshot() }

    func perform(_ action: HostAction) async -> HostOutcome {
        recorded.append(action)
        return outcomes.isEmpty ? .done(HostSnapshot()) : outcomes.removeFirst()
    }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { checkAnswer }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription { HostSubscription {} }
}

@MainActor
final class TrustInsideTrustedTests: XCTestCase {
    private static let path = "/Users/sam/Notes/Drafts"
    private static let inside = HostCheckAnswer(name: "Drafts", path: path, warnings: [
        HostWarning(code: "inside-trusted", text: "This folder is inside Notes.", names: ["Notes"]),
    ])
    private static let needsConfirmation = HostRefusal(code: "needs-confirmation", text: "This action needs your approval.")

    private func sheet(_ host: TrustHost) -> TrustSheetModel {
        TrustSheetModel(name: "Drafts", path: Self.path, model: AppModel(host: host, initial: HostSnapshot()))
    }

    private func trust(acknowledged: [String]) -> HostAction {
        .trustFolder(name: "Drafts", path: Self.path, acknowledged: acknowledged)
    }

    func testTheCheckShowsTheWarningAndTheButtonNamesWhatTheClickAccepts() async {
        let host = TrustHost()
        host.checkAnswer = .success(Self.inside)
        let sheet = sheet(host)
        XCTAssertNil(sheet.newAddressWarning)
        XCTAssertEqual(sheet.trustTitle, Strings.Request.trustNamed("Drafts"))
        XCTAssertEqual(sheet.acknowledged, [])

        await sheet.check()

        XCTAssertEqual(sheet.insideTrusted, "Notes")
        XCTAssertEqual(sheet.newAddressWarning, Strings.TrustSheet.insideTrusted("Drafts", around: "Notes"))
        XCTAssertEqual(sheet.trustTitle, Strings.TrustSheet.trustNewAddress)
        XCTAssertEqual(sheet.acknowledged, ["inside-trusted"])
        XCTAssertNil(sheet.problem)
        XCTAssertTrue(sheet.ready)
        XCTAssertEqual(host.recorded, [])
    }

    func testTheClickUnderTheWarningSendsTheAcknowledgement() async {
        let host = TrustHost()
        host.checkAnswer = .success(Self.inside)
        let sheet = sheet(host)
        await sheet.check()

        let trusted = await sheet.trust()

        XCTAssertTrue(trusted)
        XCTAssertEqual(host.recorded, [trust(acknowledged: ["inside-trusted"])])
    }

    func testATrustBeforeTheWarningWasShownAcceptsNothingAndThenShowsIt() async {
        let host = TrustHost()
        host.checkAnswer = .success(Self.inside)
        host.outcomes = [.refused(Self.needsConfirmation)]
        let sheet = sheet(host)

        let first = await sheet.trust()

        XCTAssertFalse(first)
        XCTAssertEqual(host.recorded, [trust(acknowledged: [])])
        XCTAssertNil(sheet.problem)
        XCTAssertEqual(sheet.newAddressWarning, Strings.TrustSheet.insideTrusted("Drafts", around: "Notes"))
        XCTAssertEqual(sheet.trustTitle, Strings.TrustSheet.trustNewAddress)
        XCTAssertTrue(sheet.ready)

        let second = await sheet.trust()

        XCTAssertTrue(second)
        XCTAssertEqual(host.recorded, [trust(acknowledged: []), trust(acknowledged: ["inside-trusted"])])
    }

    func testAFolderInsideNothingSendsNoAcknowledgement() async {
        let host = TrustHost()
        host.checkAnswer = .success(HostCheckAnswer(name: "Drafts", path: Self.path, warnings: [
            HostWarning(code: "contains-trusted", text: "", names: ["Week"]),
        ]))
        let sheet = sheet(host)
        await sheet.check()

        XCTAssertNil(sheet.insideTrusted)
        XCTAssertNil(sheet.newAddressWarning)
        XCTAssertEqual(sheet.trustTitle, Strings.Request.trustNamed("Drafts"))
        let trusted = await sheet.trust()

        XCTAssertTrue(trusted)
        XCTAssertEqual(host.recorded, [trust(acknowledged: [])])
    }

    func testAFailedCheckDropsTheWarningAndTheAcceptance() async {
        let host = TrustHost()
        host.checkAnswer = .success(Self.inside)
        let sheet = sheet(host)
        await sheet.check()
        XCTAssertEqual(sheet.acknowledged, ["inside-trusted"])

        host.checkAnswer = .failure(HostRefusal(code: "interrupted", text: "Try again.", retryable: true))
        await sheet.check()

        XCTAssertNil(sheet.insideTrusted)
        XCTAssertEqual(sheet.acknowledged, [])
        XCTAssertEqual(sheet.trustTitle, Strings.Request.trustNamed("Drafts"))
    }

    func testARefusedAcknowledgedTrustShowsTheRefusalAndKeepsTheWarning() async {
        let host = TrustHost()
        host.checkAnswer = .success(Self.inside)
        let unsaved = HostRefusal(code: "unsaved-edits", text: "Some edits haven't been saved yet.", documents: ["notes/index.html"])
        host.outcomes = [.refused(unsaved)]
        let sheet = sheet(host)
        await sheet.check()

        let trusted = await sheet.trust()

        XCTAssertFalse(trusted)
        XCTAssertEqual(sheet.problem, unsaved.text)
        XCTAssertEqual(sheet.insideTrusted, "Notes")
        XCTAssertEqual(host.recorded, [trust(acknowledged: ["inside-trusted"])])
    }

    func testAHostThatStillWantsConfirmationAfterTheAcceptanceShowsItsRefusal() async {
        let host = TrustHost()
        host.checkAnswer = .success(Self.inside)
        host.outcomes = [.refused(Self.needsConfirmation)]
        let sheet = sheet(host)
        await sheet.check()

        let trusted = await sheet.trust()

        XCTAssertFalse(trusted)
        XCTAssertEqual(sheet.problem, Self.needsConfirmation.text)
        XCTAssertEqual(host.recorded, [trust(acknowledged: ["inside-trusted"])])
    }

    func testAConfirmationTheCheckCannotExplainShowsTheRefusal() async {
        let host = TrustHost()
        host.checkAnswer = .success(HostCheckAnswer(name: "Drafts", path: Self.path))
        host.outcomes = [.refused(Self.needsConfirmation)]
        let sheet = sheet(host)

        let trusted = await sheet.trust()

        XCTAssertFalse(trusted)
        XCTAssertEqual(sheet.problem, Self.needsConfirmation.text)
        XCTAssertNil(sheet.insideTrusted)
        XCTAssertEqual(host.recorded, [trust(acknowledged: [])])
    }

    func testTheSampleHostWarnsInsideATrustedFolderAndChoosingAnotherDropsIt() async {
        let host = SampleHost(persona: .trustedFolder)
        let model = AppModel(host: host, initial: host.current)
        model.folderChooser = SampleFolderChooser()
        let sheet = TrustSheetModel(name: "Drafts", path: "~/Documents/Notes/Drafts", model: model)

        await sheet.check()

        XCTAssertEqual(sheet.insideTrusted, "Notes")
        XCTAssertEqual(sheet.acknowledged, ["inside-trusted"])

        let chose = await sheet.chooseAnother()

        XCTAssertTrue(chose)
        XCTAssertEqual(sheet.path, "~/Documents/Recipes")
        XCTAssertNil(sheet.insideTrusted)
        XCTAssertEqual(sheet.acknowledged, [])
        XCTAssertEqual(sheet.trustTitle, Strings.Request.trustNamed("Recipes"))
    }

    func testTheSampleHostDoesNotWarnForAListedFolderOrInsideAServedOne() async {
        let trusted = SampleHost(persona: .trustedFolder)
        let listed = TrustSheetModel(name: "Journal", path: "~/Documents/Notes/Journal",
                                     model: AppModel(host: trusted, initial: trusted.current))
        await listed.check()
        XCTAssertNil(listed.insideTrusted)

        let served = SampleHost(persona: .servedFolder)
        let folder = try? XCTUnwrap(served.current.folders.first { $0.served })
        let inside = TrustSheetModel(name: "drafts", path: (folder?.path ?? "") + "/drafts",
                                     model: AppModel(host: served, initial: served.current))
        await inside.check()
        XCTAssertNotNil(folder)
        XCTAssertNil(inside.insideTrusted)
    }
}

@MainActor
final class TrustFolderWireTests: XCTestCase {
    override func setUp() async throws {
        StubHost.reset()
    }

    override func tearDown() async throws {
        StubHost.reset()
    }

    private func client() -> LoopbackHostClient {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [StubHost.self]
        return LoopbackHostClient(port: 1212, secret: "s3cret", configuration: configuration,
                                  makeID: { "action-1" }, lookupDelays: [0, 0, 0], reconnectDelays: [0])
    }

    private func arguments(_ action: HostAction) throws -> [String: Any] {
        let wire = try XCTUnwrap(HostActionWire.wire(action))
        XCTAssertEqual(wire.type, "folder.trust")
        let body = try XCTUnwrap(JSONSerialization.jsonObject(with: try HostActionWire.body(id: "a1", wire)) as? [String: Any])
        return try XCTUnwrap(body["args"] as? [String: Any])
    }

    func testTheWireCarriesTheAcknowledgement() throws {
        let accepted = try arguments(.trustFolder(name: "Drafts", path: "/Users/sam/Notes/Drafts", acknowledged: ["inside-trusted"]))
        XCTAssertEqual(accepted["path"] as? String, "/Users/sam/Notes/Drafts")
        XCTAssertEqual(accepted["acknowledged"] as? [String], ["inside-trusted"])

        let plain = try arguments(.trustFolder(name: "Drafts", path: "/Users/sam/Notes/Drafts"))
        XCTAssertEqual(plain["acknowledged"] as? [String], [])
    }

    func testTheCheckDecodesTheInsideTrustedWarning() async {
        let reply = #"{"ok":true,"preview":{"name":"drafts","path":"/Users/sam/notes/drafts","warnings":[{"code":"inside-trusted","text":"This folder is inside notes.","names":["notes"]}]}}"#
        StubHost.script("POST", "/shell/v1/check", [.answer(200, Data(reply.utf8))])

        let answer = await client().check(.trustFolder(name: "drafts", path: "/Users/sam/notes/drafts"))

        XCTAssertEqual(answer, .success(HostCheckAnswer(name: "drafts", path: "/Users/sam/notes/drafts", warnings: [
            HostWarning(code: "inside-trusted", text: "This folder is inside notes.", names: ["notes"]),
        ])))
    }

    func testTheNeedsConfirmationRefusalReachesTheSheet() async throws {
        let refused = #"{"ok":false,"rev":1,"error":{"code":"needs-confirmation","retryable":false,"text":"This action needs your approval."}}"#
        StubHost.script("POST", "/shell/v1/actions", [.answer(409, Data(refused.utf8))])

        let outcome = await client().perform(.trustFolder(name: "drafts", path: "/Users/sam/notes/drafts"))

        guard case let .refused(refusal) = outcome else { return XCTFail("the trust was not refused: \(outcome)") }
        XCTAssertEqual(refusal.code, "needs-confirmation")
        let sent = try XCTUnwrap(StubHost.requests(for: "POST /shell/v1/actions").first)
        let body = try XCTUnwrap(JSONSerialization.jsonObject(with: sent.body) as? [String: Any])
        XCTAssertEqual((body["args"] as? [String: Any])?["acknowledged"] as? [String], [])
    }
}
