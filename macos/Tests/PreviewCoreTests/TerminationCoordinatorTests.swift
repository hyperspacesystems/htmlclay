import XCTest
@testable import PreviewCore

@MainActor
private final class ScriptedQuitHost: HostClient {
    private(set) var recorded: [HostAction] = []
    var answers: [HostOutcome] = []
    var answerDelay: TimeInterval = 0
    var holdFirst = false
    private var held: [CheckedContinuation<HostOutcome, Never>] = []

    func load() async throws -> HostSnapshot { HostSnapshot() }

    func perform(_ action: HostAction) async -> HostOutcome {
        recorded.append(action)
        if answerDelay > 0 { try? await Task.sleep(nanoseconds: UInt64(answerDelay * 1_000_000_000)) }
        let holdsFirst = holdFirst
        holdFirst = false
        if holdsFirst {
            return await withCheckedContinuation { held.append($0) }
        }
        return answers.isEmpty ? .done(HostSnapshot()) : answers.removeFirst()
    }

    func releaseHeld() {
        let waiting = held
        held = []
        for continuation in waiting { continuation.resume(returning: .done(HostSnapshot())) }
    }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .success(HostCheckAnswer()) }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription {
        HostSubscription {}
    }
}

@MainActor
private final class ScriptedPresenter: QuitPresenter {
    private(set) var asked: [[String]] = []
    private var answers: [QuitChoice]

    init(_ answers: [QuitChoice]) { self.answers = answers }

    func askAboutUnsaved(_ lines: [String]) async -> QuitChoice {
        asked.append(lines)
        return answers.isEmpty ? .cancel : answers.removeFirst()
    }
}

@MainActor
private final class ScriptedQuitProcess: QuitHostProcess {
    var isRunning = true
    private(set) var waits = 0
    private(set) var stops = 0
    var exitedOnWait = true

    func waitUntilExited(timeout: TimeInterval) async -> Bool {
        waits += 1
        if exitedOnWait { isRunning = false }
        return exitedOnWait
    }

    func stop(timeout: TimeInterval) async -> Bool {
        stops += 1
        isRunning = false
        return true
    }
}

@MainActor
final class TerminationCoordinatorTests: XCTestCase {
    private func unsaved(_ documents: [String] = ["notes/unsaved.htmlclay"]) -> HostOutcome {
        .refused(HostRefusal(code: "quit-unsaved", text: "Some edits aren't saved yet.", documents: documents))
    }

    private func make(host: ScriptedQuitHost, process: ScriptedQuitProcess, presenter: ScriptedPresenter,
                      replyBound: TimeInterval = 5) -> TerminationCoordinator {
        TerminationCoordinator(host: host, process: process, presenter: presenter,
                               replyBound: replyBound, exitBound: 5)
    }

    func testAStoppedHostQuitsAtOnce() async {
        let host = ScriptedQuitHost()
        let process = ScriptedQuitProcess()
        process.isRunning = false
        let presenter = ScriptedPresenter([.cancel])
        let coordinator = make(host: host, process: process, presenter: presenter)

        let quit = await coordinator.quit()

        XCTAssertTrue(quit)
        XCTAssertEqual(host.recorded, [])
        XCTAssertEqual(presenter.asked, [])
        XCTAssertEqual(process.stops, 0)
    }

    func testAnAcceptedQuitWaitsForTheHostToExit() async {
        let host = ScriptedQuitHost()
        let process = ScriptedQuitProcess()
        let presenter = ScriptedPresenter([.cancel])
        let coordinator = make(host: host, process: process, presenter: presenter)

        let quit = await coordinator.quit()

        XCTAssertTrue(quit)
        XCTAssertEqual(host.recorded, [.quit(force: false)])
        XCTAssertEqual(process.waits, 1)
        XCTAssertEqual(process.stops, 0)
        XCTAssertEqual(presenter.asked, [])
    }

    func testAnUnsavedRefusalNamesTheDocuments() async {
        let host = ScriptedQuitHost()
        host.answers = [unsaved(["notes/a.htmlclay", "served/b.htmlclay"])]
        let process = ScriptedQuitProcess()
        let presenter = ScriptedPresenter([.cancel])
        let coordinator = make(host: host, process: process, presenter: presenter)

        let quit = await coordinator.quit()

        XCTAssertFalse(quit)
        XCTAssertEqual(presenter.asked, [["notes/a.htmlclay", "served/b.htmlclay"]])
        XCTAssertEqual(process.stops, 0)
        XCTAssertTrue(process.isRunning)
    }

