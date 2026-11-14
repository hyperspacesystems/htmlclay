import XCTest
@testable import PreviewCore
import Darwin

private struct HostReady: Decodable {
    var v: Int
    var shell: String
    var port: Int
    var epoch: String
    var pid: Int
    var version: String
}

private enum LiveHostFailure: Error, CustomStringConvertible {
    case start(String)

    var description: String {
        switch self {
        case let .start(message): return message
        }
    }
}

private final class PipeReader {
    private let handle: FileHandle
    private let lock = NSLock()
    private var buffer = Data()
    private var atEnd = false

    init(_ handle: FileHandle) {
        self.handle = handle
        DispatchQueue.global().async { [self] in drain() }
    }

    private func drain() {
        let descriptor = handle.fileDescriptor
        var chunk = [UInt8](repeating: 0, count: 65536)
        while true {
            let count = read(descriptor, &chunk, chunk.count)
            guard count > 0 else {
                lock.lock()
                atEnd = true
                lock.unlock()
                return
            }
            lock.lock()
            buffer.append(contentsOf: chunk[0 ..< count])
            lock.unlock()
        }
    }

    var text: String {
        lock.lock()
        defer { lock.unlock() }
        return String(decoding: buffer, as: UTF8.self)
    }

    var lines: [String] {
        text.split(separator: "\n", omittingEmptySubsequences: false).map(String.init)
    }

    var finished: Bool {
        lock.lock()
        defer { lock.unlock() }
        return atEnd
    }
}

@MainActor
private final class LiveHost {
    let home: URL
    let base: URL
    let ready: HostReady
    let secret: String
    let client: LoopbackHostClient
    let shellPort: Int

    private let process: Process
    private let stdinPipe: Pipe
    private let stderrReader: PipeReader
    private var stdinClosed = false

    private static let requireVariable = "HTMLCLAY_REQUIRE_LIVE_HOST"

