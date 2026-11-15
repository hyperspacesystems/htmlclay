import XCTest
@testable import PreviewCore

final class StubHost: URLProtocol {
    enum Step {
        case http(status: Int, body: Data, headers: [String: String])
        case transportFailure
        case stream(chunks: [Data], headers: [String: String])

        static func answer(_ status: Int, _ body: Data, headers: [String: String] = [:]) -> Step {
            .http(status: status, body: body, headers: headers)
        }

        static func stream(_ lines: [Data]) -> Step {
            .stream(chunks: lines, headers: [:])
        }
    }

    struct Seen {
        var method: String
        var url: URL
        var headers: [String: String]
        var body: Data

        var path: String { url.path }
        var line: String { method + " " + url.path }
    }

    private static let lock = NSLock()
    private static var scripts: [String: [Step]] = [:]
    private static var requests: [Seen] = []

    static func key(_ method: String, _ path: String) -> String { method + " " + path }

    static func script(_ method: String, _ path: String, _ steps: [Step]) {
        lock.lock()
        defer { lock.unlock() }
        scripts[key(method, path)] = steps
    }

    static func reset() {
        lock.lock()
        defer { lock.unlock() }
        scripts = [:]
        requests = []
    }

    static var seen: [Seen] {
        lock.lock()
        defer { lock.unlock() }
        return requests
    }

    static func requests(for line: String) -> [Seen] {
        seen.filter { $0.line == line }
    }

    private static func take(_ line: String) -> Step {
        lock.lock()
        defer { lock.unlock() }
        guard var queue = scripts[line], !queue.isEmpty else { return .transportFailure }
        let step = queue.removeFirst()
        scripts[line] = queue
        return step
    }

    private static func record(_ request: URLRequest) {
        let seen = Seen(
            method: request.httpMethod ?? "GET",
            url: request.url ?? URL(string: "about:blank")!,
            headers: request.allHTTPHeaderFields ?? [:],
            body: bodyData(request)
        )
        lock.lock()
        requests.append(seen)
        lock.unlock()
    }

    private static func bodyData(_ request: URLRequest) -> Data {
        if let body = request.httpBody { return body }
        guard let stream = request.httpBodyStream else { return Data() }
        stream.open()
        defer { stream.close() }
        var data = Data()
        var buffer = [UInt8](repeating: 0, count: 4096)
        while stream.hasBytesAvailable {
            let read = stream.read(&buffer, maxLength: buffer.count)
            if read <= 0 { break }
            data.append(buffer, count: read)
        }
        return data
    }

    override class func canInit(with request: URLRequest) -> Bool { true }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        guard let request = request as URLRequest?, let url = request.url else {
            client?.urlProtocol(self, didFailWithError: URLError(.badURL))
            return
        }
        StubHost.record(request)
        let line = StubHost.key(request.httpMethod ?? "GET", url.path)
        switch StubHost.take(line) {
        case let .http(status, body, headers):
            guard let response = HTTPURLResponse(url: url, statusCode: status, httpVersion: "HTTP/1.1", headerFields: headers) else {
                client?.urlProtocol(self, didFailWithError: URLError(.badServerResponse))
                return
            }
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            if !body.isEmpty { client?.urlProtocol(self, didLoad: body) }
            client?.urlProtocolDidFinishLoading(self)
        case let .stream(chunks, headers):
            guard let response = HTTPURLResponse(url: url, statusCode: 200, httpVersion: "HTTP/1.1", headerFields: headers) else {
                client?.urlProtocol(self, didFailWithError: URLError(.badServerResponse))
                return
            }
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            for chunk in chunks { client?.urlProtocol(self, didLoad: chunk) }
            client?.urlProtocolDidFinishLoading(self)
        case .transportFailure:
            client?.urlProtocol(self, didFailWithError: URLError(.cannotConnectToHost))
        }
    }

    override func stopLoading() {}
}

@MainActor
final class LoopbackHostClientTests: XCTestCase {
    private static let fixtures = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .appendingPathComponent("Fixtures/v1")

    override func setUp() async throws {
        StubHost.reset()
    }

    override func tearDown() async throws {
        StubHost.reset()
    }

    private func fixture(_ name: String) throws -> Data {
        try Data(contentsOf: Self.fixtures.appendingPathComponent(name))
    }

    private func snapshotObject(rev: Int, epoch: String? = nil) throws -> [String: Any] {
        let data = try fixture("servedFolder.snapshot.json")
        var raw = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        raw["rev"] = rev
        if let epoch { raw["epoch"] = epoch }
        return raw
    }

    private func snapshotData(rev: Int, epoch: String? = nil) throws -> Data {
        try JSONSerialization.data(withJSONObject: try snapshotObject(rev: rev, epoch: epoch))
    }

