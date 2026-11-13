import XCTest
@testable import PreviewCore

@MainActor
final class HostClientContractTests: XCTestCase {
    private func waitUntil(_ condition: @MainActor () -> Bool, _ message: String) async {
        for _ in 0..<100 {
            if condition() { return }
            await Task.yield()
        }
        XCTAssertTrue(condition(), message)
    }

    func testCancelledSubscriptionGetsNothing() async {
        let host = SampleHost(persona: .trustedFolder)
        var first = 0
        var second = 0
        let firstSubscription = host.subscribe { _ in first += 1 }
        _ = host.subscribe { _ in second += 1 }
        firstSubscription.cancel()
        _ = await host.perform(.setAIEditing(false))
        firstSubscription.cancel()
        XCTAssertEqual(first, 0)
        XCTAssertEqual(second, 1)
    }

    func testPerformAnswersDone() async {
        let host = SampleHost(persona: .trustedFolder)
        let outcome = await host.perform(.setAIEditing(false))
        guard case let .done(next) = outcome else {
            XCTFail("expected done")
            return
        }
        XCTAssertFalse(next.preferences.aiEditing)
    }

    func testMarkSeenReachesTheHost() async {
        let host = SampleHost(persona: .trustedFolder)
        let model = AppModel(host: host, initial: host.current)
        XCTAssertFalse(host.current.waiting.isEmpty)
        model.markWaitingSeen()
        await waitUntil({ !host.current.waiting.isEmpty && host.current.waiting.allSatisfy { $0.seen } }, "host never saw markSeen")
    }

    func testRefusalIsShown() async {
        let host = RefusingHost()
        let model = AppModel(host: host, initial: HostSnapshot())
        model.send(.setAIEditing(true))
        await waitUntil({ model.lastRefusal?.code == "name-taken" && model.note == "That name is taken." }, "refusal never shown")
    }

    func testCheckAnswersServeAddress() async {
        let host = SampleHost(persona: .trustedFolder)
        let answer = await host.check(.serveFolder(path: "/x", name: "blog"))
        guard case let .success(check) = answer else {
            XCTFail("expected success")
            return
        }
        XCTAssertTrue(check.address.contains("blog.htmlclay.localhost"))
    }
}

@MainActor
private final class RefusingHost: HostClient {
    func load() async throws -> HostSnapshot { HostSnapshot() }
    func perform(_ action: HostAction) async -> HostOutcome {
        .refused(HostRefusal(code: "name-taken", text: "That name is taken."))
    }
    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .success(HostCheckAnswer()) }
    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        HostSubscription {}
    }
}