    static var binary: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent(".build/host-for-tests/htmlclay")
    }

    static func start() throws -> LiveHost {
        let binary = Self.binary
        guard FileManager.default.isExecutableFile(atPath: binary.path) else {
            if ProcessInfo.processInfo.environment[Self.requireVariable] == "1" {
                throw LiveHostFailure.start("the live host is missing: run scripts/build-host-for-tests.sh")
            }
            throw XCTSkip("run scripts/build-host-for-tests.sh to build \(binary.path)")
        }
        return try LiveHost(binary: binary)
    }

    private init(binary: URL) throws {
        let homePath = try Self.makeHome()
        let basePath = homePath + "/c"
        let configDir = basePath + "/htmlclay"
        try FileManager.default.createDirectory(atPath: configDir, withIntermediateDirectories: true)
        let port = try Self.freeLoopbackPort()
        let config = "{\"startOnLogin\": false, \"port\": \(port), \"updates\": \"off\"}\n"
        try Data(config.utf8).write(to: URL(fileURLWithPath: configDir + "/config.json"))
        let secret = Self.randomSecret()

        let process = Process()
        let stdinPipe = Pipe()
        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()
        process.executableURL = binary
        process.arguments = ["--shell-stdin"]
        var environment = ProcessInfo.processInfo.environment
        environment["HOME"] = homePath
        environment["HTMLCLAY_CONFIG_BASE"] = basePath
        process.environment = environment
        process.standardInput = stdinPipe
        process.standardOutput = stdoutPipe
        process.standardError = stderrPipe

        do {
            try process.run()
        } catch {
            try? FileManager.default.removeItem(atPath: homePath)
            throw LiveHostFailure.start("could not start \(binary.path): \(error)")
        }

        let stdoutReader = PipeReader(stdoutPipe.fileHandleForReading)
        let stderrReader = PipeReader(stderrPipe.fileHandleForReading)
        do {
            try stdinPipe.fileHandleForWriting.write(contentsOf: Data("{\"v\":1,\"secret\":\"\(secret)\"}\n".utf8))
        } catch {
            process.terminate()
            try? FileManager.default.removeItem(atPath: homePath)
            throw LiveHostFailure.start("could not write the bootstrap line: \(error)")
        }

        guard let ready = Self.readReady(stdout: stdoutReader, timeout: 30) else {
            let errors = stderrReader.text
            try? stdinPipe.fileHandleForWriting.close()
            Self.wait(until: { !process.isRunning }, timeout: 5)
            if process.isRunning { process.terminate() }
            try? FileManager.default.removeItem(atPath: homePath)
            throw LiveHostFailure.start("no READY line within 30s; stderr:\n\(errors)")
        }
        guard ready.v == 1, let shellURL = URL(string: ready.shell), shellURL.host == "127.0.0.1",
              let shellPort = shellURL.port else {
            process.terminate()
            try? FileManager.default.removeItem(atPath: homePath)
            throw LiveHostFailure.start("READY = \(ready.shell), wanted v1 with a shell on 127.0.0.1")
        }

        self.home = URL(fileURLWithPath: homePath)
        self.base = URL(fileURLWithPath: basePath)
        self.ready = ready
        self.secret = secret
        self.shellPort = shellPort
        self.client = LoopbackHostClient(port: shellPort, secret: secret)
        self.process = process
        self.stdinPipe = stdinPipe
        self.stderrReader = stderrReader
    }

    var stderr: String { stderrReader.text }

    func closeStdin() {
        guard !stdinClosed else { return }
        stdinClosed = true
        try? stdinPipe.fileHandleForWriting.close()
    }

    func waitForExit(timeout: TimeInterval) async -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if !process.isRunning { return true }
            try? await Task.sleep(nanoseconds: 50_000_000)
        }
        return !process.isRunning
    }

    func stop() {
        closeStdin()
        if !Self.wait(until: { !self.process.isRunning }, timeout: 10) {
            process.terminate()
            _ = Self.wait(until: { !self.process.isRunning }, timeout: 5)
        }
        try? FileManager.default.removeItem(at: home)
    }

    private static func wait(until check: () -> Bool, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if check() { return true }
            Thread.sleep(forTimeInterval: 0.05)
        }
        return check()
    }

    private static func readReady(stdout: PipeReader, timeout: TimeInterval) -> HostReady? {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            for line in stdout.lines where line.hasPrefix("READY ") {
                let payload = String(line.dropFirst("READY ".count))
                return try? JSONDecoder().decode(HostReady.self, from: Data(payload.utf8))
            }
            if stdout.finished { return nil }
            Thread.sleep(forTimeInterval: 0.02)
        }
        return nil
    }

    private static func makeHome() throws -> String {
        var template = Array("/tmp/hc-XXXXXXXX".utf8CString)
        let made = template.withUnsafeMutableBufferPointer { buffer -> String? in
            guard let path = mkdtemp(buffer.baseAddress!) else { return nil }
            return String(cString: path)
        }
        guard let made else { throw LiveHostFailure.start("mkdtemp failed: \(errno)") }
        guard let resolved = made.withCString({ realpath($0, nil) }) else {
            try? FileManager.default.removeItem(atPath: made)
            throw LiveHostFailure.start("realpath failed for \(made): \(errno)")
        }
        defer { free(resolved) }
        return String(cString: resolved)
    }

    private static func randomSecret() -> String {
        var generator = SystemRandomNumberGenerator()
        return (0 ..< 32).map { _ in String(format: "%02x", UInt8.random(in: 0 ... 255, using: &generator)) }.joined()
    }

    private static func freeLoopbackPort() throws -> Int {
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        guard fd >= 0 else { throw LiveHostFailure.start("socket: \(errno)") }
        defer { close(fd) }
        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = 0
        address.sin_addr.s_addr = inet_addr("127.0.0.1")
        let bound = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        guard bound == 0 else { throw LiveHostFailure.start("bind: \(errno)") }
        var actual = sockaddr_in()
        var length = socklen_t(MemoryLayout<sockaddr_in>.size)
        let named = withUnsafeMutablePointer(to: &actual) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                getsockname(fd, $0, &length)
            }
        }
        guard named == 0 else { throw LiveHostFailure.start("getsockname: \(errno)") }
        return Int(UInt16(bigEndian: actual.sin_port))
    }
}

@MainActor
final class LiveHostTests: XCTestCase {
    private func makeFolder(_ url: URL) throws {
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        try Data("<!doctype html><title>\(url.lastPathComponent)</title><p>hello".utf8)
            .write(to: url.appendingPathComponent("index.html"))
    }

    private func waitUntil(_ message: String, timeout: TimeInterval = 5, _ check: () -> Bool) async {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if check() { return }
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertTrue(check(), message)
    }

    private func done(_ outcome: HostOutcome, _ message: String) throws -> HostSnapshot {
        guard case let .done(snapshot) = outcome else {
            XCTFail(message)
            throw LiveHostFailure.start(message)
        }
        return snapshot
    }

    private struct FileState: Equatable {
        var bytes: Data
        var modified: Date?
    }

    private func fileState(_ url: URL) throws -> FileState? {
        guard FileManager.default.fileExists(atPath: url.path) else { return nil }
        let attributes = try FileManager.default.attributesOfItem(atPath: url.path)
        return FileState(bytes: try Data(contentsOf: url), modified: attributes[.modificationDate] as? Date)
    }

    func testLoadShowsTheRealHost() async throws {
        let host = try LiveHost.start()
        defer { host.stop() }

        let snapshot = try await host.client.load()
        XCTAssertEqual(snapshot.epoch, host.ready.epoch)
        XCTAssertEqual(snapshot.host.can["folder.trust"], true)
        XCTAssertEqual(snapshot.folders, [])
    }

