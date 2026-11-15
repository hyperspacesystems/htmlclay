import Foundation

public enum UpdateEvent: Equatable, Sendable {
    case started
    case startFailed(String)
    case ready(String)
    case aborted(String)
    case installFailed(String)
    case checkSkipped(String)
}

/// Sparkle, seen from the app. The real one is SparkleDriver over HCSparkleBridge.
@MainActor
public protocol UpdateDriver: AnyObject {
    func start(events: @escaping @MainActor (UpdateEvent) -> Void, skipReason: @escaping @MainActor (String) -> String?)
    func installNow()
    func checkNow()
}

@MainActor
public protocol UpdatePresenter: QuitPresenter {
    /// An AI edit or a document's program is running. True means restart anyway.
    func askAboutRunningWork() async -> Bool
    /// Restart to update was chosen but no quit began within the wait.
    func updateNotStarted()
    /// The host could not get ready for a restart; HTML Clay keeps running.
    func restartFailed()
}

public enum QuitPlan: Equatable, Sendable {
    case proceed(force: Bool)
    case cancel
}

/// Whether this copy of the app may update itself. Mirrors the host's sparkleGate.
public enum UpdateGate {
    public static func allows(feedURL: String?, publicKey: String?, bundlePath: String, writable: Bool, version: String) -> Bool {
        guard let feedURL, !feedURL.isEmpty, let publicKey, !publicKey.isEmpty else { return false }
        guard !bundlePath.contains("/AppTranslocation/"), writable else { return false }
        return isStrictVersion(version)
    }

    /// True when a is a newer strict X.Y.Z than b.
    public static func isNewer(_ a: String, than b: String) -> Bool {
        guard isStrictVersion(a), isStrictVersion(b) else { return false }
        let x = a.split(separator: ".").compactMap { Int($0) }
        let y = b.split(separator: ".").compactMap { Int($0) }
        return x.lexicographicallyPrecedes(y) == false && x != y
    }

    public static func isStrictVersion(_ version: String) -> Bool {
        let parts = version.split(separator: ".", omittingEmptySubsequences: false)
        return parts.count == 3 && parts.allSatisfy { !$0.isEmpty && $0.allSatisfy(\.isASCII) && $0.allSatisfy(\.isNumber) }
    }
}

/// Runs updates for the app: starts Sparkle when allowed, offers the restart row,
/// and before any quit asks the host to get ready (C3) so every acknowledged save
/// lands before Sparkle replaces the bundle.
@MainActor
public final class UpdateCoordinator {
    private let driver: UpdateDriver?
    private let host: HostCommands
    private let presenter: UpdatePresenter
    private let appPID: Int32
    private let notStartedAfter: TimeInterval
    private let version: String
    private let now: () -> Date

    public private(set) var prepared: String?
    public private(set) var started = false
    /// For the candidate's state file: "" until decided, "on", or "off <reason>".
    public private(set) var gate = ""
    /// For the candidate's state file: "" until Sparkle answers, "started", or "failed <reason>".
    public private(set) var sparkle = ""
    public var onChange: (() -> Void)?

    private var relaunch = false
    private var txid: String?
    private var quitAsked = false
    private var failedVersion: String?
    private var failedReason = ""
    private var offReason: String?
    private var startFailed: String?
    private var declined: String?
    private var requested: String?
    private var requestedAt = Date.distantPast

    /// How long an update-now request waits for Sparkle to prepare the version. The canary resends every 30 s.
    public static let updateNowWindow: TimeInterval = 60

    public init(driver: UpdateDriver?, host: HostCommands, presenter: UpdatePresenter, appPID: Int32,
                version: String = "", notStartedAfter: TimeInterval = 40, now: @escaping () -> Date = Date.init) {
        self.driver = driver
        self.host = host
        self.presenter = presenter
        self.appPID = appPID
        self.version = version
        self.notStartedAfter = notStartedAfter
        self.now = now
    }

    /// Call on every host ready. Starts Sparkle once, when the gate allows it and the host does not say off.
    public func start(allowed: Bool) async {
        guard !started, offReason == nil else { return }
        guard allowed else {
            turnOff("this copy of HTML Clay cannot update itself (no feed or key, not X.Y.Z, translocated or read-only)")
            return
        }
        guard let driver else {
            turnOff("Sparkle is not running")
            return
        }
        guard let status = try? await host.status() else { return }
        if status.hasPrefix("off") {
            turnOff(String(status.dropFirst("off".count)).trimmingCharacters(in: .whitespaces))
            return
        }
        gate = "on"
        onChange?()
        if status.hasPrefix("failed ") {
            let fields = status.split(separator: " ", maxSplits: 2).map(String.init)
            failedVersion = fields.count > 1 ? fields[1] : nil
            failedReason = fields.count > 2 ? fields[2] : ""
        }
        started = true
        driver.start(events: { [weak self] event in self?.handle(event) },
                     skipReason: { [weak self] version in self?.skipReason(version) })
    }

