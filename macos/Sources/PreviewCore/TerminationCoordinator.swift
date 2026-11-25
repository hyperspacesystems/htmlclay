import Foundation

public enum QuitChoice: Equatable, Sendable { case retry, quitAnyway, cancel }

@MainActor
public protocol QuitPresenter: AnyObject {
    /// Shows the unsaved-edits dialog and answers the person's choice.
    func askAboutUnsaved(_ lines: [String]) async -> QuitChoice
}

@MainActor
public protocol QuitHostProcess: AnyObject {
    var isRunning: Bool { get }
    /// The host is exiting on its own after an accepted host.quit: wait for it.
    func waitUntilExited(timeout: TimeInterval) async -> Bool
    /// Close stdin, then terminate, then kill (HostProcess.stop).
    func stop(timeout: TimeInterval) async -> Bool
}

/// Owns Quit from every entry: ask the host, name what it could not save, and only
/// answer true once the host has exited or was never running.
@MainActor
public final class TerminationCoordinator {
    private let host: HostClient
    private let process: QuitHostProcess
    private let presenter: QuitPresenter
    private let replyBound: TimeInterval
    private let exitBound: TimeInterval
    private var inFlight = false

    public init(host: HostClient, process: QuitHostProcess, presenter: QuitPresenter,
                replyBound: TimeInterval = 15, exitBound: TimeInterval = 10) {
        self.host = host
        self.process = process
        self.presenter = presenter
        self.replyBound = replyBound
        self.exitBound = exitBound
    }

    /// True when the app may terminate: the host has exited (or was not running). False on Cancel.
    /// A second call while one is running returns false at once. force sends the first ask forced,
    /// for an update the host has already prepared.
    public func quit(force: Bool = false) async -> Bool {
        guard !inFlight else { return false }
        inFlight = true
        defer { inFlight = false }
        guard process.isRunning else { return true }
        var forceNext = force
        while true {
            let reply = await askHost(force: forceNext)
            forceNext = false
            switch reply {
            case .done:
                await waitForExit()
                return true
            case .silent:
                if let answer = await askPerson([Strings.Quit.noAnswer]) { return answer }
            case let .refused(refusal):
                if let answer = await askPerson(lines(for: refusal)) { return answer }
            }
        }
    }

    /// Nil means Retry: send the quit again.
    private func askPerson(_ lines: [String]) async -> Bool? {
        switch await presenter.askAboutUnsaved(lines) {
        case .cancel:
            return false
        case .retry:
            return nil
        case .quitAnyway:
            if case .done = await askHost(force: true) {
                await waitForExit()
            } else {
                _ = await process.stop(timeout: exitBound)
            }
            return true
        }
    }

    private func lines(for refusal: HostRefusal) -> [String] {
        if refusal.code == "quit-unsaved" {
            return refusal.documents.isEmpty ? [refusal.text] : refusal.documents
        }
        if refusal.code == "unreachable" || refusal.code == "interrupted" || refusal.code == "host-error" {
            return [Strings.Quit.noAnswer]
        }
        return [refusal.text]
    }

    private func waitForExit() async {
        if await process.waitUntilExited(timeout: exitBound) { return }
        _ = await process.stop(timeout: 2)
    }

    /// The host's answer, or silent when it did not answer within replyBound.
    private func askHost(force: Bool) async -> HostReply {
        let race = ReplyRace()
        let host = self.host
        let bound = replyBound
        Task { @MainActor in
            let outcome = await host.perform(.quit(force: force))
            switch outcome {
            case .done: await race.finish(.done)
            case let .refused(refusal): await race.finish(.refused(refusal))
            }
        }
        Task { @MainActor in
            try? await Task.sleep(nanoseconds: UInt64(max(bound, 0) * 1_000_000_000))
            await race.finish(.silent)
        }
        return await race.answer()
    }
}

private enum HostReply {
    case done
    case refused(HostRefusal)
    case silent
}

@MainActor
private final class ReplyRace {
    private var waiting: CheckedContinuation<HostReply, Never>?
    private var answered: HostReply?

    func answer() async -> HostReply {
        if let answered { return answered }
        return await withCheckedContinuation { continuation in
            waiting = continuation
        }
    }

    func finish(_ reply: HostReply) {
        guard answered == nil else { return }
        answered = reply
        waiting?.resume(returning: reply)
        waiting = nil
    }
}
