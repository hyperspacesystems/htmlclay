import XCTest
@testable import PreviewCore

@MainActor
private final class AnswerHost: HostClient {
    private(set) var recorded: [HostAction] = []
    private let outcome: HostOutcome

    init(_ outcome: HostOutcome) { self.outcome = outcome }

    func load() async throws -> HostSnapshot { HostSnapshot() }

    func perform(_ action: HostAction) async -> HostOutcome {
        recorded.append(action)
        return outcome
    }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .success(HostCheckAnswer()) }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription { HostSubscription {} }
}

@MainActor
final class RequestFlowTests: XCTestCase {
    private static let fixtures = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .appendingPathComponent("Fixtures/v1")

    private let budget = ProgramRequest(id: "r3f09a1c22b7e", kind: .folderRead, document: "budget.htmlclay",
                                        program: String(), folder: "Receipts", path: "/Users/x/Receipts",
                                        detail: String(), revision: 2,
                                        answers: ["deny", "allow-once", "trust-folder"])
    private var budgetItem: WaitingItem {
        WaitingItem(id: "w-budget", kind: .programRequest, title: "budget.htmlclay wants", subjectID: budget.id)
    }

    override func tearDown() async throws {
        await LiveHost.stopAll()
    }

    private func waitUntil(_ message: String, timeout: TimeInterval = 10, _ check: () -> Bool) async {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if check() { return }
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertTrue(check(), message)
    }

    func testASnapshotWithAnswersDecodes() throws {
        let data = try Data(contentsOf: Self.fixtures.appendingPathComponent("trustedFolder.snapshot.json"))
        let wire = try HostWireCoding.decoder().decode(WireSnapshot.self, from: data)
        let snapshot = HostSnapshotAdapter.snapshot(from: wire)
        let source = try XCTUnwrap(wire.requests.first)
        let request = try XCTUnwrap(snapshot.requests.first)
        XCTAssertEqual(request.id, source.id)
        XCTAssertEqual(request.kind.rawValue, source.kind)
        XCTAssertEqual(request.revision, source.revision)
        XCTAssertEqual(request.answers, source.answers)
        XCTAssertFalse(request.answers.isEmpty)
    }

    func testOneClickSendsOneAnswerWithTheRequestsIDAndRevision() throws {
        XCTAssertEqual(RequestFlow.action(RequestAnswer.allowOnce, request: budget),
                       .answer(requestID: "r3f09a1c22b7e", revision: 2, answer: "allow-once", path: String()))

        let plain = try XCTUnwrap(HostActionWire.wire(RequestFlow.action(RequestAnswer.allowOnce, request: budget)))
        XCTAssertEqual(plain.type, "request.answer")
        XCTAssertEqual(plain.args, ["id": .string("r3f09a1c22b7e"),
                                    "revision": .int(2),
                                    "answer": .string("allow-once")])

        let picked = try XCTUnwrap(HostActionWire.wire(
            RequestFlow.action(RequestAnswer.chooseFile, path: "/Users/x/bin/resize", request: budget)))
        XCTAssertEqual(picked.args["path"], .string("/Users/x/bin/resize"))
    }

    func testTheAllTrustedOptionIsOneAnswer() throws {
        let found = ProgramRequest(id: "r1", kind: .found, document: "todo.htmlclay", program: "todo-search",
                                   path: "/Users/x/bin/todo-search", revision: 4,
                                   answers: ["deny", "allow-document", "allow-all-trusted", "choose-file"])
        XCTAssertEqual(RequestControls.options(found, isSample: false),
                       [RequestAnswer.allowDocument, RequestAnswer.allowAllTrusted])
        XCTAssertTrue(RequestControls.showsAllTrusted(found, isSample: false))

        let wire = try XCTUnwrap(HostActionWire.wire(
            RequestFlow.action(RequestAnswer.allowAllTrusted, request: found)))
        XCTAssertEqual(wire.args, ["id": .string("r1"),
                                   "revision": .int(4),
                                   "answer": .string("allow-all-trusted")])
    }

    func testATeamDocumentCanNeverOfferAllTrusted() {
        let team = ProgramRequest(id: "r-team", kind: .teamDocument, document: "roadmap.html",
                                  program: "export-csv", folder: "Design Team", path: "/Users/x/bin/export-csv.py",
                                  revision: 1, answers: ["deny", "allow-document", "choose-file"])
        XCTAssertFalse(RequestControls.showsAllTrusted(team, isSample: false))
        XCTAssertFalse(RequestControls.options(team, isSample: false).contains(RequestAnswer.allowAllTrusted))
        XCTAssertNil(RequestControls.choice(RequestAnswer.allowAllTrusted, in: team, isSample: false))

        let sample = SampleData.request(.teamDocument).request
        XCTAssertFalse(RequestControls.showsAllTrusted(sample, isSample: true))
        XCTAssertFalse(RequestControls.options(sample, isSample: true).contains(RequestAnswer.allowAllTrusted))
    }

    func testOnlyOfferedAnswersAreShown() {
        let mainFolder = ProgramRequest(id: "r2", kind: .folderRead, document: "budget.htmlclay", program: String(),
                                        folder: "Documents", path: "~/Documents", revision: 1,
                                        answers: ["deny", "allow-once"])
        XCTAssertNil(RequestControls.choice(RequestAnswer.trustFolder, in: mainFolder, isSample: false))
        XCTAssertNotNil(RequestControls.choice(RequestAnswer.trustFolder, in: budget, isSample: false))
        XCTAssertNotNil(RequestControls.choice(RequestAnswer.allowOnce, in: budget, isSample: false))
        XCTAssertNotNil(RequestControls.choice(RequestAnswer.allowOnce, in: mainFolder, isSample: false))
    }

