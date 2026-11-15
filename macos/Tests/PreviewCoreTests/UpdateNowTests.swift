import XCTest
@testable import PreviewCore

@MainActor
private final class FakeClock {
    var now = Date(timeIntervalSince1970: 1_000_000)
}

@MainActor
private final class NowRig {
    let log = Log()
    let driver: FakeDriver
    let host: FakeHost
    let presenter: FakePresenter
    let clock = FakeClock()
    let coordinator: UpdateCoordinator

    init(status: String = "ok", prepare: [String] = [], unsaved: [QuitChoice] = []) {
        driver = FakeDriver(log: log)
        host = FakeHost(log: log, answers: prepare, status: status)
        presenter = FakePresenter(log: log, unsaved: unsaved, running: [])
        coordinator = UpdateCoordinator(driver: driver, host: host, presenter: presenter, appPID: 42,
                                        version: "9.0.1", now: { [clock] in clock.now })
    }

    func begin(allowed: Bool = true) async {
        await coordinator.start(allowed: allowed)
        log.clear()
    }
}

@MainActor
final class UpdateNowTests: XCTestCase {
    private let own = "/Applications/HTML Clay.app"

    private func request(_ path: String, _ version: String = "9.0.2",
                         reply: String = UpdateNowWire.replyPrefix + "r") -> String {
        UpdateNowWire.encode(.init(bundlePath: path, version: version, reply: reply))
    }

    func testWireRoundTrip() {
        let r = UpdateNowWire.Request(bundlePath: own, version: "9.0.2", reply: UpdateNowWire.replyPrefix + "abc")
        XCTAssertEqual(UpdateNowWire.decode(UpdateNowWire.encode(r)), r)
        XCTAssertNil(UpdateNowWire.decode("a\nb"))
        XCTAssertNil(UpdateNowWire.decode("a\n\n\(UpdateNowWire.replyPrefix)x"))
        XCTAssertNil(UpdateNowWire.decode("\(own)\n\n\(UpdateNowWire.replyPrefix)x"))
        XCTAssertNil(UpdateNowWire.decode("a\n9.0.2\nreply"))
    }

    func testAnswerIgnoresAnotherBundle() {
        var decided: [String] = []
        let other = UpdateNowWire.answer(object: request("/Applications/Other.app"), ownBundlePath: own,
                                        decide: { decided.append($0); return "accepted" })
        XCTAssertNil(other)
        XCTAssertEqual(decided, [])

        for ownPath in [own, own + "/", "/Applications/../Applications/HTML Clay.app"] {
            let answer = UpdateNowWire.answer(object: request(own), ownBundlePath: ownPath,
                                              decide: { "accepted \($0)" })
            XCTAssertEqual(answer?.reply, UpdateNowWire.replyPrefix + "r")
            XCTAssertEqual(answer?.line, "accepted 9.0.2")
        }
    }

    func testExitCodes() {
        XCTAssertEqual(UpdateNowWire.exitCode(for: "accepted"), 0)
        XCTAssertEqual(UpdateNowWire.exitCode(for: "already 9.0.1"), 0)
        XCTAssertEqual(UpdateNowWire.exitCode(for: "refused x"), 3)
        XCTAssertEqual(UpdateNowWire.exitCode(for: "failed x"), 5)
        XCTAssertEqual(UpdateNowWire.exitCode(for: "starting"), 6)
    }

    func testBadVersion() async {
        let rig = NowRig()
        await rig.begin()
        XCTAssertEqual(rig.coordinator.updateNow("9.0"), "refused \"9.0\" is not X.Y.Z")
    }

    func testAlready() async {
        let rig = NowRig()
        await rig.begin()
        XCTAssertEqual(rig.coordinator.updateNow("9.0.1"), "already 9.0.1")
    }

    func testNotNewer() async {
        let rig = NowRig()
        await rig.begin()
        XCTAssertEqual(rig.coordinator.updateNow("9.0.0"),
                       "refused v9.0.0 is not newer than the running v9.0.1")
        XCTAssertTrue(UpdateGate.isNewer("10.0.0", than: "9.9.9"))
        XCTAssertFalse(UpdateGate.isNewer("9.0.1", than: "9.0.1"))
    }

    func testOffWhenNotAllowed() async {
        let rig = NowRig()
        await rig.begin(allowed: false)
        XCTAssertTrue(rig.coordinator.updateNow("9.0.2").hasPrefix("refused self-update is off: "))
    }

    func testOffFromStatus() async {
        let rig = NowRig(status: "off config sets \"updates\": \"off\"")
        await rig.begin()
        XCTAssertEqual(rig.coordinator.updateNow("9.0.2"),
                       "refused self-update is off: config sets \"updates\": \"off\"")
    }

    func testStartingBeforeStart() {
        let rig = NowRig()
        XCTAssertEqual(rig.coordinator.updateNow("9.0.2"), "starting")
    }

    func testRolledBackVersionRefused() async {
        let rig = NowRig(status: "failed 9.0.2 it did not start")
        await rig.begin()
        XCTAssertEqual(rig.coordinator.updateNow("9.0.2"), "refused it did not start")
    }

    func testStartFailed() async {
        let rig = NowRig()
        await rig.begin()
        rig.driver.send(.startFailed("no feed"))
        XCTAssertEqual(rig.coordinator.updateNow("9.0.2"), "refused Sparkle could not start: no feed")
    }

    func testPreparedTargetRestarts() async {
        let rig = NowRig()
        await rig.begin()
        rig.driver.send(.ready("9.0.2"))
        XCTAssertEqual(rig.coordinator.updateNow("9.0.2"), "accepted")
        XCTAssertEqual(rig.log.lines.filter { $0 == "install" }.count, 1)
        XCTAssertEqual(rig.coordinator.updateNow("9.0.2"), "accepted")
        XCTAssertEqual(rig.log.lines.filter { $0 == "install" }.count, 1)
    }

    func testOtherVersionPrepared() async {
        let rig = NowRig()
        await rig.begin()
        rig.driver.send(.ready("9.0.3"))
        XCTAssertEqual(rig.coordinator.updateNow("9.0.2"),
                       "failed Sparkle holds v9.0.3 prepared, which installs at the next quit")
        XCTAssertFalse(rig.log.lines.contains("check"))
        XCTAssertFalse(rig.log.lines.contains("install"))
    }

    func testNothingPreparedChecksThenRestartsOnReady() async {
        let rig = NowRig()
        await rig.begin()
        XCTAssertEqual(rig.coordinator.updateNow("9.0.2"), "accepted")
        XCTAssertEqual(rig.log.lines, ["check"])
        rig.driver.send(.ready("9.0.2"))
        XCTAssertEqual(rig.log.lines, ["check", "install"])
    }

    func testRequestExpires() async {
        let rig = NowRig()
        await rig.begin()
        XCTAssertEqual(rig.coordinator.updateNow("9.0.2"), "accepted")
        rig.clock.now = rig.clock.now.addingTimeInterval(61)
        rig.driver.send(.ready("9.0.2"))
        XCTAssertFalse(rig.log.lines.contains("install"))
    }

    func testDeclinedRestart() async {
        let rig = NowRig(prepare: ["refused unsaved [\"x\"]"], unsaved: [.cancel])
        await rig.begin()
        rig.driver.send(.ready("9.0.2"))
        rig.coordinator.restartToUpdate()
        let ok = await rig.coordinator.terminate { _ in true }
        XCTAssertFalse(ok)
        XCTAssertEqual(rig.coordinator.updateNow("9.0.2"), "failed the restart to v9.0.2 was cancelled on screen")
    }
}
