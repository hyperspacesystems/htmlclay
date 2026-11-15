import XCTest
@testable import PreviewCore

private final class Log: @unchecked Sendable {
    private let lock = NSLock()
    private var stored: [String] = []

    func add(_ line: String) {
        lock.lock()
        stored.append(line)
        lock.unlock()
    }

    var lines: [String] {
        lock.lock()
        defer { lock.unlock() }
        return stored
    }

    func clear() {
        lock.lock()
        stored = []
        lock.unlock()
    }
}

@MainActor
private final class FakeDriver: UpdateDriver {
    private let log: Log
    private var events: (@MainActor (UpdateEvent) -> Void)?
    private var skip: (@MainActor (String) -> String?)?

    init(log: Log) { self.log = log }

    func start(events: @escaping @MainActor (UpdateEvent) -> Void, skipReason: @escaping @MainActor (String) -> String?) {
        log.add("driver.start")
        self.events = events
        skip = skipReason
    }

    func installNow() { log.add("install") }

    func checkNow() { log.add("check") }

    func send(_ event: UpdateEvent) { events?(event) }

    func skipReason(_ version: String) -> String? { skip?(version) }
}

private final class FakeHost: HostCommands, @unchecked Sendable {
    private let log: Log
    private let lock = NSLock()
    private var answers: [String]
    private let statusAnswer: String

    init(log: Log, answers: [String], status: String) {
        self.log = log
        self.answers = answers
        statusAnswer = status
    }

    func info() async throws -> String { "" }

    func status() async throws -> String {
        log.add("status")
        return statusAnswer
    }

    func confirm() async throws -> String { "" }

    func prepare(version: String, appPID: Int32, quit: Bool, force: Bool) async throws -> String {
        log.add("prepare \(version) \(quit ? "quit" : "restart")\(force ? " force" : "")")
        lock.lock()
        let answer = answers.isEmpty ? "prepared -" : answers.removeFirst()
        lock.unlock()
        if answer == "THROW" { throw HostCommandError.timedOut("prepare") }
        return answer
    }

    func abort(txid: String) async throws -> String {
        log.add("abort \(txid)")
        return "aborted"
    }
}

@MainActor
private final class FakePresenter: UpdatePresenter {
    private let log: Log
    private var unsaved: [QuitChoice]
    private var running: [Bool]

    init(log: Log, unsaved: [QuitChoice], running: [Bool]) {
        self.log = log
        self.unsaved = unsaved
        self.running = running
    }

    func askAboutUnsaved(_ lines: [String]) async -> QuitChoice {
        log.add("ask unsaved \(lines.joined(separator: "|"))")
        return unsaved.isEmpty ? .cancel : unsaved.removeFirst()
    }

    func askAboutRunningWork() async -> Bool {
        log.add("ask running")
        return running.isEmpty ? false : running.removeFirst()
    }

    func updateNotStarted() { log.add("notice not-started") }

    func restartFailed() { log.add("notice restart-failed") }
}

@MainActor
private final class Rig {
    let log: Log
    let driver: FakeDriver
    let host: FakeHost
    let presenter: FakePresenter
    let coordinator: UpdateCoordinator

    init(prepare: [String] = [], status: String = "ok", unsaved: [QuitChoice] = [], running: [Bool] = [],
         notStartedAfter: TimeInterval = 40) {
        let log = Log()
        self.log = log
        driver = FakeDriver(log: log)
        host = FakeHost(log: log, answers: prepare, status: status)
        presenter = FakePresenter(log: log, unsaved: unsaved, running: running)
        coordinator = UpdateCoordinator(driver: driver, host: host, presenter: presenter, appPID: 42,
                                        notStartedAfter: notStartedAfter)
    }
}

@MainActor
final class UpdateCoordinatorTests: XCTestCase {
    private func readyCoordinator(prepare: [String] = [], unsaved: [QuitChoice] = [], running: [Bool] = [],
                                  notStartedAfter: TimeInterval = 40) async -> Rig {
        let rig = Rig(prepare: prepare, unsaved: unsaved, running: running, notStartedAfter: notStartedAfter)
        await rig.coordinator.start(allowed: true)
        rig.driver.send(.ready("9.0.2"))
        rig.log.clear()
        return rig
    }

    private func quitter(_ log: Log, exits: Bool, result: Bool) -> (Bool) async -> Bool {
        { force in
            log.add("quit force=\(force ? "true" : "false")")
            if exits { log.add("exit") }
            return result
        }
    }

