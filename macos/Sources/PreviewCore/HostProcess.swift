import Foundation
import Darwin
import Security

public struct HostReady: Decodable, Equatable, Sendable {
    public let v: Int
    public let shell: String
    public let port: Int
    public let epoch: String
    public let pid: Int
    public let version: String
}

public struct HostFailure: Decodable, Equatable, Sendable {
    public let v: Int
    public let code: String
    public let port: Int?
    public let message: String?
}

public struct HostLaunch: Sendable {
    public var binary: URL
    public var arguments: [String] = ["--shell-stdin"]
    /// Added to the current environment (HOME and HTMLCLAY_CONFIG_BASE for a test or profile home).
    public var environment: [String: String] = [:]
    public var readyTimeout: TimeInterval = 30
    public init(binary: URL, environment: [String: String] = [:]) {
        self.binary = binary
        self.environment = environment
    }
}

public enum HostState: Equatable, Sendable {
    case starting
    case running(HostReady)
    /// A typed FAILED line, or a READY line this app refuses ("bad-ready"). Not retried.
    case failed(HostFailure)
    /// Exited before READY or too many fast exits after it. reason is for the log, not the person.
    case stopped(reason: String)
}

private let sigPipeIgnored: Bool = {
    signal(SIGPIPE, SIG_IGN)
    return true
}()

private final class HostPipeDrain {
    private let handle: FileHandle
    private let descriptor: Int32
    private let limit: Int
    private let keepLast: Bool
    private let lock = NSLock()
    private var buffer = Data()
    private var atEnd = false

    init(_ handle: FileHandle, limit: Int, keepLast: Bool) {
        self.handle = handle
        self.descriptor = handle.fileDescriptor
        self.limit = limit
        self.keepLast = keepLast
        DispatchQueue.global().async { [self] in drain() }
    }

    private func drain() {
        var chunk = [UInt8](repeating: 0, count: 65536)
        while true {
            let count = read(descriptor, &chunk, chunk.count)
            if count > 0 {
                append(chunk, count)
                continue
            }
            if count < 0 && errno == EINTR { continue }
            lock.lock()
            atEnd = true
            lock.unlock()
            return
        }
    }

    private func append(_ chunk: [UInt8], _ count: Int) {
        lock.lock()
        if keepLast {
            buffer.append(contentsOf: chunk[0 ..< count])
            if buffer.count > limit { buffer.removeFirst(buffer.count - limit) }
        } else if buffer.count < limit {
            buffer.append(contentsOf: chunk[0 ..< min(count, limit - buffer.count)])
        }
        lock.unlock()
    }

    var text: String {
        lock.lock()
        defer { lock.unlock() }
        return String(decoding: buffer, as: UTF8.self)
    }

    var finished: Bool {
        lock.lock()
        defer { lock.unlock() }
        return atEnd
    }
}

private final class HostChild {
    let process: Process
    let stdin: FileHandle
    let stdinPipe: Pipe
    let stdoutPipe: Pipe
    let stderrPipe: Pipe
    let stdout: HostPipeDrain
    let stderr: HostPipeDrain
    let secret: String
    var stdinClosed = false

    init(process: Process, stdinPipe: Pipe, stdoutPipe: Pipe, stderrPipe: Pipe,
         stdout: HostPipeDrain, stderr: HostPipeDrain, secret: String) {
        self.process = process
        self.stdin = stdinPipe.fileHandleForWriting
        self.stdinPipe = stdinPipe
        self.stdoutPipe = stdoutPipe
        self.stderrPipe = stderrPipe
        self.stdout = stdout
        self.stderr = stderr
        self.secret = secret
    }
}

private enum HostBootstrap {
    case line(String)
    case exited(Int32)
    case timedOut
    case stopped
}

@MainActor
public final class HostProcess: QuitHostProcess {
    private let launch: HostLaunch
    private let restartDelays: [Double]
    private let fastFailureWindow: TimeInterval
    private let fastFailureLimit: Int

    public private(set) var state: HostState = .starting
    /// Called on every state change, on the main actor.
    public var onState: (@MainActor (HostState) -> Void)?
    /// Called each time a host (first or restarted) is ready, with a fresh client for it.
    public var onReady: (@MainActor (HostReady, LoopbackHostClient) -> Void)?

    private var child: HostChild?
    private var superviseTask: Task<Void, Never>?
    private var stopRequested = false
    private var firstOutcomeDelivered = false
    private var emittedState = false
    private var startWaiter: CheckedContinuation<HostState, Never>?