    func testRetryAsksTheHostAgain() async {
        let host = ScriptedQuitHost()
        host.answers = [unsaved(), .done(HostSnapshot())]
        let process = ScriptedQuitProcess()
        let presenter = ScriptedPresenter([.retry])
        let coordinator = make(host: host, process: process, presenter: presenter)

        let quit = await coordinator.quit()

        XCTAssertTrue(quit)
        XCTAssertEqual(host.recorded, [.quit(force: false), .quit(force: false)])
        XCTAssertEqual(presenter.asked.count, 1)
        XCTAssertEqual(process.waits, 1)
    }

    func testQuitAnywayForcesTheQuit() async {
        let host = ScriptedQuitHost()
        host.answers = [unsaved(), .done(HostSnapshot())]
        let process = ScriptedQuitProcess()
        let presenter = ScriptedPresenter([.quitAnyway])
        let coordinator = make(host: host, process: process, presenter: presenter)

        let quit = await coordinator.quit()

        XCTAssertTrue(quit)
        XCTAssertEqual(host.recorded, [.quit(force: false), .quit(force: true)])
        XCTAssertEqual(process.waits, 1)
        XCTAssertEqual(process.stops, 0)
    }

    func testAForcedQuitThatIsRefusedStopsTheHost() async {
        let host = ScriptedQuitHost()
        host.answers = [unsaved(), unsaved()]
        let process = ScriptedQuitProcess()
        let presenter = ScriptedPresenter([.quitAnyway])
        let coordinator = make(host: host, process: process, presenter: presenter)

        let quit = await coordinator.quit()

        XCTAssertTrue(quit)
        XCTAssertEqual(host.recorded.last, .quit(force: true))
        XCTAssertEqual(process.stops, 1)
        XCTAssertFalse(process.isRunning)
    }

    func testCancelKeepsTheHostRunning() async {
        let host = ScriptedQuitHost()
        host.answers = [unsaved()]
        let process = ScriptedQuitProcess()
        let presenter = ScriptedPresenter([.cancel])
        let coordinator = make(host: host, process: process, presenter: presenter)

        let quit = await coordinator.quit()

        XCTAssertFalse(quit)
        XCTAssertEqual(process.stops, 0)
        XCTAssertEqual(process.waits, 0)
        XCTAssertTrue(process.isRunning)
    }

    func testAnUnreachableHostIsShownAsNoAnswer() async {
        let host = ScriptedQuitHost()
        host.answers = [.refused(HostRefusal(code: "unreachable", text: "HTML Clay isn't responding.")),
                        .done(HostSnapshot())]
        let process = ScriptedQuitProcess()
        let presenter = ScriptedPresenter([.retry])
        let coordinator = make(host: host, process: process, presenter: presenter)

        let quit = await coordinator.quit()

        XCTAssertTrue(quit)
        XCTAssertEqual(presenter.asked, [[Strings.Quit.noAnswer]])
    }

    func testASilentHostAsksAfterTheBound() async {
        let host = ScriptedQuitHost()
        host.answerDelay = 3
        let process = ScriptedQuitProcess()
        let presenter = ScriptedPresenter([.cancel])
        let coordinator = make(host: host, process: process, presenter: presenter, replyBound: 0.2)

        let began = Date()
        let quit = await coordinator.quit()

        XCTAssertFalse(quit)
        XCTAssertLessThan(Date().timeIntervalSince(began), 2)
        XCTAssertEqual(presenter.asked, [[Strings.Quit.noAnswer]])
        XCTAssertEqual(process.stops, 0)
    }

    func testASecondQuitWhileOneRunsAnswersFalseAtOnce() async {
        let host = ScriptedQuitHost()
        host.holdFirst = true
        let process = ScriptedQuitProcess()
        let presenter = ScriptedPresenter([.cancel])
        let coordinator = make(host: host, process: process, presenter: presenter)

        let first = Task { await coordinator.quit() }
        let deadline = Date().addingTimeInterval(2)
        while host.recorded.isEmpty && Date() < deadline { await Task.yield() }
        XCTAssertEqual(host.recorded.count, 1)

        let second = await coordinator.quit()
        XCTAssertFalse(second)
        XCTAssertEqual(host.recorded.count, 1)

        host.releaseHeld()
        let firstAnswer = await first.value
        XCTAssertTrue(firstAnswer)
    }

    func testASecondRestartStartsNoSecondHost() {
        XCTAssertTrue(HostRestartPolicy.shouldStart(current: nil))
        XCTAssertTrue(HostRestartPolicy.shouldStart(current: .failed(HostFailure(v: 1, code: "already-running", port: nil, message: nil))))
        XCTAssertTrue(HostRestartPolicy.shouldStart(current: .stopped(reason: "exited 4 times in a row")))
        XCTAssertFalse(HostRestartPolicy.shouldStart(current: .starting))
        XCTAssertFalse(HostRestartPolicy.shouldStart(current: .running(
            HostReady(v: 1, shell: "http://127.0.0.1:4545", port: 4545, epoch: "e1", pid: 42, version: "1"))))
    }
}