    func testPrepareThenQuitThenExitThenReply() async {
        let rig = await readyCoordinator(prepare: ["prepared t1"])
        let ok = await rig.coordinator.terminate(quitter: quitter(rig.log, exits: true, result: true))
        rig.log.add("reply \(ok)")
        XCTAssertEqual(rig.log.lines, ["prepare 9.0.2 quit", "quit force=true", "exit", "reply true"])
    }

    func testRestartRowPreparesARestart() async {
        let rig = await readyCoordinator(prepare: ["prepared t1"])
        rig.coordinator.restartToUpdate()
        let ok = await rig.coordinator.terminate(quitter: quitter(rig.log, exits: true, result: true))
        rig.log.add("reply \(ok)")
        XCTAssertEqual(rig.log.lines,
                       ["install", "prepare 9.0.2 restart", "quit force=true", "exit", "reply true"])
    }

    func testNothingPreparedQuitsUnforced() async {
        let rig = Rig()
        rig.log.clear()
        let ok = await rig.coordinator.terminate(quitter: quitter(rig.log, exits: true, result: true))
        rig.log.add("reply \(ok)")
        XCTAssertEqual(rig.log.lines, ["quit force=false", "exit", "reply true"])
    }

    func testUnsavedRetryThenQuitAnyway() async {
        let rig = await readyCoordinator(prepare: ["refused unsaved [\"site/a.html\"]",
                                                   "refused unsaved [\"site/a.html\"]",
                                                   "prepared t1"],
                                         unsaved: [.retry, .quitAnyway])
        let ok = await rig.coordinator.terminate(quitter: quitter(rig.log, exits: true, result: true))
        rig.log.add("reply \(ok)")
        XCTAssertEqual(rig.log.lines, ["prepare 9.0.2 quit", "ask unsaved site/a.html",
                                       "prepare 9.0.2 quit", "ask unsaved site/a.html",
                                       "prepare 9.0.2 quit force", "quit force=true", "exit", "reply true"])
    }

    func testUnsavedCancelDoesNotQuit() async {
        let rig = await readyCoordinator(prepare: ["refused unsaved [\"x\"]"], unsaved: [.cancel])
        let ok = await rig.coordinator.terminate(quitter: quitter(rig.log, exits: true, result: true))
        rig.log.add("reply \(ok)")
        XCTAssertEqual(rig.log.lines, ["prepare 9.0.2 quit", "ask unsaved x", "reply false"])
    }

    func testRunningWorkRefusedThenCancelled() async {
        let rig = await readyCoordinator(prepare: ["refused running-work"], running: [false])
        let ok = await rig.coordinator.terminate(quitter: quitter(rig.log, exits: true, result: true))
        rig.log.add("reply \(ok)")
        XCTAssertEqual(rig.log.lines, ["prepare 9.0.2 quit", "ask running", "reply false"])
    }

    func testRunningWorkRestartAnyway() async {
        let rig = await readyCoordinator(prepare: ["refused running-work", "prepared t1"], running: [true])
        let ok = await rig.coordinator.terminate(quitter: quitter(rig.log, exits: true, result: true))
        rig.log.add("reply \(ok)")
        XCTAssertEqual(rig.log.lines, ["prepare 9.0.2 quit", "ask running", "prepare 9.0.2 quit force",
                                       "quit force=true", "exit", "reply true"])
    }

    func testFailedPlainQuitStillQuits() async {
        let rig = await readyCoordinator(prepare: ["failed busy"])
        let ok = await rig.coordinator.terminate(quitter: quitter(rig.log, exits: true, result: true))
        rig.log.add("reply \(ok)")
        XCTAssertEqual(rig.log.lines, ["prepare 9.0.2 quit", "quit force=false", "exit", "reply true"])

        let thrown = await readyCoordinator(prepare: ["THROW"])
        let thrownOK = await thrown.coordinator.terminate(quitter: quitter(thrown.log, exits: true, result: true))
        thrown.log.add("reply \(thrownOK)")
        XCTAssertEqual(thrown.log.lines, ["prepare 9.0.2 quit", "quit force=false", "exit", "reply true"])
    }

    func testFailedRestartCancelsWithNotice() async {
        let rig = await readyCoordinator(prepare: ["failed busy"])
        rig.coordinator.restartToUpdate()
        let ok = await rig.coordinator.terminate(quitter: quitter(rig.log, exits: true, result: true))
        rig.log.add("reply \(ok)")
        XCTAssertEqual(rig.log.lines, ["install", "prepare 9.0.2 restart", "notice restart-failed", "reply false"])
    }