    public init(launch: HostLaunch, restartDelays: [Double] = [0.5, 1, 2, 4, 8], fastFailureWindow: TimeInterval = 60, fastFailureLimit: Int = 3) {
        self.launch = launch
        self.restartDelays = restartDelays.isEmpty ? [0] : restartDelays
        self.fastFailureWindow = fastFailureWindow
        self.fastFailureLimit = fastFailureLimit
    }

    /// The last 64 KB the child wrote to stderr, for a bug report; never shown as is.
    public var stderrTail: String { child?.stderr.text ?? "" }

    /// The child's process id while running (tests).
    public var pid: Int32? {
        guard let child, child.process.isRunning else { return nil }
        return child.process.processIdentifier
    }

    var currentSecret: String? { child?.secret }

    /// Starts the host and keeps it running; returns once the first READY, FAILED or stop.
    public func start() async -> HostState {
        _ = sigPipeIgnored
        if superviseTask == nil {
            superviseTask = Task { await supervise() }
        }
        if firstOutcomeDelivered { return state }
        return await withCheckedContinuation { continuation in
            startWaiter = continuation
        }
    }

    /// Closes stdin and waits up to timeout for the host to exit, then SIGTERM, then 2 s later SIGKILL.
    /// Returns true when it exited on its own. No restart follows.
    public func stop(timeout: TimeInterval = 10) async -> Bool {
        stopRequested = true
        guard let child else {
            setState(.stopped(reason: "stopped"))
            return true
        }
        closeStdin(child)
        var exited = true
        if child.process.isRunning {
            exited = await waitForExit(child, timeout: timeout)
        }
        if !exited {
            child.process.terminate()
            let terminated = await waitForExit(child, timeout: 2)
            if !terminated {
                kill(child.process.processIdentifier, SIGKILL)
                _ = await waitForExit(child, timeout: 2)
            }
        }
        if state == .starting || isRunning { setState(.stopped(reason: "stopped")) }
        return exited
    }

    public var isRunning: Bool {
        if case .running = state { return true }
        return false
    }

    /// Waits up to timeout for a host that is exiting after an accepted host.quit.
    /// Returns true when it exited. No restart follows.
    public func waitUntilExited(timeout: TimeInterval) async -> Bool {
        stopRequested = true
        guard let child else { return true }
        return await waitForExit(child, timeout: timeout)
    }

    private func supervise() async {
        var attempt = 0
        var unexpectedExits: [Date] = []
        setState(.starting)
        while !stopRequested {
            guard let child = launchChild() else {
                setState(.stopped(reason: "could not start \(launch.binary.path)"))
                return
            }
            switch await awaitBootstrap(child) {
            case .stopped:
                return
            case .timedOut:
                child.process.terminate()
                _ = await waitForExit(child, timeout: 5)
                setState(.stopped(reason: "no line within \(launch.readyTimeout)s"))
                return
            case let .exited(status):
                setState(.stopped(reason: "exited before ready (status \(status))"))
                return
            case let .line(line):
                if line.hasPrefix("FAILED ") {
                    let payload = String(line.dropFirst("FAILED ".count))
                    let failure = (try? JSONDecoder().decode(HostFailure.self, from: Data(payload.utf8)))
                        ?? HostFailure(v: 1, code: "fatal", port: nil, message: nil)
                    setState(.failed(failure))
                    _ = await waitForExit(child, timeout: 10)
                    return
                }
                let payload = String(line.dropFirst("READY ".count))
                let decoded = try? JSONDecoder().decode(HostReady.self, from: Data(payload.utf8))
                guard let ready = decoded, let port = acceptedPort(ready, pid: child.process.processIdentifier) else {
                    child.process.terminate()
                    _ = await waitForExit(child, timeout: 5)
                    setState(.failed(HostFailure(v: 1, code: "bad-ready", port: nil, message: nil)))
                    return
                }
                let began = Date()
                let client = LoopbackHostClient(port: port, secret: child.secret)
                setState(.running(ready))
                onReady?(ready, client)
                _ = await waitForExit(child, timeout: nil)
                if stopRequested { return }
                let now = Date()
                if now.timeIntervalSince(began) >= fastFailureWindow { unexpectedExits.removeAll() }
                unexpectedExits.removeAll { now.timeIntervalSince($0) > fastFailureWindow }
                unexpectedExits.append(now)
                if unexpectedExits.count > fastFailureLimit {
                    setState(.stopped(reason: "exited \(unexpectedExits.count) times in a row"))
                    return
                }
                let delay = restartDelays[min(attempt, restartDelays.count - 1)]
                attempt += 1
                setState(.starting)
                try? await Task.sleep(nanoseconds: UInt64(max(delay, 0) * 1_000_000_000))
                if stopRequested { return }
            }
        }
    }

