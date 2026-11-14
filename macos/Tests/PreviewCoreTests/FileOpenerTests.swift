import XCTest
@testable import PreviewCore

@MainActor
final class FileOpenerTests: XCTestCase {
    private func waitUntil(_ condition: @MainActor () -> Bool, _ message: String, file: StaticString = #filePath, line: UInt = #line) async {
        let deadline = Date().addingTimeInterval(2)
        while Date() < deadline {
            if condition() { return }
            await Task.yield()
        }
        XCTAssertTrue(condition(), message, file: file, line: line)
    }

    private func makeOpener(_ host: FakeFileHost) -> FileOpener {
        let model = AppModel(host: host, initial: HostSnapshot())
        return FileOpener(model: model)
    }

    func testOpensQueueUntilTheHostIsReady() async {
        let host = FakeFileHost()
        let opener = makeOpener(host)
        opener.open(["/p/one.htmlclay", "/p/two.html", "/p/one.htmlclay"])
        XCTAssertEqual(opener.queued, ["/p/one.htmlclay", "/p/two.html"])
        await Task.yield()
        XCTAssertEqual(host.recorded, [])

        opener.hostReady()
        await waitUntil({ host.recorded.count == 2 }, "the queued opens never reached the host")
        XCTAssertEqual(host.recorded, [.openFile(path: "/p/one.htmlclay"), .openFile(path: "/p/two.html")])
        XCTAssertEqual(opener.queued, [])
    }

    func testHostReadyKeepsTheOrder() async {
        let host = FakeFileHost()
        let opener = makeOpener(host)
        opener.open(["/p/a.htmlclay"])
        opener.open(["/p/b.htmlclay"])
        opener.open(["/p/c.htmlclay"])
        opener.hostReady()
        await waitUntil({ host.recorded.count == 3 }, "not every queued open reached the host")
        XCTAssertEqual(host.recorded, [.openFile(path: "/p/a.htmlclay"),
                                       .openFile(path: "/p/b.htmlclay"),
                                       .openFile(path: "/p/c.htmlclay")])
    }

    func testARefusedOpenIsReported() async {
        let host = FakeFileHost()
        let refusal = HostRefusal(code: "file-outside-home", text: "That file is outside your home folder.")
        host.answer = .refused(refusal)
        let opener = makeOpener(host)
        var refusals: [(String, HostRefusal)] = []
        opener.onRefused = { refusals.append(($0, $1)) }
        opener.hostReady()

        opener.open(["/somewhere/else/notes.htmlclay"])
        await waitUntil({ !refusals.isEmpty }, "the refusal was never reported")
        XCTAssertEqual(refusals.count, 1)
        XCTAssertEqual(refusals.first?.0, "notes.htmlclay")
        XCTAssertEqual(refusals.first?.1, refusal)
    }

    func testADoneOpenIsNotReported() async {
        let host = FakeFileHost()
        let opener = makeOpener(host)
        var refusals: [HostRefusal] = []
        opener.onRefused = { _, refusal in refusals.append(refusal) }
        opener.hostReady()

        opener.open(["/p/one.htmlclay"])
        await waitUntil({ host.recorded.count == 1 }, "the open never reached the host")
        await Task.yield()
        XCTAssertEqual(refusals, [])
    }

    func testAnOpenAfterTheHostGoesQueuesAgain() async {
        let host = FakeFileHost()
        let opener = makeOpener(host)
        opener.hostReady()
        opener.hostGone()

        opener.open(["/p/after.htmlclay"])
        XCTAssertEqual(opener.queued, ["/p/after.htmlclay"])
        await Task.yield()
        XCTAssertEqual(host.recorded, [])

        opener.hostReady()
        await waitUntil({ host.recorded.count == 1 }, "the open made while the host was gone never reached it")
        XCTAssertEqual(host.recorded, [.openFile(path: "/p/after.htmlclay")])
    }
}

@MainActor
private final class FakeFileHost: HostClient {
    private(set) var recorded: [HostAction] = []
    var answer: HostOutcome = .done(HostSnapshot())

    func load() async throws -> HostSnapshot { HostSnapshot() }

    func perform(_ action: HostAction) async -> HostOutcome {
        recorded.append(action)
        return answer
    }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .success(HostCheckAnswer()) }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        HostSubscription {}
    }
}