    private func done(rev: Int, epoch: String? = nil) throws -> Data {
        try JSONSerialization.data(withJSONObject: [
            "ok": true,
            "rev": rev,
            "snapshot": try snapshotObject(rev: rev, epoch: epoch),
        ])
    }

    private func refusal(_ code: String, retryable: Bool = false, text: String = "no", documents: [String] = []) throws -> Data {
        var error: [String: Any] = ["code": code, "retryable": retryable, "text": text]
        if !documents.isEmpty { error["documents"] = documents }
        return try JSONSerialization.data(withJSONObject: ["ok": false, "rev": 1, "error": error])
    }

    private func eventLine(rev: Int, epoch: String? = nil) throws -> Data {
        let snapshot = try snapshotObject(rev: rev, epoch: epoch)
        var data = try JSONSerialization.data(withJSONObject: ["type": "snapshot", "rev": rev, "snapshot": snapshot])
        data.append(0x0A)
        return data
    }

    private func byeLine() -> Data {
        Data("{\"type\":\"bye\",\"reason\":\"quit\"}\n".utf8)
    }

    private func makeClient(
        lookupDelays: [Double] = [0, 0, 0],
        makeID: @escaping () -> String = { "action-1" }
    ) -> LoopbackHostClient {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [StubHost.self]
        return LoopbackHostClient(
            port: 1212,
            secret: "s3cret",
            configuration: configuration,
            makeID: makeID,
            lookupDelays: lookupDelays,
            reconnectDelays: [0]
        )
    }

    private func since(of url: URL) -> String? {
        URLComponents(url: url, resolvingAgainstBaseURL: false)?
            .queryItems?.first { $0.name == "since" }?.value
    }

