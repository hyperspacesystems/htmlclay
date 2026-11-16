import XCTest
@testable import PreviewCore

@MainActor
final class FakeLoginItem: LoginItemService {
    var status: LoginItemStatus = .notRegistered
    private(set) var calls: [String] = []
    var registerTurns: LoginItemStatus = .enabled

    func register() throws {
        calls.append("register")
        status = registerTurns
    }

    func unregister() throws {
        calls.append("unregister")
        status = .notRegistered
    }
}

@MainActor
final class LoginItemReconcilerTests: XCTestCase {
    private var home: URL!
    private var plist: URL!
    private var defaults: UserDefaults!
    private var suite: String!
    private var service: FakeLoginItem!

    override func setUp() async throws {
        try await super.setUp()
        home = FileManager.default.temporaryDirectory.appendingPathComponent("LoginItemReconcilerTests-\(UUID())")
        plist = home.appendingPathComponent("Library/LaunchAgents/com.htmlclay.plist")
        try FileManager.default.createDirectory(at: plist.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data("legacy".utf8).write(to: plist)
        suite = "LoginItemReconcilerTests-\(UUID())"
        defaults = UserDefaults(suiteName: suite)!
        defaults.removePersistentDomain(forName: suite)
        service = FakeLoginItem()
    }

    override func tearDown() async throws {
        defaults.removePersistentDomain(forName: suite)
        try? FileManager.default.removeItem(at: home)
        try await super.tearDown()
    }

    private func make() -> LoginItemReconciler {
        LoginItemReconciler(service: service, legacyPlist: plist, defaults: defaults, delay: 0.05)
    }

    private func wait() async {
        try? await Task.sleep(nanoseconds: 200_000_000)
    }

    private var plistExists: Bool { FileManager.default.fileExists(atPath: plist.path) }

    func testWantedAndEnabledDeletesThePlist() async {
        let reconciler = make()
        service.status = .notRegistered
        reconciler.update(wanted: true)
        await wait()
        XCTAssertEqual(service.calls, ["register"])
        XCTAssertFalse(plistExists)
        XCTAssertTrue(defaults.bool(forKey: LoginItemReconciler.movedKey))
    }

    func testWantedButNeedsApprovalKeepsThePlist() async {
        let reconciler = make()
        service.registerTurns = .requiresApproval
        reconciler.update(wanted: true)
        await wait()
        XCTAssertEqual(service.calls, ["register"])
        XCTAssertTrue(plistExists)
        XCTAssertFalse(defaults.bool(forKey: LoginItemReconciler.movedKey))
    }

    func testNotWantedUnregistersAndDeletes() async {
        let reconciler = make()
        reconciler.update(wanted: false)
        await wait()
        XCTAssertEqual(service.calls, ["unregister"])
        XCTAssertFalse(plistExists)
    }

    func testNothingBeforeTheDelay() async {
        let reconciler = make()
        reconciler.update(wanted: true)
        XCTAssertTrue(service.calls.isEmpty)
        XCTAssertTrue(plistExists)
        await wait()
        XCTAssertEqual(service.calls, ["register"])
    }

    func testMovedOnceIsNotOverridden() async {
        defaults.set(true, forKey: LoginItemReconciler.movedKey)
        let reconciler = make()
        service.status = .notRegistered
        reconciler.update(wanted: true)
        await wait()
        XCTAssertTrue(service.calls.isEmpty)
        reconciler.update(wanted: false)
        reconciler.update(wanted: true)
        XCTAssertEqual(service.calls, ["unregister", "register"])
    }

    func testSameValueDoesNotRepeat() async {
        let reconciler = make()
        reconciler.update(wanted: true)
        await wait()
        XCTAssertEqual(service.calls, ["register"])
        reconciler.update(wanted: true)
        XCTAssertEqual(service.calls, ["register"])
    }
}