    private func turnOff(_ reason: String) {
        offReason = reason
        gate = "off \(reason)"
        onChange?()
    }

    /// The row's action: Sparkle sends the quit, and beforeQuit prepares a restart.
    public func restartToUpdate() {
        guard prepared != nil, let driver else { return }
        relaunch = true
        driver.installNow()
        let wait = notStartedAfter
        Task { @MainActor [weak self] in
            try? await Task.sleep(nanoseconds: UInt64(max(wait, 0) * 1_000_000_000))
            guard let self, !self.quitAsked, self.relaunch else { return }
            self.relaunch = false
            self.presenter.updateNotStarted()
        }
    }

    public func checkNow() { driver?.checkNow() }

    /// The answer to `htmlclay update-now --version target` (C4), ported branch by branch from
    /// `(a *app) updateNow` in cmd/htmlclay/update_now.go.
    public func updateNow(_ target: String) -> String {
        guard UpdateGate.isStrictVersion(target) else { return "refused \"\(target)\" is not X.Y.Z" }
        if target == version { return "already \(version)" }
        guard UpdateGate.isNewer(target, than: version) else {
            return "refused v\(target) is not newer than the running v\(version)"
        }
        if let offReason { return "refused self-update is off: \(offReason)" }
        guard started else { return HostCLI.starting }
        if let reason = skipReason(target) { return "refused \(reason)" }
        if let startFailed { return "refused Sparkle could not start: \(startFailed)" }
        if declined == target { return "failed the restart to v\(target) was cancelled on screen" }
        if prepared == target {
            if !relaunch { restartToUpdate() }
            return "accepted"
        }
        if let prepared { return "failed Sparkle holds v\(prepared) prepared, which installs at the next quit" }
        requested = target
        requestedAt = now()
        driver?.checkNow()
        return "accepted"
    }

    /// The whole quit: prepare, then the host's quit through quitter (which answers only after the host
    /// exited), then abort a prepared update if the quit was cancelled. True means reply yes to AppKit.
    public func terminate(quitter: (Bool) async -> Bool) async -> Bool {
        switch await beforeQuit() {
        case .cancel:
            return false
        case let .proceed(force):
            let ok = await quitter(force)
            if !ok {
                relaunch = false
                await abortPrepared()
            }
            return ok
        }
    }

    func beforeQuit() async -> QuitPlan {
        quitAsked = true
        guard let version = prepared else { return .proceed(force: false) }
        var force = false
        while true {
            let answer: String
            do {
                answer = try await host.prepare(version: version, appPID: appPID, quit: !relaunch, force: force)
            } catch {
                answer = "failed \(error)"
            }
            if answer.hasPrefix("prepared ") {
                let id = String(answer.dropFirst("prepared ".count))
                txid = id == "-" ? nil : id
                return .proceed(force: true)
            }
            if answer.hasPrefix("refused unsaved ") {
                let json = Data(answer.dropFirst("refused unsaved ".count).utf8)
                let lines = (try? JSONDecoder().decode([String].self, from: json)) ?? [Strings.Quit.noAnswer]
                switch await presenter.askAboutUnsaved(lines) {
                case .retry: continue
                case .quitAnyway: force = true; continue
                case .cancel: return cancel()
                }
            }
            if answer == "refused running-work" {
                if await presenter.askAboutRunningWork() { force = true; continue }
                return cancel()
            }
            if relaunch {
                presenter.restartFailed()
                return cancel()
            }
            return .proceed(force: false)
        }
    }

    private func cancel() -> QuitPlan {
        if relaunch { declined = prepared }
        relaunch = false
        return .cancel
    }

    private func abortPrepared() async {
        guard let id = txid else { return }
        txid = nil
        _ = try? await host.abort(txid: id)
    }

    private func handle(_ event: UpdateEvent) {
        switch event {
        case let .ready(version):
            prepared = version
            onChange?()
            if requested == version {
                requested = nil
                if now().timeIntervalSince(requestedAt) <= Self.updateNowWindow { restartToUpdate() }
            }
        case let .startFailed(reason):
            startFailed = reason
            sparkle = "failed \(reason)"
            onChange?()
        case .started:
            sparkle = "started"
            onChange?()
        case .aborted, .installFailed:
            relaunch = false
            Task { @MainActor [weak self] in await self?.abortPrepared() }
        case .checkSkipped:
            break
        }
    }

    private func skipReason(_ version: String) -> String? {
        guard version == failedVersion else { return nil }
        return failedReason.isEmpty ? "an earlier update to \(version) did not start" : failedReason
    }
}