    private func waitUntil(_ message: String, _ condition: @MainActor () -> Bool) async {
        for _ in 0..<200 {
            if condition() { return }
            await Task.yield()
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertTrue(condition(), message)
    }

    private func waitForStream(_ client: LoopbackHostClient) async {
        _ = client.subscribe { _ in }
        await waitUntil("the stream never opened") { !StubHost.requests(for: "GET /shell/v1/events").isEmpty }
    }

    func testRequestsCarryOnlyWhatTheHostAccepts() async throws {
        StubHost.script("GET", "/shell/v1/snapshot", [.answer(200, try snapshotData(rev: 1))])
        StubHost.script("POST", "/shell/v1/actions", [.answer(200, try done(rev: 2))])
        StubHost.script("POST", "/shell/v1/check", [.answer(200, Data("{\"ok\":true,\"preview\":{\"name\":\"blog\"}}".utf8))])
        StubHost.script("GET", "/shell/v1/events", [.stream([byeLine()])])
        let client = makeClient()
        _ = try await client.load()
        _ = await client.perform(.setAIEditing(false))
        _ = await client.check(.setAIEditing(true))
        await waitForStream(client)
        let seen = StubHost.seen
        XCTAssertEqual(seen.count, 4)
        for request in seen {
            XCTAssertEqual(request.url.host, "127.0.0.1")
            XCTAssertEqual(request.url.port, 1212)
            XCTAssertEqual(request.headers["Authorization"], "Bearer s3cret")
            for header in ["Origin", "Cookie", "Referer", "Sec-Fetch-Site", "Sec-Fetch-Mode", "Sec-Fetch-Dest"] {
                XCTAssertNil(request.headers[header], header)
            }
        }
    }

    func testCookiesAreNeverStoredOrSent() async throws {
        StubHost.script("GET", "/shell/v1/snapshot", [
            .answer(200, try snapshotData(rev: 1), headers: ["Set-Cookie": "a=b; Path=/"]),
            .answer(200, try snapshotData(rev: 2)),
        ])
        let client = makeClient()
        XCTAssertNil(client.session.configuration.httpCookieStorage)
        XCTAssertFalse(client.session.configuration.httpShouldSetCookies)
        XCTAssertEqual(client.session.configuration.httpCookieAcceptPolicy, .never)
        _ = try await client.load()
        _ = try await client.load()
        let seen = StubHost.seen
        XCTAssertEqual(seen.count, 2)
        XCTAssertNotNil(seen[0].headers["Authorization"])
        XCTAssertNil(seen[1].headers["Cookie"])
    }

    func testRedirectIsNotFollowed() async throws {
        StubHost.script("POST", "/shell/v1/actions", [
            .answer(302, Data(), headers: ["Location": "http://example.com/"]),
        ])
        let client = makeClient()
        let outcome = await client.perform(.setAIEditing(false))
        guard case let .refused(refusal) = outcome else {
            XCTFail("expected refused")
            return
        }
        XCTAssertEqual(refusal.code, "host-error")
        XCTAssertTrue(StubHost.seen.allSatisfy { $0.url.host == "127.0.0.1" })
        XCTAssertEqual(StubHost.seen.count, 1)
    }

    func testRefusalCarriesItsFields() async throws {
        StubHost.script("POST", "/shell/v1/actions", [
            .answer(409, try refusal("quit-unsaved", text: "There are unsaved notes.", documents: ["notes/a.htmlclay"])),
        ])
        let client = makeClient()
        let outcome = await client.perform(.setAIEditing(false))
        guard case let .refused(refusal) = outcome else {
            XCTFail("expected refused")
            return
        }
        XCTAssertEqual(refusal.code, "quit-unsaved")
        XCTAssertFalse(refusal.retryable)
        XCTAssertEqual(refusal.documents, ["notes/a.htmlclay"])
    }

    func testLostReplyIsLookedUpNotResent() async throws {
        StubHost.script("POST", "/shell/v1/actions", [.transportFailure])
        StubHost.script("GET", "/shell/v1/actions/action-1", [.answer(200, try done(rev: 4))])
        let client = makeClient()
        let outcome = await client.perform(.setAIEditing(false))
        guard case let .done(snapshot) = outcome else {
            XCTFail("expected done")
            return
        }
        XCTAssertEqual(snapshot.rev, 4)
        XCTAssertEqual(StubHost.requests(for: "POST /shell/v1/actions").count, 1)
        XCTAssertEqual(StubHost.requests(for: "GET /shell/v1/actions/action-1").count, 1)
    }

    func testStillRunningThenDone() async throws {
        StubHost.script("POST", "/shell/v1/actions", [.transportFailure])
        StubHost.script("GET", "/shell/v1/actions/action-1", [
            .answer(409, try refusal("still-running", retryable: true)),
            .answer(200, try done(rev: 5)),
        ])
        let client = makeClient()
        let outcome = await client.perform(.setAIEditing(false))
        guard case let .done(snapshot) = outcome else {
            XCTFail("expected done")
            return
        }
        XCTAssertEqual(snapshot.rev, 5)
        XCTAssertEqual(StubHost.requests(for: "GET /shell/v1/actions/action-1").count, 2)
        XCTAssertEqual(StubHost.requests(for: "POST /shell/v1/actions").count, 1)
    }

    func testNeverArrivedIsInterrupted() async throws {
        StubHost.script("POST", "/shell/v1/actions", [.transportFailure])
        StubHost.script("GET", "/shell/v1/actions/action-1", [.answer(404, Data())])
        StubHost.script("GET", "/shell/v1/snapshot", [.answer(200, try snapshotData(rev: 6))])
        let client = makeClient()
        let outcome = await client.perform(.setAIEditing(false))
        guard case let .refused(refusal) = outcome else {
            XCTFail("expected refused")
            return
        }
        XCTAssertEqual(refusal.code, "interrupted")
        XCTAssertTrue(refusal.retryable)
        let seen = StubHost.seen
        let lookup = try XCTUnwrap(seen.firstIndex { $0.line == "GET /shell/v1/actions/action-1" })
        XCTAssertTrue(seen.suffix(from: lookup).contains { $0.line == "GET /shell/v1/snapshot" })
        XCTAssertEqual(StubHost.requests(for: "POST /shell/v1/actions").count, 1)
    }

    func testUnreachable() async throws {
        let client = makeClient()
        let outcome = await client.perform(.setAIEditing(false))
        guard case let .refused(refusal) = outcome else {
            XCTFail("expected refused")
            return
        }
        XCTAssertEqual(refusal.code, "unreachable")
        XCTAssertTrue(refusal.retryable)
    }

    func testNotYetForSampleOnly() async throws {
        let client = makeClient()
        let outcome = await client.perform(.connectSample)
        guard case let .refused(refusal) = outcome else {
            XCTFail("expected refused")
            return
        }
        XCTAssertEqual(refusal.code, "not-yet")
        XCTAssertTrue(StubHost.seen.isEmpty)
    }

    func testStreamDeliversInOrderAcrossChunks() async throws {
        let transcript = try fixture("events.v1.ndjson")
        var chunks: [Data] = []
        var index = transcript.startIndex
        while index < transcript.endIndex {
            let end = transcript.index(index, offsetBy: 5, limitedBy: transcript.endIndex) ?? transcript.endIndex
            chunks.append(transcript[index..<end])
            index = end
        }
        StubHost.script("GET", "/shell/v1/events", [.stream(chunks)])
        let client = makeClient()
        var revs: [Int] = []
        var endings: [String] = []
        client.onEnded = { endings.append($0) }
        _ = client.subscribe { revs.append($0.rev) }
        await waitUntil("the stream never finished") { !endings.isEmpty }
        XCTAssertEqual(revs, [0, 1, 3, 5, 7])
        XCTAssertEqual(endings, ["quit"])
        XCTAssertEqual(StubHost.requests(for: "GET /shell/v1/events").count, 1)
    }

    func testStreamReconnectsWithSince() async throws {
        StubHost.script("GET", "/shell/v1/events", [
            .stream([try eventLine(rev: 0), try eventLine(rev: 1)]),
            .stream([byeLine()]),
        ])
        let client = makeClient()
        _ = client.subscribe { _ in }
        await waitUntil("the stream never reconnected") {
            StubHost.requests(for: "GET /shell/v1/events").count == 2
        }
        let events = StubHost.requests(for: "GET /shell/v1/events")
        XCTAssertEqual(since(of: events[0].url), "0")
        XCTAssertEqual(since(of: events[1].url), "1")
    }

    func testCancelStopsTheStream() async throws {
        StubHost.script("GET", "/shell/v1/events", [.stream([try eventLine(rev: 1)])])
        let client = makeClient()
        var shown = 0
        var subscription: HostSubscription?
        subscription = client.subscribe { _ in
            shown += 1
            subscription?.cancel()
        }
        await waitUntil("the stream never delivered") { shown == 1 }
        try await Task.sleep(nanoseconds: 300_000_000)
        XCTAssertEqual(shown, 1)
        XCTAssertEqual(StubHost.requests(for: "GET /shell/v1/events").count, 1)
    }

    func testNewEpochIsReported() async throws {
        StubHost.script("GET", "/shell/v1/events", [
            .stream([try eventLine(rev: 1, epoch: "epoch-one"), try eventLine(rev: 2, epoch: "epoch-two")]),
        ])
        let client = makeClient()
        var epochs: [(String, String)] = []
        client.onNewEpoch = { epochs.append(($0, $1)) }
        _ = client.subscribe { _ in }
        await waitUntil("the new epoch was never reported") { !epochs.isEmpty }
        XCTAssertEqual(epochs.count, 1)
        XCTAssertEqual(epochs.first?.0, "epoch-one")
        XCTAssertEqual(epochs.first?.1, "epoch-two")
    }

    func testSecretNeverPrinted() async throws {
        let client = makeClient()
        XCTAssertFalse(String(describing: client).contains("s3cret"))
        XCTAssertFalse(String(reflecting: client).contains("s3cret"))
        XCTAssertFalse("\(client)".contains("s3cret"))
    }
}

// 08-sync.md 8.44 and 8.45: the history route and the notice frame, against the stubbed host.
extension LoopbackHostClientTests {
    func testNoticesANoticeFrameReachesOnNotice() async throws {
        let notice = Data(#"{"type":"notice","id":"c-1","kind":"conflict","folderID":"0dd3adfe2b553e75","name":"board.html","detail":"both-edited"}"#.utf8) + Data([0x0A])
        let unknown = Data(#"{"type":"notice","id":"x-1","kind":"someday","folderID":null,"name":"","detail":""}"#.utf8) + Data([0x0A])
        StubHost.script("GET", "/shell/v1/events", [.stream([try eventLine(rev: 0), notice, unknown, byeLine()])])
        let client = makeClient()
        var notices: [HostNotice] = []
        var endings: [String] = []
        client.onNotice = { notices.append($0) }
        client.onEnded = { endings.append($0) }
        _ = client.subscribe { _ in }
        await waitUntil("the stream never finished") { !endings.isEmpty }
        XCTAssertEqual(notices, [HostNotice(id: "c-1", kind: .conflict, folderID: "0dd3adfe2b553e75", name: "board.html", detail: "both-edited")])
    }

    func testFolderHistoryAsksWithTheCursorAndANotFoundIsNil() async throws {
        let body = Data(#"{"entries":[{"id":"a","at":"2026-11-15T13:58:01Z","kind":"downloaded","path":"blog/launch.html","detail":"download"}],"next":null}"#.utf8)
        StubHost.script("GET", "/shell/v1/folders/0dd3adfe2b553e75/history", [.answer(200, body), .answer(404, Data())])
        let client = makeClient()
        let page = await client.folderHistory("0dd3adfe2b553e75", before: "b 1")
        XCTAssertEqual(page?.entries.map(\.id), ["a"])
        XCTAssertEqual(page?.entries.first?.kind, .downloaded)
        XCTAssertNil(page?.next)
        let seen = StubHost.requests(for: "GET /shell/v1/folders/0dd3adfe2b553e75/history")
        XCTAssertEqual(seen.first?.url.query, "before=b%201")
        XCTAssertEqual(seen.first?.headers["Authorization"], "Bearer s3cret")
        let missing = await client.folderHistory("0dd3adfe2b553e75", before: nil)
        XCTAssertNil(missing)
    }
}
