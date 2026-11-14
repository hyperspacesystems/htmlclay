import XCTest
@testable import PreviewCore

@MainActor
private final class RelayFakeHost: HostClient {
    var current: HostSnapshot
    var loaded: HostSnapshot?
    var onNewEpoch: (@MainActor (String, String) -> Void)?
    var performed: [HostAction] = []
    private var listeners: [Int: @MainActor (HostSnapshot) -> Void] = [:]
    private var next = 0

    init(_ snapshot: HostSnapshot) { current = snapshot }

    func load() async throws -> HostSnapshot { loaded ?? current }

    func perform(_ action: HostAction) async -> HostOutcome {
        performed.append(action)
        return .done(current)
    }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> {
        .success(HostCheckAnswer())
    }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        let key = next
        next += 1
        listeners[key] = onChange
        onChange(current)
        return HostSubscription { [weak self] in self?.listeners[key] = nil }
    }

    func push(_ snapshot: HostSnapshot) {
        current = snapshot
        listeners.values.forEach { $0(snapshot) }
    }
}

@MainActor
final class RelayHostClientTests: XCTestCase {
    private func attach(_ relay: RelayHostClient, _ host: RelayFakeHost) async {
        await relay.attach(host) { [weak host] hook in
            host?.onNewEpoch = { _, _ in hook.call() }
        }
    }

    func testAttachForwardsSnapshots() async {
        let relay = RelayHostClient()
        let first = RelayFakeHost(HostSnapshot(rev: 1, epoch: "e1"))
        await attach(relay, first)
        XCTAssertEqual(relay.last.rev, 1)
        var seen: [Int] = []
        _ = relay.subscribe { seen.append($0.rev) }
        first.push(HostSnapshot(rev: 2, epoch: "e1"))
        XCTAssertEqual(seen, [2])
        XCTAssertEqual(relay.last.rev, 2)
    }

    func testSecondAttachCallsOnRestartOnce() async {
        let relay = RelayHostClient()
        var restarts = 0
        relay.onRestart = { restarts += 1 }
        await attach(relay, RelayFakeHost(HostSnapshot(rev: 1, epoch: "e1")))
        XCTAssertEqual(restarts, 0)
        await attach(relay, RelayFakeHost(HostSnapshot(rev: 1, epoch: "e2")))
        XCTAssertEqual(restarts, 1)
    }

    func testNewEpochCallsOnRestart() async {
        let relay = RelayHostClient()
        let host = RelayFakeHost(HostSnapshot(rev: 1, epoch: "e1"))
        var restarts = 0
        relay.onRestart = { restarts += 1 }
        await attach(relay, host)
        host.onNewEpoch?("e1", "e2")
        XCTAssertEqual(restarts, 1)
    }

    func testHostGoneShowsStoppedAndRefusesActions() async {
        let relay = RelayHostClient()
        let host = RelayFakeHost(HostSnapshot(folders: [Folder(id: "a", name: "notes", path: "~/notes")], rev: 4, epoch: "e1"))
        var stopped: Bool?
        _ = relay.subscribe { stopped = $0.hostStopped }
        await attach(relay, host)
        relay.hostGone()
        XCTAssertEqual(stopped, true)
        XCTAssertTrue(relay.last.hostStopped)
        XCTAssertEqual(relay.last.folders.count, 1)
        let outcome = await relay.perform(.setAIEditing(true))
        guard case let .refused(refusal) = outcome else {
            XCTFail("expected refused")
            return
        }
        XCTAssertEqual(refusal.code, "unreachable")
        XCTAssertTrue(refusal.retryable)
        XCTAssertTrue(host.performed.isEmpty)
    }

    func testRestartHostAsksTheAppAndSendsNothing() async {
        let relay = RelayHostClient()
        let host = RelayFakeHost(HostSnapshot(rev: 3, epoch: "e1"))
        var asked = 0
        relay.onRestartRequested = { asked += 1 }
        await attach(relay, host)
        let outcome = await relay.perform(.restartHost)
        XCTAssertEqual(asked, 1)
        guard case let .done(snapshot) = outcome else {
            XCTFail("expected done")
            return
        }
        XCTAssertEqual(snapshot.rev, 3)
        XCTAssertTrue(host.performed.isEmpty)
    }

    func testOlderLoadDoesNotReplaceNewerStream() async {
        let relay = RelayHostClient()
        let host = RelayFakeHost(HostSnapshot(rev: 5, epoch: "e1"))
        host.loaded = HostSnapshot(rev: 3, epoch: "e1")
        await attach(relay, host)
        XCTAssertEqual(relay.last.rev, 5)
    }
}