    func testTrustServeAndStopThroughTheStream() async throws {
        let host = try LiveHost.start()
        defer { host.stop() }
        let notes = host.home.appendingPathComponent("notes")
        try makeFolder(notes)

        let witness = LoopbackHostClient(port: host.shellPort, secret: host.secret)
        var seen: [HostSnapshot] = []
        let subscription = host.client.subscribe { seen.append($0) }
        defer { subscription.cancel() }

        let trustedSnapshot = try done(await witness.perform(.trustFolder(name: "notes", path: notes.path)),
                                       "trusting notes was refused")
        let trusted = try XCTUnwrap(trustedSnapshot.folders.first { $0.fullPath == notes.path })
        XCTAssertEqual(trusted.fullPath, notes.path)

        let servedSnapshot = try done(await witness.perform(.serveFolder(path: notes.path, name: "notes")),
                                      "serving notes was refused")
        let served = try XCTUnwrap(servedSnapshot.folders.first { $0.fullPath == notes.path })
        XCTAssertTrue(served.served)
        XCTAssertTrue(served.address.contains("notes.htmlclay.localhost"))

        await waitUntil("the live stream never showed notes served") {
            seen.contains { $0.folders.first { $0.fullPath == notes.path }?.served == true }
        }

        let stoppedSnapshot = try done(await witness.perform(.setServed(folderID: served.id, on: false)),
                                       "stopping notes was refused")
        XCTAssertEqual(stoppedSnapshot.folders.first { $0.id == served.id }?.served, false)

        let revisions = seen.map(\.rev)
        XCTAssertEqual(revisions, revisions.sorted())
    }

    func testRefusalComesBackTyped() async throws {
        let host = try LiveHost.start()
        defer { host.stop() }
        let notes = host.home.appendingPathComponent("notes")
        let blog = host.home.appendingPathComponent("blog")
        try makeFolder(notes)
        try makeFolder(blog)

        _ = try done(await host.client.perform(.serveFolder(path: notes.path, name: "notes")),
                     "serving notes was refused")

        guard case let .refused(refusal) = await host.client.perform(.serveFolder(path: blog.path, name: "notes")) else {
            XCTFail("expected a refusal for the taken name")
            return
        }
        XCTAssertEqual(refusal.code, "name-taken")
        XCTAssertFalse(refusal.text.isEmpty)
    }

    func testCheckAnswersTheTrustSheet() async throws {
        let host = try LiveHost.start()
        defer { host.stop() }
        let notes = host.home.appendingPathComponent("notes")
        try makeFolder(notes)

        guard case let .failure(refusal) = await host.client.check(.trustFolder(name: "", path: host.home.path)) else {
            XCTFail("expected the home folder to be refused")
            return
        }
        XCTAssertEqual(refusal.code, "cannot-trust")

        guard case let .success(answer) = await host.client.check(.trustFolder(name: "notes", path: notes.path)) else {
            XCTFail("expected notes to be checkable")
            return
        }
        XCTAssertEqual(answer.path, notes.path)
    }

    func testWrongSecretIsRefused() async throws {
        let host = try LiveHost.start()
        defer { host.stop() }
        let stranger = LoopbackHostClient(port: host.shellPort, secret: String(repeating: "0", count: 64))

        guard case let .refused(refusal) = await stranger.perform(.setAIEditing(false)) else {
            XCTFail("expected the wrong secret to be refused")
            return
        }
        XCTAssertEqual(refusal.code, "host-error")

        _ = try done(await host.client.perform(.setAIEditing(false)),
                     "the right secret stopped working after the wrong one")
    }

    func testClosingStdinEndsTheStream() async throws {
        let host = try LiveHost.start()
        defer { host.stop() }

        var endings: [String] = []
        var seen = 0
        host.client.onEnded = { endings.append($0) }
        let subscription = host.client.subscribe { _ in seen += 1 }
        defer { subscription.cancel() }

        await waitUntil("the stream never delivered a snapshot after load") { seen > 0 }
        host.closeStdin()

        await waitUntil("onEnded never heard quit", timeout: 10) { !endings.isEmpty }
        XCTAssertEqual(endings.first, "quit")
        let exited = await host.waitForExit(timeout: 10)
        XCTAssertTrue(exited, "the host did not exit after stdin closed")
    }

    func testNothingWrittenOutsideTheTestHome() async throws {
        let real = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support/htmlclay/config.json")
        let before = try fileState(real)

        let host = try LiveHost.start()
        defer { host.stop() }
        let notes = host.home.appendingPathComponent("notes")
        try makeFolder(notes)

        _ = try done(await host.client.perform(.trustFolder(name: "notes", path: notes.path)),
                     "trusting notes was refused")
        let servedSnapshot = try done(await host.client.perform(.serveFolder(path: notes.path, name: "notes")),
                                      "serving notes was refused")
        let served = try XCTUnwrap(servedSnapshot.folders.first { $0.fullPath == notes.path })
        _ = try done(await host.client.perform(.setServed(folderID: served.id, on: false)),
                     "stopping notes was refused")

        let after = try fileState(real)
        XCTAssertEqual(before, after)
    }
}
