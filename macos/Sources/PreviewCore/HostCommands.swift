import Foundation
import Darwin

public enum HostCommandError: Error, Equatable, Sendable {
    case timedOut(String)
    case noAnswer(String, status: Int32)
    case stillStarting(String)
}

public protocol HostCommands: Sendable {
    func info() async throws -> String
    func status() async throws -> String
    func confirm() async throws -> String
    func prepare(version: String, appPID: Int32, quit: Bool, force: Bool) async throws -> String
    func abort(txid: String) async throws -> String
}

public struct HostCLI: HostCommands {
    public static let starting = "starting"

    public var binary: URL
    public var environment: [String: String]
    public var commandLimit: TimeInterval = 10
    public var prepareLimit: TimeInterval = 45
    public var startingLimit: TimeInterval = 5
    public var retryDelay: TimeInterval = 0.2

    public init(binary: URL, environment: [String: String]) {
        self.binary = binary
        self.environment = environment
    }

    public func info() async throws -> String { try await run(["update-info"], limit: commandLimit) }
    public func status() async throws -> String { try await run(["update-status"], limit: commandLimit) }
    public func confirm() async throws -> String { try await run(["update-confirm"], limit: commandLimit) }

    public func prepare(version: String, appPID: Int32, quit: Bool, force: Bool) async throws -> String {
        var arguments = ["update-prepare", version, String(appPID), quit ? "quit" : "restart"]
        if force { arguments.append("force") }
        return try await run(arguments, limit: prepareLimit)
    }

    public func abort(txid: String) async throws -> String { try await run(["update-abort", txid], limit: commandLimit) }

    public func run(_ arguments: [String], limit: TimeInterval) async throws -> String {
        let deadline = Date().addingTimeInterval(startingLimit)
        while true {
            let line = try await once(arguments, limit: limit)
            if line != Self.starting { return line }
            if Date() >= deadline { throw HostCommandError.stillStarting(arguments.joined(separator: " ")) }
            try await Task.sleep(nanoseconds: UInt64(retryDelay * 1_000_000_000))
        }
    }

    private func once(_ arguments: [String], limit: TimeInterval) async throws -> String {
        let binary = binary
        var merged = ProcessInfo.processInfo.environment
        for (key, value) in environment { merged[key] = value }
        let environment = merged
        return try await Task.detached {
            try Self.blocking(binary: binary, arguments: arguments, environment: environment, limit: limit)
        }.value
    }

    private static func blocking(binary: URL, arguments: [String], environment: [String: String], limit: TimeInterval) throws -> String {
        let name = arguments.joined(separator: " ")
        let process = Process()
        process.executableURL = binary
        process.arguments = arguments
        process.environment = environment
        let output = Pipe()
        process.standardOutput = output
        process.standardError = FileHandle.nullDevice
        process.standardInput = FileHandle.nullDevice
        let finished = DispatchSemaphore(value: 0)
        process.terminationHandler = { _ in finished.signal() }
        try process.run()
        if finished.wait(timeout: .now() + limit) == .timedOut {
            process.terminate()
            if finished.wait(timeout: .now() + 1) == .timedOut {
                kill(process.processIdentifier, SIGKILL)
                _ = finished.wait(timeout: .now() + 1)
            }
            throw HostCommandError.timedOut(name)
        }
        let data = output.fileHandleForReading.readDataToEndOfFile()
        let first = String(decoding: data, as: UTF8.self)
            .split(separator: "\n", omittingEmptySubsequences: true)
            .first
            .map { $0.trimmingCharacters(in: .whitespaces) } ?? ""
        guard !first.isEmpty else { throw HostCommandError.noAnswer(name, status: process.terminationStatus) }
        return first
    }
}
