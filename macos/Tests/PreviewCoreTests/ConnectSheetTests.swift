import XCTest
@testable import PreviewCore

/// A host that answers sync.connect with a refusal or a connection, and records what it got.
@MainActor
private final class ConnectHost: HostClient {
    var answer: HostOutcome = .refused(HostRefusal(code: "sync-key-refused", text: "Hyperclay didn't accept that sync key."))
    private(set) var performed: [HostAction] = []

    func load() async throws -> HostSnapshot { HostSnapshot() }
    func perform(_ action: HostAction) async -> HostOutcome {
        performed.append(action)
        return answer
    }
    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .failure(HostRefusal(code: "unsupported", text: "")) }
    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription { HostSubscription {} }
}

@MainActor
final class ConnectSheetTests: XCTestCase {
    private let key = "hcsk_" + String(repeating: "ab", count: 32)

    private func sheet() -> (ConnectHost, AppModel, ConnectSheetModel) {
        let host = ConnectHost()
        let model = AppModel(host: host, initial: HostSnapshot())
        let form = ConnectSheetModel(model: model)
        form.username = "maya"
        return (host, model, form)
    }

    func testConnectSendsSyncConnectWithTheKeyAndUsername() async {
        let (host, _, form) = sheet()
        form.key = key
        _ = await form.connect()
        XCTAssertEqual(host.performed, [.connect(username: "maya", key: key)])
        XCTAssertEqual(HostActionWire.wire(HostAction.connect(username: "maya", key: key))?.type, "sync.connect")
    }

    func testARefusalShowsItsLineAndTheKeyIsCleared() async {
        let (_, _, form) = sheet()
        form.key = key
        let connected = await form.connect()
        XCTAssertFalse(connected)
        XCTAssertEqual(form.refusal, "Hyperclay didn't accept that sync key.")
        XCTAssertEqual(form.key, "", "the key is cleared")
        XCTAssertFalse(form.ready)
        form.key = key
        XCTAssertNil(form.refusal, "typing again clears the refusal")
    }

    func testAConnectionAnswersTrueAndKeepsNoKey() async {
        let (host, _, form) = sheet()
        host.answer = .done(HostSnapshot())
        form.key = key
        let connected = await form.connect()
        XCTAssertTrue(connected)
        XCTAssertEqual(form.key, "")
        XCTAssertNil(form.refusal)
    }

    func testAfterAConnectTheAccountFolderSheetUsesTheHostsUsername() {
        var snapshot = HostSnapshot()
        snapshot.sync.connected = true
        snapshot.sync.username = "maya"
        snapshot.folders = [Folder(id: "f1", name: "maya", path: "", origin: .account(username: "maya"), onThisMac: false)]
        XCTAssertEqual(ConnectSheetModel.nextSheet(after: snapshot, wasConnected: false), .accountFolder(username: "maya"))
        XCTAssertNil(ConnectSheetModel.nextSheet(after: snapshot, wasConnected: true), "replacing a key opens nothing")
        snapshot.folders[0].onThisMac = true
        XCTAssertNil(ConnectSheetModel.nextSheet(after: snapshot, wasConnected: false), "a folder already on this Mac opens nothing")
    }

    func testAWrongUsernameKeepsTheKeyAndAnyOtherRefusalClearsIt() async {
        let (host, _, form) = sheet()
        form.username = "mia"
        form.key = key
        host.answer = .refused(HostRefusal(code: "sync-wrong-account", text: "other account"))
        _ = await form.connect()
        XCTAssertEqual(form.key, key, "only the username was wrong")
        XCTAssertEqual(form.refusal, "other account", "the message says why")
        host.answer = .refused(HostRefusal(code: "sync-key-refused", text: "refused"))
        _ = await form.connect()
        XCTAssertEqual(form.key, "", "a refused key is not kept")
    }

    func testClosingClearsTheKey() {
        let (_, _, form) = sheet()
        form.key = key
        form.close()
        XCTAssertEqual(form.key, "")
    }

    func testTheKeyIsNeverPartOfTheModelsInFlightKey() {
        XCTAssertEqual(AppModel.slot(of: .connect(username: "maya", key: key)), "sync.connect")
    }

    func testTheKeyAndSignUpLinksGoToHyperclay() {
        XCTAssertEqual(URL(string: Strings.Connect.keyPageURL)?.host, "hyperclay.com")
        XCTAssertEqual(Strings.Connect.signUpURL, "https://hyperclay.com")
        XCTAssertEqual(Strings.Connect.introLead + Strings.Connect.signUpLink,
                       "Hyperclay is a platform that lets you sync your local HTML files to the web, so you can open and collaborate on them from anywhere. Sign up ↗")
    }
}
