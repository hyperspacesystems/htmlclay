import XCTest
@testable import PreviewCore

@MainActor
final class AppModelPendingTests: XCTestCase {
    private func waitUntil(_ condition: @MainActor () -> Bool, _ message: String, file: StaticString = #filePath, line: UInt = #line) async {
        let deadline = Date().addingTimeInterval(2)
        while Date() < deadline {
            if condition() { return }
            await Task.yield()
        }
        XCTAssertTrue(condition(), message, file: file, line: line)
    }

    private func settle() async {
        for _ in 0..<20 { await Task.yield() }
    }

    func testDuplicateClickSendsOnce() async {
        let host = GateHost()
        let model = AppModel(host: host, initial: HostSnapshot())
        let action = HostAction.trustFolder(name: "n", path: "/p")
        model.send(action)
        await waitUntil({ host.recorded.count == 1 }, "host never saw the first click")
        model.send(action)
        await settle()
        XCTAssertEqual(host.recorded.count, 1)
        await host.resume(at: 0, with: .done(HostSnapshot()))
        await waitUntil({ !model.isPending(action) }, "first click never finished")
        model.send(action)
        await waitUntil({ host.recorded.count == 2 }, "second click never reached the host")
        XCTAssertEqual(host.recorded.count, 2)
    }

    func testNewestTypedValueWins() async {
        let host = GateHost()
        let model = AppModel(host: host, initial: HostSnapshot())
        model.send(.setYourName("A"))
        await waitUntil({ host.recorded.count == 1 }, "host never saw A")
        model.send(.setYourName("AB"))
        model.send(.setYourName("ABC"))
        await settle()
        XCTAssertEqual(host.recorded, [.setYourName("A")])
        XCTAssertEqual(model.pending("prefs.yourName"), .setYourName("ABC"))
        await host.resume(at: 0, with: .done(HostSnapshot()))
        await waitUntil({ host.recorded.count == 2 }, "queued value never sent")
        XCTAssertEqual(host.recorded.count, 2)
        XCTAssertEqual(host.recorded[1], .setYourName("ABC"))
        await host.resume(at: 1, with: .done(HostSnapshot()))
        await waitUntil({ model.pending("prefs.yourName") == nil }, "pending value never cleared")
        XCTAssertNil(model.pending("prefs.yourName"))
    }

    func testSameValueNotResent() async {
        let host = GateHost()
        let model = AppModel(host: host, initial: HostSnapshot())
        model.send(.setAIEditing(false))
        await waitUntil({ host.recorded.count == 1 }, "host never saw the first switch")
        model.send(.setAIEditing(false))
        await host.resume(at: 0, with: .done(HostSnapshot()))
        await settle()
        XCTAssertEqual(host.recorded.count, 1)
        XCTAssertNil(model.pending("prefs.aiEditing"))
    }

    func testSwitchShowsPendingValue() async {
        let host = GateHost()
        let model = AppModel(host: host, initial: HostSnapshot())
        model.send(.setServed(folderID: "f", on: true))
        await waitUntil({ host.recorded.count == 1 }, "host never saw the switch")
        XCTAssertEqual(model.pending("folder.served.f"), .setServed(folderID: "f", on: true))
        XCTAssertTrue(model.isPending(.setServed(folderID: "f", on: false)))
        await host.resume(at: 0, with: .done(HostSnapshot()))
        await waitUntil({ model.pending("folder.served.f") == nil }, "switch never finished")
    }

    func testRefusalKeepsTheSheet() async {
        let host = GateHost()
        let model = AppModel(host: host, initial: HostSnapshot())
        model.sheet = .addProgram
        let submission = Task { await model.submit(.serveFolder(path: "/p", name: "x")) }
        await waitUntil({ host.recorded.count == 1 }, "host never saw the sheet action")
        let refusal = HostRefusal(code: "name-taken", text: "Taken.")
        await host.resume(at: 0, with: .refused(refusal))
        let outcome = await submission.value
        XCTAssertEqual(outcome, .refused(refusal))
        XCTAssertEqual(model.sheet, .addProgram)
        XCTAssertEqual(model.lastRefusal?.code, "name-taken")
    }

    func testRestartInterruptsAndNeverResends() async {
        let host = GateHost()
        let initial = HostSnapshot(rev: 7)
        let model = AppModel(host: host, initial: initial)
        model.send(.serveFolder(path: "/p", name: "x"))
        model.send(.setYourName("A"))
        await waitUntil({ host.recorded.count == 2 }, "host never saw both actions")
        model.send(.setYourName("B"))
        await settle()
        XCTAssertEqual(model.pending("prefs.yourName"), .setYourName("B"))
        model.hostRestarted()
        let restarted = HostSnapshot(rev: 99)
        await host.resume(at: 0, with: .done(restarted))
        await host.resume(at: 1, with: .done(restarted))
        await settle()
        XCTAssertEqual(model.snapshot, initial)
        XCTAssertEqual(host.recorded.count, 2)
        XCTAssertNil(model.pending("prefs.yourName"))
        XCTAssertEqual(model.note, "HTML Clay restarted before that finished. Try again.")
    }

    func testSampleHostUnchanged() async {
        let host = SampleHost(persona: .newPerson)
        let model = AppModel(host: host, initial: host.current)
        model.send(.setAIEditing(false))
        await waitUntil({ !model.snapshot.preferences.aiEditing }, "sample host never answered")
        XCTAssertFalse(model.snapshot.preferences.aiEditing)
    }
}

@MainActor
private final class GateHost: HostClient {
    private(set) var recorded: [HostAction] = []
    private var continuations: [Int: CheckedContinuation<HostOutcome, Never>] = [:]

    func load() async throws -> HostSnapshot { HostSnapshot() }

    func perform(_ action: HostAction) async -> HostOutcome {
        let index = recorded.count
        recorded.append(action)
        return await withCheckedContinuation { continuation in
            continuations[index] = continuation
        }
    }

    func resume(at index: Int, with outcome: HostOutcome) async {
        let deadline = Date().addingTimeInterval(2)
        while continuations[index] == nil && Date() < deadline { await Task.yield() }
        guard let continuation = continuations[index] else {
            XCTFail("nothing was waiting at \(index)")
            return
        }
        continuations[index] = nil
        continuation.resume(returning: outcome)
    }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .success(HostCheckAnswer()) }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        HostSubscription {}
    }
}
