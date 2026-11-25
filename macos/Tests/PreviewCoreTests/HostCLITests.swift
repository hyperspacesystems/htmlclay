import XCTest
@testable import PreviewCore

final class HostCLITests: XCTestCase {
    private var folder: URL!

    override func setUpWithError() throws {
        folder = FileManager.default.temporaryDirectory.appendingPathComponent("hostcli-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: folder)
    }

    private func stub(_ body: String) throws -> URL {
        let script = folder.appendingPathComponent("htmlclay-host")
        try ("#!/bin/sh\n" + body + "\n").write(to: script, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: script.path)
        return script
    }

    func testEachCommandSendsItsArgumentsAndReturnsTheFirstLine() async throws {
        let host = HostCLI(binary: try stub("echo \"$*\"; echo second line"), environment: [:])
        let confirm = try await host.confirm()
        XCTAssertEqual(confirm, "update-confirm")
        let restart = try await host.prepare(version: "2.0.0", appPID: 42, quit: false, force: false)
        XCTAssertEqual(restart, "update-prepare 2.0.0 42 restart")
        let quit = try await host.prepare(version: "2.0.0", appPID: 42, quit: true, force: true)
        XCTAssertEqual(quit, "update-prepare 2.0.0 42 quit force")
        let abort = try await host.abort(txid: "abc")
        XCTAssertEqual(abort, "update-abort abc")
        let status = try await host.status()
        XCTAssertEqual(status, "update-status")
        let info = try await host.info()
        XCTAssertEqual(info, "update-info")
    }

    func testTheHostEnvironmentReachesTheCommand() async throws {
        let host = HostCLI(binary: try stub("echo \"base=$HTMLCLAY_CONFIG_BASE\""), environment: ["HTMLCLAY_CONFIG_BASE": "/tmp/hc-x/c"])
        let answer = try await host.status()
        XCTAssertEqual(answer, "base=/tmp/hc-x/c")
    }

    func testStartingIsSentAgainUntilTheHostAnswers() async throws {
        let count = folder.appendingPathComponent("count").path
        let body = """
        n=$(cat '\(count)' 2>/dev/null || echo 0); n=$((n+1)); echo $n > '\(count)'
        if [ $n -lt 3 ]; then echo starting; else echo confirmed; fi
        """
        var host = HostCLI(binary: try stub(body), environment: [:])
        host.retryDelay = 0.05
        let answer = try await host.confirm()
        XCTAssertEqual(answer, "confirmed")
        XCTAssertEqual(try String(contentsOfFile: count, encoding: .utf8).trimmingCharacters(in: .whitespacesAndNewlines), "3")
    }

    func testAHostThatStaysStartingIsAnError() async throws {
        var host = HostCLI(binary: try stub("echo starting"), environment: [:])
        host.retryDelay = 0.05
        host.startingLimit = 0.3
        do {
            _ = try await host.confirm()
            XCTFail("a host that never stops starting was accepted")
        } catch let error as HostCommandError {
            XCTAssertEqual(error, .stillStarting("update-confirm"))
        }
    }

    func testAStuckCommandIsStoppedAtItsLimit() async throws {
        var host = HostCLI(binary: try stub("sleep 30"), environment: [:])
        host.commandLimit = 0.5
        let started = Date()
        do {
            _ = try await host.confirm()
            XCTFail("a stuck command returned")
        } catch let error as HostCommandError {
            XCTAssertEqual(error, .timedOut("update-confirm"))
        }
        XCTAssertLessThan(Date().timeIntervalSince(started), 5)
    }

    func testNoAnswerIsAnError() async throws {
        let host = HostCLI(binary: try stub("exit 4"), environment: [:])
        do {
            _ = try await host.confirm()
            XCTFail("an empty answer was accepted")
        } catch let error as HostCommandError {
            XCTAssertEqual(error, .noAnswer("update-confirm", status: 4))
        }
    }
}