    func testCancelledQuitAborts() async {
        let rig = await readyCoordinator(prepare: ["prepared t1"])
        let ok = await rig.coordinator.terminate(quitter: quitter(rig.log, exits: false, result: false))
        rig.log.add("reply \(ok)")
        XCTAssertEqual(rig.log.lines, ["prepare 9.0.2 quit", "quit force=true", "abort t1", "reply false"])
    }

    func testSparkleFailureAfterPrepareAborts() async {
        let rig = await readyCoordinator(prepare: ["prepared t1"])
        let plan = await rig.coordinator.beforeQuit()
        XCTAssertEqual(plan, .proceed(force: true))
        rig.driver.send(.installFailed("x"))
        for _ in 0..<100 where !rig.log.lines.contains("abort t1") {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(rig.log.lines.last, "abort t1")
    }

    func testPrepareWithoutTxidDoesNotAbort() async {
        let rig = await readyCoordinator(prepare: ["prepared -"])
        let ok = await rig.coordinator.terminate(quitter: quitter(rig.log, exits: false, result: false))
        rig.log.add("reply \(ok)")
        XCTAssertEqual(rig.log.lines, ["prepare 9.0.2 quit", "quit force=true", "reply false"])
        XCTAssertFalse(rig.log.lines.contains { $0.hasPrefix("abort ") })
    }

    func testNotAllowedNeverStartsDriver() async {
        let rig = Rig()
        await rig.coordinator.start(allowed: false)
        XCTAssertEqual(rig.log.lines, [])
        XCTAssertFalse(rig.coordinator.started)

        XCTAssertFalse(UpdateGate.allows(feedURL: nil, publicKey: "k", bundlePath: "/Applications/X.app",
                                         writable: true, version: "2.0.0"))
        XCTAssertFalse(UpdateGate.allows(feedURL: "https://example.com/appcast.xml", publicKey: "",
                                         bundlePath: "/Applications/X.app", writable: true, version: "2.0.0"))
        XCTAssertFalse(UpdateGate.allows(feedURL: "https://example.com/appcast.xml", publicKey: "k",
                                         bundlePath: "/private/var/folders/AppTranslocation/X.app",
                                         writable: true, version: "2.0.0"))
        XCTAssertFalse(UpdateGate.allows(feedURL: "https://example.com/appcast.xml", publicKey: "k",
                                         bundlePath: "/Applications/X.app", writable: false, version: "2.0.0"))
        XCTAssertFalse(UpdateGate.allows(feedURL: "https://example.com/appcast.xml", publicKey: "k",
                                         bundlePath: "/Applications/X.app", writable: true, version: "2.0"))
        XCTAssertTrue(UpdateGate.allows(feedURL: "https://example.com/appcast.xml", publicKey: "k",
                                        bundlePath: "/Applications/X.app", writable: true, version: "2.0.0"))
    }

    func testStatusOffNeverStartsDriver() async {
        let rig = Rig(status: "off config sets \"updates\": \"off\"")
        await rig.coordinator.start(allowed: true)
        XCTAssertEqual(rig.log.lines, ["status"])
        XCTAssertFalse(rig.coordinator.started)
    }

    func testFailedVersionIsSkipped() async {
        let rig = Rig(status: "failed 9.0.2 it did not start")
        await rig.coordinator.start(allowed: true)
        XCTAssertTrue(rig.coordinator.started)
        XCTAssertEqual(rig.driver.skipReason("9.0.2"), "it did not start")
        XCTAssertNil(rig.driver.skipReason("9.0.3"))
    }

    func testNotStartedNotice() async {
        let rig = await readyCoordinator(notStartedAfter: 0.05)
        rig.coordinator.restartToUpdate()
        try? await Task.sleep(nanoseconds: 200_000_000)
        XCTAssertEqual(rig.log.lines, ["install", "notice not-started"])

        let quiet = await readyCoordinator(notStartedAfter: 0.05)
        quiet.coordinator.restartToUpdate()
        let ok = await quiet.coordinator.terminate(quitter: quitter(quiet.log, exits: true, result: true))
        quiet.log.add("reply \(ok)")
        try? await Task.sleep(nanoseconds: 200_000_000)
        XCTAssertEqual(quiet.log.lines, ["install", "prepare 9.0.2 restart", "quit force=true", "exit", "reply true"])
    }
}