    func testARefusedAnswerKeepsTheWindowOnTheSameRequest() async {
        let item = budgetItem
        let roadmap = ProgramRequest(id: "r-roadmap", kind: .found, document: "roadmap.html", program: "export-csv",
                                     path: "/Users/x/bin/export-csv.py", revision: 1,
                                     answers: ["deny", "allow-document", "allow-all-trusted", "choose-file"])
        let queued = WaitingItem(id: "w-roadmap", kind: .programRequest, title: "roadmap.html wants", subjectID: roadmap.id)
        let refusal = HostRefusal(code: "request-changed", text: "That question changed.")
        let host = AnswerHost(.refused(refusal))
        let model = AppModel(host: host, initial: HostSnapshot(requests: [budget, roadmap], waiting: [item, queued]))

        let move = await RequestFlow.answer(RequestAnswer.allowOnce, request: budget, item: item, model: model)

        XCTAssertEqual(move, .stay)
        XCTAssertEqual(host.recorded, [.answer(requestID: "r3f09a1c22b7e", revision: 2, answer: "allow-once", path: String())])
        XCTAssertEqual(model.snapshot.waiting.map(\.id), ["w-budget", "w-roadmap"])
        XCTAssertEqual(model.snapshot.requests.map(\.id), ["r3f09a1c22b7e", "r-roadmap"])
        XCTAssertEqual(model.lastRefusal?.code, "request-changed")
    }

    func testAnAcceptedAnswerMovesToTheNextRequest() async {
        let item = budgetItem
        let next = WaitingItem(id: "w-roadmap", kind: .programRequest, title: "roadmap.html wants", subjectID: "r-team")
        let after = HostSnapshot(requests: [budget], waiting: [item, next])
        let host = AnswerHost(.done(after))
        let model = AppModel(host: host, initial: HostSnapshot(requests: [budget], waiting: [item, next]))

        let move = await RequestFlow.answer(RequestAnswer.allowOnce, request: budget, item: item, model: model)

        XCTAssertEqual(move, .next(next))
        XCTAssertEqual(host.recorded, [.answer(requestID: "r3f09a1c22b7e", revision: 2, answer: "allow-once", path: String())])
    }

    func testAnsweringALiveReadRequestLetsTheReadThrough() async throws {
        let host = try await LiveHost.start()
        let fable = host.home.appendingPathComponent("work/review/fable")
        let shared = host.home.appendingPathComponent("work/review/shared")
        try FileManager.default.createDirectory(at: fable, withIntermediateDirectories: true)
        try FileManager.default.createDirectory(at: shared, withIntermediateDirectories: true)
        let page = fable.appendingPathComponent("fable.htmlclay")
        try Data("<html><body>fable</body></html>".utf8).write(to: page)
        try Data("console.log('redpen')".utf8).write(to: shared.appendingPathComponent("redpen.js"))

        let model = AppModel(host: host.client, initial: try await host.client.load())
        _ = await model.submit(.openFile(path: page.path))

        let opened = try await openedURL(in: host.home.appendingPathComponent("open.log"))
        let read = Task { await fetch(opened, path: "/work/review/shared/redpen.js") }

        await waitUntil("the read question never reached the snapshot") {
            model.snapshot.requests.contains { $0.kind == .folderRead }
        }
        let request = try XCTUnwrap(model.snapshot.requests.first { $0.kind == .folderRead })
        XCTAssertEqual(request.answers, ["deny", "allow-once", "trust-folder"])
        XCTAssertTrue(RequestControls.offers(RequestAnswer.allowOnce, in: request, isSample: false))
        let item = try XCTUnwrap(model.snapshot.waiting.first { $0.subjectID == request.id })

        let move = await RequestFlow.answer(RequestAnswer.allowOnce, request: request, item: item, model: model)
        guard case let .next(remaining) = move else {
            XCTFail("the live host refused allow-once: \(model.lastRefusal?.code ?? String())")
            return
        }
        XCTAssertNil(remaining, "the answered request stayed in the queue")

        let (status, body) = await read.value
        XCTAssertEqual(status, 200, "an allowed read was not served")
        XCTAssertTrue(body.contains("redpen"), "the allowed read answered without the file's text: \(body)")
        XCTAssertFalse(model.snapshot.requests.contains { $0.id == request.id }, "the question stayed after the answer")
    }

    private func openedURL(in log: URL) async throws -> URL {
        let deadline = Date().addingTimeInterval(10)
        while Date() < deadline {
            if let text = try? String(contentsOf: log, encoding: .utf8),
               let line = text.split(separator: "\n").first(where: { $0.hasPrefix("http") }),
               let url = URL(string: String(line)) {
                return url
            }
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        throw LiveHostFailure.start("the host never logged the URL it opened at \(log.path)")
    }

    private func fetch(_ base: URL, path: String) async -> (Int, String) {
        var components = URLComponents(url: base, resolvingAgainstBaseURL: false)
        components?.path = path
        components?.query = nil
        guard let url = components?.url,
              let (data, response) = try? await URLSession.shared.data(from: url) else { return (0, String()) }
        return ((response as? HTTPURLResponse)?.statusCode ?? 0, String(decoding: data, as: UTF8.self))
    }
}
