import XCTest
@testable import PreviewCore
import Darwin

private struct HostTestFailure: Error, CustomStringConvertible {
    let message: String
    var description: String { message }
}

@MainActor
final class HostProcessTests: XCTestCase {
    private static let requireVariable = "HTMLCLAY_REQUIRE_LIVE_HOST"

    private static var binary: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent(".build/host-for-tests/htmlclay")
    }

    private func requireRealHost() throws -> URL {
        let binary = Self.binary
        guard FileManager.default.isExecutableFile(atPath: binary.path) else {
            if ProcessInfo.processInfo.environment[Self.requireVariable] == "1" {
                throw HostTestFailure(message: "the live host is missing: run scripts/build-host-for-tests.sh")
            }
            throw XCTSkip("run scripts/build-host-for-tests.sh to build \(binary.path)")
        }
        return binary
    }

    private struct Home {
        var home: URL
        var base: URL
        var port: Int
    }

    private func makeHome() throws -> Home {
        var template = Array("/tmp/hc-XXXXXXXX".utf8CString)
        let made = template.withUnsafeMutableBufferPointer { buffer -> String? in
            guard let path = mkdtemp(buffer.baseAddress!) else { return nil }
            return String(cString: path)
        }
        guard let made else { throw HostTestFailure(message: "mkdtemp failed: \(errno)") }
        guard let resolved = made.withCString({ realpath($0, nil) }) else {
            try? FileManager.default.removeItem(atPath: made)
            throw HostTestFailure(message: "realpath failed for \(made): \(errno)")
        }
        defer { free(resolved) }
        let homePath = String(cString: resolved)
        let basePath = homePath + "/c"
        let configDir = basePath + "/htmlclay"
        try FileManager.default.createDirectory(atPath: configDir, withIntermediateDirectories: true)
        let port = try freeLoopbackPort()
        let config = "{\"startOnLogin\": false, \"port\": \(port), \"updates\": \"off\"}\n"
        try Data(config.utf8).write(to: URL(fileURLWithPath: configDir + "/config.json"))
        return Home(home: URL(fileURLWithPath: homePath), base: URL(fileURLWithPath: basePath), port: port)
    }

    private func launch(binary: URL, home: Home, readyTimeout: TimeInterval = 30) -> HostLaunch {
        var launch = HostLaunch(binary: binary, environment: ["HOME": home.home.path, "HTMLCLAY_CONFIG_BASE": home.base.path])
        launch.readyTimeout = readyTimeout
        return launch
    }

    private func tearDownLater(_ process: HostProcess, home: URL?) {
        addTeardownBlock {
            _ = await process.stop(timeout: 5)
            if let home { try? FileManager.default.removeItem(at: home) }
        }
    }

    private func waitUntil(_ message: String, timeout: TimeInterval = 10, _ check: () -> Bool) async {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if check() { return }
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertTrue(check(), message)
    }

    private func makeScript(_ home: Home, _ body: String) throws -> URL {
        let url = home.home.appendingPathComponent("fake-host.sh")
        try Data("#!/bin/sh\n\(body)".utf8).write(to: url)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: url.path)
        return url
    }

    private static let readyLine = #"printf 'READY {"v":1,"shell":"http://127.0.0.1:1","port":1,"epoch":"fake","pid":%s,"version":"fake"}\n' "$$""#

    func testStartsAndHandsBackAWorkingClient() async throws {
        let binary = try requireRealHost()
        let home = try makeHome()
        let process = HostProcess(launch: launch(binary: binary, home: home))
        tearDownLater(process, home: home.home)

        var handed: (HostReady, LoopbackHostClient)?
        process.onReady = { ready, client in handed = (ready, client) }

        let state = await process.start()
        guard case let .running(ready) = state else {
            XCTFail("the host did not come up: \(process.state); stderr: \(process.stderrTail)")
            return
        }
        let running = try XCTUnwrap(process.pid)
        XCTAssertEqual(ready.pid, Int(running))

        let client = try XCTUnwrap(handed).1
        let snapshot = try await client.load()
        XCTAssertEqual(snapshot.epoch, ready.epoch)

        let exited = await process.stop()
        XCTAssertTrue(exited, "the host did not exit after stdin closed")
        XCTAssertNil(process.pid)
        XCTAssertEqual(kill(running, 0), -1)
        XCTAssertEqual(errno, ESRCH)
    }

    func testAlreadyRunningIsATypedFailureAndNotRetried() async throws {
        let binary = try requireRealHost()
        let home = try makeHome()
        let first = HostProcess(launch: launch(binary: binary, home: home))
        tearDownLater(first, home: home.home)

        let firstState = await first.start()
        guard case .running = firstState else {
            XCTFail("the first host did not come up: \(first.state); stderr: \(first.stderrTail)")
            return
        }

        let second = HostProcess(launch: launch(binary: binary, home: home))
        var starts = 0
        second.onState = { state in
            if case .starting = state { starts += 1 }
        }
        let state = await second.start()
        guard case let .failed(failure) = state else {
            XCTFail("the second host was not refused: \(state); stderr: \(second.stderrTail)")
            return
        }
        XCTAssertEqual(failure.code, "already-running")

        try? await Task.sleep(nanoseconds: 2_000_000_000)
        XCTAssertEqual(starts, 1, "the refused host was started again")
    }

    func testPortTakenIsATypedFailure() async throws {
        let binary = try requireRealHost()
        let home = try makeHome()
        let held = try holdPort(home.port)
        defer { close(held) }

        let process = HostProcess(launch: launch(binary: binary, home: home))
        tearDownLater(process, home: home.home)

        let state = await process.start()
        guard case let .failed(failure) = state else {
            XCTFail("the host was not refused: \(state); stderr: \(process.stderrTail)")
            return
        }
        XCTAssertEqual(failure.code, "port-taken")
        XCTAssertEqual(failure.port, home.port)
    }

    func testKilledHostRestartsWithANewEpoch() async throws {
        let binary = try requireRealHost()
        let home = try makeHome()
        let process = HostProcess(launch: launch(binary: binary, home: home))
        tearDownLater(process, home: home.home)

        var readies: [HostReady] = []
        var clients: [LoopbackHostClient] = []
        process.onReady = { ready, client in
            readies.append(ready)
            clients.append(client)
        }

        let state = await process.start()
        guard case .running = state else {
            XCTFail("the host did not come up: \(process.state); stderr: \(process.stderrTail)")
            return
        }
        let firstPID = try XCTUnwrap(process.pid)
        XCTAssertEqual(kill(firstPID, SIGKILL), 0)

        await waitUntil("the host was not restarted", timeout: 15) { readies.count == 2 }
        guard readies.count == 2 else { return }
        XCTAssertNotEqual(readies[0].epoch, readies[1].epoch)
        XCTAssertNotEqual(readies[0].pid, readies[1].pid)
        XCTAssertNotEqual(Int(firstPID), readies[1].pid)

        let snapshot = try await clients[1].load()
        XCTAssertEqual(snapshot.epoch, readies[1].epoch)
    }

    func testFastFailuresStop() async throws {
        let home = try makeHome()
        let script = try makeScript(home, Self.readyLine + "\nexit 1\n")
        let process = HostProcess(launch: launch(binary: script, home: home, readyTimeout: 5),
                                  restartDelays: [0.05], fastFailureLimit: 3)
        tearDownLater(process, home: home.home)

        var starts = 0
        var readies = 0
        process.onState = { state in
            switch state {
            case .starting: starts += 1
            case .running: readies += 1
            default: break
            }
        }

        let first = await process.start()
        guard case .running = first else {
            XCTFail("the fake host never came up: \(first); stderr: \(process.stderrTail)")
            return
        }
        await waitUntil("the fast failures never stopped the host") {
            if case .stopped = process.state { return true }
            return false
        }
        guard case .stopped = process.state else { return }

        XCTAssertEqual(readies - 1, 3, "restarted a different number of times than fastFailureLimit")
        let startsAtStop = starts
        let readiesAtStop = readies
        try? await Task.sleep(nanoseconds: 1_000_000_000)
        XCTAssertEqual(starts, startsAtStop, "the host was started again after it was declared stopped")
        XCTAssertEqual(readies, readiesAtStop, "the host came up again after it was declared stopped")
    }

    func testBadReadyIsRefused() async throws {
        let home = try makeHome()
        let ready = #"printf 'READY {"v":1,"shell":"http://10.0.0.1:5000","port":1,"epoch":"fake","pid":%s,"version":"fake"}\n' "$$""#
        let script = try makeScript(home, ready + "\nexit 0\n")
        let process = HostProcess(launch: launch(binary: script, home: home, readyTimeout: 5), restartDelays: [0.05])
        tearDownLater(process, home: home.home)

        var starts = 0
        process.onState = { state in
            if case .starting = state { starts += 1 }
        }

        let state = await process.start()
        guard case let .failed(failure) = state else {
            XCTFail("a READY on another host was accepted: \(state)")
            return
        }
        XCTAssertEqual(failure.code, "bad-ready")

        try? await Task.sleep(nanoseconds: 1_000_000_000)
        XCTAssertEqual(starts, 1, "a refused READY was started again")
    }

    func testExitBeforeReady() async throws {
        let home = try makeHome()
        let script = try makeScript(home, "exit 3\n")
        let process = HostProcess(launch: launch(binary: script, home: home, readyTimeout: 5), restartDelays: [0.05])
        tearDownLater(process, home: home.home)

        var starts = 0
        process.onState = { state in
            if case .starting = state { starts += 1 }
        }

        let state = await process.start()
        guard case let .stopped(reason) = state else {
            XCTFail("a host that exited before READY was not reported stopped: \(state)")
            return
        }
        XCTAssertTrue(reason.contains("3"), "reason was \(reason)")

        try? await Task.sleep(nanoseconds: 1_000_000_000)
        XCTAssertEqual(starts, 1, "a host that exited before READY was started again")
    }

    func testNoisyStderrDoesNotBlock() async throws {
        let home = try makeHome()
        let body = "head -c 1048576 /dev/zero | tr '\\0' x >&2\n" + Self.readyLine + "\nsleep 30\n"
        let script = try makeScript(home, body)
        let process = HostProcess(launch: launch(binary: script, home: home, readyTimeout: 10), restartDelays: [0.05])
        tearDownLater(process, home: home.home)

        let state = await process.start()
        guard case .running = state else {
            XCTFail("a noisy stderr kept the host from coming up: \(state)")
            return
        }
        XCTAssertLessThanOrEqual(process.stderrTail.utf8.count, 65536)

        let exited = await process.stop(timeout: 1)
        XCTAssertFalse(exited, "the fake host exited on its own; it should have been terminated")
        XCTAssertNil(process.pid)
    }

    func testSecretNeverInState() async throws {
        let binary = try requireRealHost()
        let home = try makeHome()
        let process = HostProcess(launch: launch(binary: binary, home: home))
        tearDownLater(process, home: home.home)

        let state = await process.start()
        guard case .running = state else {
            XCTFail("the host did not come up: \(state); stderr: \(process.stderrTail)")
            return
        }
        let secret = try XCTUnwrap(process.currentSecret)
        XCTAssertEqual(secret.count, 64)
        XCTAssertFalse(String(describing: process.state).contains(secret))
        XCTAssertFalse(process.stderrTail.contains(secret))
    }

    private func freeLoopbackPort() throws -> Int {
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        guard fd >= 0 else { throw HostTestFailure(message: "socket: \(errno)") }
        defer { close(fd) }
        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = 0
        address.sin_addr.s_addr = inet_addr("127.0.0.1")
        let bound = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        guard bound == 0 else { throw HostTestFailure(message: "bind: \(errno)") }
        var actual = sockaddr_in()
        var length = socklen_t(MemoryLayout<sockaddr_in>.size)
        let named = withUnsafeMutablePointer(to: &actual) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                getsockname(fd, $0, &length)
            }
        }
        guard named == 0 else { throw HostTestFailure(message: "getsockname: \(errno)") }
        return Int(UInt16(bigEndian: actual.sin_port))
    }

    private func holdPort(_ port: Int) throws -> Int32 {
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        guard fd >= 0 else { throw HostTestFailure(message: "socket: \(errno)") }
        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = UInt16(port).bigEndian
        address.sin_addr.s_addr = inet_addr("127.0.0.1")
        let bound = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        guard bound == 0 else {
            close(fd)
            throw HostTestFailure(message: "bind to the held port: \(errno)")
        }
        guard listen(fd, 1) == 0 else {
            close(fd)
            throw HostTestFailure(message: "listen: \(errno)")
        }
        return fd
    }
}
