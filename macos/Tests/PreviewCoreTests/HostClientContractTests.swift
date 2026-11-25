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

@MainActor
private final class PushHost: HostClient {
    var current: HostSnapshot
    private var listeners: [Int: @MainActor (HostSnapshot) -> Void] = [:]
    private var next = 0
    init(_ snapshot: HostSnapshot) { current = snapshot }
    func load() async throws -> HostSnapshot { current }
    func perform(_ action: HostAction) async -> HostOutcome { .done(current) }
    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .success(HostCheckAnswer()) }
    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        let key = next
        next += 1
        listeners[key] = onChange
        return HostSubscription { [weak self] in self?.listeners[key] = nil }
    }
    func push(_ snapshot: HostSnapshot) {
        current = snapshot
        listeners.values.forEach { $0(snapshot) }
    }
}

@MainActor
final class FolderRouteTests: XCTestCase {
    private func snapshot(_ folders: [Folder]) -> HostSnapshot { HostSnapshot(folders: folders) }

    func testOpenFolderFollowsANewIDAtTheSamePlace() {
        let host = PushHost(snapshot([Folder(id: "tabc", name: "notes", path: "~/notes", fullPath: "/Users/sam/notes")]))
        let model = AppModel(host: host, initial: host.current)
        model.route = .folder("tabc")
        host.push(snapshot([Folder(id: "0123456789abcdef", name: "notes", path: "~/notes", served: true, fullPath: "/Users/sam/notes")]))
        XCTAssertEqual(model.route, .folder("0123456789abcdef"))
        model.route = .folderHistory("0123456789abcdef")
        host.push(snapshot([Folder(id: "tabc", name: "notes", path: "~/notes", fullPath: "/Users/sam/notes")]))
        XCTAssertEqual(model.route, .folderHistory("tabc"))
    }

    func testRemovedFolderGoesBackToTheList() {
        let host = PushHost(snapshot([Folder(id: "tabc", name: "notes", path: "~/notes", fullPath: "/Users/sam/notes")]))
        let model = AppModel(host: host, initial: host.current)
        model.route = .folder("tabc")
        host.push(snapshot([]))
        XCTAssertEqual(model.route, .pane(.folders))
    }
}
