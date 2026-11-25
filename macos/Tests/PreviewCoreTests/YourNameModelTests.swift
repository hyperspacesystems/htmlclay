import XCTest
@testable import PreviewCore

/// A host that remembers what it was asked and answers with the setting changed.
@MainActor
private final class RecorderHost: HostClient {
    private(set) var recorded: [HostAction] = []
    private var current: HostSnapshot
    private let refusal: HostRefusal?

    init(_ snapshot: HostSnapshot = HostSnapshot(), refusal: HostRefusal? = nil) {
        self.current = snapshot
        self.refusal = refusal
    }

    func load() async throws -> HostSnapshot { current }

    func perform(_ action: HostAction) async -> HostOutcome {
        recorded.append(action)
        if let refusal { return .refused(refusal) }
        switch action {
        case let .setYourName(name): current.preferences.yourName = name
        case let .setUseName(on): current.preferences.useName = on
        default: break
        }
        return .done(current)
    }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .success(HostCheckAnswer()) }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        HostSubscription {}
    }
}

@MainActor
final class YourNameModelTests: XCTestCase {
    private func waitUntil(_ message: String, _ check: @MainActor () -> Bool) async {
        let deadline = Date().addingTimeInterval(2)
        while Date() < deadline {
            if check() { return }
            await Task.yield()
        }
        XCTAssertTrue(check(), message)
    }

    private func settle() async {
        for _ in 0..<20 { await Task.yield() }
    }

    private func emptyNameSnapshot() -> HostSnapshot {
        var snapshot = HostSnapshot()
        snapshot.preferences.yourName = ""
        snapshot.preferences.useName = false
        return snapshot
    }

    func testTurningOnWithNoNameSendsNothingAndShowsTheField() async {
        let snapshot = emptyNameSnapshot()
        let host = RecorderHost(snapshot)
        let model = AppModel(host: host, initial: snapshot)
        let name = YourNameModel(model: model)

        name.setUseName(true)
        await settle()

        XCTAssertEqual(host.recorded, [])
        XCTAssertTrue(name.settingUp)
        XCTAssertTrue(name.isOn)
        XCTAssertEqual(name.mode, .local)
    }

    func testTypingANameSetsItThenTurnsSharingOn() async {
        let snapshot = emptyNameSnapshot()
        let host = RecorderHost(snapshot)
        let model = AppModel(host: host, initial: snapshot)
        let name = YourNameModel(model: model)

        name.setUseName(true)
        name.setDraft("A")
        await waitUntil("sharing never turned on") { !name.settingUp }

        XCTAssertEqual(host.recorded, [.setYourName("A"), .setUseName(true)])
        XCTAssertEqual(name.draft, "A")
        XCTAssertNil(name.problem)
        XCTAssertEqual(name.mode, .local)
    }

    func testTurningOffWhileSettingUpSendsNothing() async {
        let snapshot = emptyNameSnapshot()
        let host = RecorderHost(snapshot)
        let model = AppModel(host: host, initial: snapshot)
        let name = YourNameModel(model: model)

        name.setUseName(true)
        name.setUseName(false)
        await settle()

        XCTAssertEqual(host.recorded, [])
        XCTAssertFalse(name.settingUp)
        XCTAssertFalse(name.isOn)
        XCTAssertEqual(name.mode, .off)
    }

    func testANameAlreadySetTurnsSharingOnAtOnce() async {
        var snapshot = HostSnapshot()
        snapshot.preferences.yourName = "Ada"
        snapshot.preferences.useName = false
        let host = RecorderHost(snapshot)
        let model = AppModel(host: host, initial: snapshot)
        let name = YourNameModel(model: model)

        name.setUseName(true)
        await waitUntil("sharing never reached the host") { host.recorded.count == 1 }

        XCTAssertEqual(host.recorded, [.setUseName(true)])
        XCTAssertFalse(name.settingUp)
        XCTAssertTrue(name.isOn)
    }

    func testARefusedNameKeepsSettingUpAndShowsTheText() async {
        let refusal = HostRefusal(code: "bad-arguments", text: "That name has an email address in it.")
        let snapshot = emptyNameSnapshot()
        let host = RecorderHost(snapshot, refusal: refusal)
        let model = AppModel(host: host, initial: snapshot)
        let name = YourNameModel(model: model)

        name.setUseName(true)
        name.setDraft("A")
        await waitUntil("the refusal never showed") { name.problem != nil }

        XCTAssertEqual(host.recorded, [.setYourName("A")])
        XCTAssertTrue(name.settingUp)
        XCTAssertEqual(name.problem, refusal.text)
        XCTAssertEqual(name.mode, .local)
    }

    func testALaterEditSendsTheNameAsBefore() async {
        var snapshot = HostSnapshot()
        snapshot.preferences.yourName = "Ada"
        snapshot.preferences.useName = true
        let host = RecorderHost(snapshot)
        let model = AppModel(host: host, initial: snapshot)
        let name = YourNameModel(model: model)

        name.setDraft("Adah")
        await waitUntil("the edit never reached the host") { host.recorded.count == 1 }

        XCTAssertEqual(host.recorded, [.setYourName("Adah")])
        XCTAssertFalse(name.settingUp)
    }
}