    private func launchChild() -> HostChild? {
        let stdinPipe = Pipe()
        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()
        let process = Process()
        process.executableURL = launch.binary
        process.arguments = launch.arguments
        var environment = ProcessInfo.processInfo.environment
        for (key, value) in launch.environment { environment[key] = value }
        process.environment = environment
        process.standardInput = stdinPipe
        process.standardOutput = stdoutPipe
        process.standardError = stderrPipe
        let secret = Self.randomSecret()
        do {
            try process.run()
        } catch {
            return nil
        }
        let stdout = HostPipeDrain(stdoutPipe.fileHandleForReading, limit: 65536, keepLast: false)
        let stderr = HostPipeDrain(stderrPipe.fileHandleForReading, limit: 65536, keepLast: true)
        let made = HostChild(process: process, stdinPipe: stdinPipe, stdoutPipe: stdoutPipe, stderrPipe: stderrPipe,
                             stdout: stdout, stderr: stderr, secret: secret)
        try? made.stdin.write(contentsOf: Data("{\"v\":1,\"secret\":\"\(secret)\"}\n".utf8))
        child = made
        return made
    }

    private func awaitBootstrap(_ child: HostChild) async -> HostBootstrap {
        let deadline = Date().addingTimeInterval(launch.readyTimeout)
        while true {
            if let line = Self.bootstrapLine(child.stdout.text) { return .line(line) }
            if stopRequested { return .stopped }
            if !child.process.isRunning {
                let grace = Date().addingTimeInterval(0.5)
                while Date() < grace {
                    if let line = Self.bootstrapLine(child.stdout.text) { return .line(line) }
                    if child.stdout.finished { break }
                    try? await Task.sleep(nanoseconds: 10_000_000)
                }
                if let line = Self.bootstrapLine(child.stdout.text) { return .line(line) }
                return .exited(child.process.terminationStatus)
            }
            if Date() >= deadline { return .timedOut }
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
    }

    private func waitForExit(_ child: HostChild, timeout: TimeInterval?) async -> Bool {
        let deadline = timeout.map { Date().addingTimeInterval($0) }
        while child.process.isRunning {
            if let deadline, Date() >= deadline { return false }
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        return true
    }

    private func closeStdin(_ child: HostChild) {
        guard !child.stdinClosed else { return }
        child.stdinClosed = true
        try? child.stdin.close()
    }

    private func acceptedPort(_ ready: HostReady, pid: Int32) -> Int? {
        guard ready.v == 1,
              let shell = URL(string: ready.shell),
              shell.scheme == "http",
              shell.host == "127.0.0.1",
              let port = shell.port,
              port >= 1, port <= 65535,
              !ready.epoch.isEmpty,
              ready.pid == Int(pid) else { return nil }
        return port
    }

    private static func bootstrapLine(_ text: String) -> String? {
        for line in text.split(separator: "\n", omittingEmptySubsequences: false)
        where line.hasPrefix("READY ") || line.hasPrefix("FAILED ") {
            return String(line)
        }
        return nil
    }

    private static func randomSecret() -> String {
        var bytes = [UInt8](repeating: 0, count: 32)
        let made = bytes.withUnsafeMutableBytes { buffer in
            SecRandomCopyBytes(kSecRandomDefault, 32, buffer.baseAddress!)
        }
        if made != errSecSuccess {
            var generator = SystemRandomNumberGenerator()
            for index in bytes.indices {
                bytes[index] = UInt8.random(in: 0 ... 255, using: &generator)
            }
        }
        return bytes.map { String(format: "%02x", $0) }.joined()
    }

    private func setState(_ new: HostState) {
        guard !emittedState || new != state else { return }
        emittedState = true
        state = new
        if !firstOutcomeDelivered, new != .starting {
            firstOutcomeDelivered = true
            startWaiter?.resume(returning: new)
            startWaiter = nil
        }
        onState?(new)
    }
}
