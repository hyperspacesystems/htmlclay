import XCTest
@testable import PreviewCore
import Darwin

enum LiveHostFailure: Error, CustomStringConvertible {
    case start(String)

    var description: String {
        switch self {
        case let .start(message): return message
        }
    }
}

@MainActor
final class LiveHost {
    let home: URL
    let base: URL
    let ready: HostReady
    let client: LoopbackHostClient
    let shellPort: Int
    let process: HostProcess

    private static let requireVariable = "HTMLCLAY_REQUIRE_LIVE_HOST"
    private static var running: [LiveHost] = []

    static var binary: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent(".build/host-for-tests/htmlclay")
    }

    static var stubBinary: URL {
        binary.deletingLastPathComponent().appendingPathComponent("stubservice")
    }

    static func start(environment extra: [String: String] = [:]) async throws -> LiveHost {
        let binary = Self.binary
        guard FileManager.default.isExecutableFile(atPath: binary.path) else {
            if ProcessInfo.processInfo.environment[Self.requireVariable] == "1" {
                throw LiveHostFailure.start("the live host is missing: run scripts/build-host-for-tests.sh")
            }
            throw XCTSkip("run scripts/build-host-for-tests.sh to build \(binary.path)")
        }

        let homePath = try Self.makeHome()
        let basePath = homePath + "/c"
        let configDir = basePath + "/htmlclay"
        try FileManager.default.createDirectory(atPath: configDir, withIntermediateDirectories: true)
        let port = try Self.freeLoopbackPort()
        let config = "{\"startOnLogin\": false, \"port\": \(port), \"updates\": \"off\"}\n"
        try Data(config.utf8).write(to: URL(fileURLWithPath: configDir + "/config.json"))

        var environment = [
            "HOME": homePath,
            "HTMLCLAY_CONFIG_BASE": basePath,
            "HTMLCLAY_OPEN_LOG": homePath + "/open.log",
        ]
        for (key, value) in extra { environment[key] = value }
        let process = HostProcess(launch: HostLaunch(binary: binary, environment: environment))
        var handed: (ready: HostReady, client: LoopbackHostClient)?
        process.onReady = { ready, client in handed = (ready, client) }

        let state = await process.start()
        guard case let .running(ready) = state, let handed else {
            _ = await process.stop(timeout: 5)
            try? FileManager.default.removeItem(atPath: homePath)
            throw LiveHostFailure.start("no READY line from \(binary.path): \(state); stderr:\n\(process.stderrTail)")
        }
        guard let shellPort = URL(string: ready.shell)?.port else {
            _ = await process.stop(timeout: 5)
            try? FileManager.default.removeItem(atPath: homePath)
            throw LiveHostFailure.start("READY = \(ready.shell), wanted a shell on 127.0.0.1")
        }

        let host = LiveHost(home: URL(fileURLWithPath: homePath),
                            base: URL(fileURLWithPath: basePath),
                            ready: ready,
                            client: handed.client,
                            shellPort: shellPort,
                            process: process)
        running.append(host)
        return host
    }

    private init(home: URL, base: URL, ready: HostReady, client: LoopbackHostClient, shellPort: Int, process: HostProcess) {
        self.home = home
        self.base = base
        self.ready = ready
        self.client = client
        self.shellPort = shellPort
        self.process = process
    }

    var secret: String { process.currentSecret ?? "" }

    @discardableResult
    func stop() async -> Bool {
        let exited = await process.stop(timeout: 10)
        try? FileManager.default.removeItem(at: home)
        return exited
    }

    static func stopAll() async {
        let hosts = running
        running = []
        for host in hosts { await host.stop() }
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
