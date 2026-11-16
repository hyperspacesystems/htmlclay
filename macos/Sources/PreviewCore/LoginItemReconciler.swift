import Foundation

public enum LoginItemStatus: Equatable, Sendable {
    case enabled
    case requiresApproval
    case notRegistered
    case notFound
}

/// The app's own login item. The product uses SMAppService.mainApp; a candidate records calls.
@MainActor
public protocol LoginItemService: AnyObject {
    var status: LoginItemStatus { get }
    func register() throws
    func unregister() throws
}

/// Decision 10: Open at Login moves from 1.x's LaunchAgent to the app's own login item, after the
/// update is judged. The first pass waits `delay` after the first ready host, so a rollback inside
/// that window finds the old LaunchAgent untouched. Later passes follow the snapshot's openAtLogin.
@MainActor
public final class LoginItemReconciler {
    public static let movedKey = "loginItemMovedV2"

    private let service: LoginItemService
    private let legacyPlist: URL
    private let defaults: UserDefaults
    private let delay: TimeInterval

    private var scheduled = false
    private var armed = false
    private var wanted: Bool?
    private var applied: Bool?

    public init(service: LoginItemService, legacyPlist: URL, defaults: UserDefaults,
                delay: TimeInterval = 60) {
        self.service = service
        self.legacyPlist = legacyPlist
        self.defaults = defaults
        self.delay = delay
    }

    /// Call with the snapshot's openAtLogin on every snapshot from a ready host.
    public func update(wanted: Bool) {
        self.wanted = wanted
        if armed {
            guard wanted != applied else { return }
            reconcile(wanted: wanted, fromSwitch: true)
            return
        }
        guard !scheduled else { return }
        scheduled = true
        let wait = delay
        Task { @MainActor [weak self] in
            try? await Task.sleep(nanoseconds: UInt64(max(wait, 0) * 1_000_000_000))
            guard let self else { return }
            self.armed = true
            if let wanted = self.wanted { self.reconcile(wanted: wanted, fromSwitch: false) }
        }
    }

    /// fromSwitch: the person changed the setting, so register even after an earlier move. On its own
    /// the app registers only once, so turning HTML Clay off in System Settings is not overridden.
    func reconcile(wanted: Bool, fromSwitch: Bool) {
        applied = wanted
        guard wanted else {
            try? service.unregister()
            removeLegacy()
            return
        }
        if service.status != .enabled, fromSwitch || !defaults.bool(forKey: Self.movedKey) {
            try? service.register()
        }
        if service.status == .enabled {
            removeLegacy()
            defaults.set(true, forKey: Self.movedKey)
        }
    }

    /// Deletes the 1.x LaunchAgent's file and never runs launchctl bootout: right after a quit
    /// install, the process that agent started at login is this app, and bootout would stop it.
    /// The job has RunAtLoad and no KeepAlive, so with its file gone it never starts again.
    private func removeLegacy() {
        try? FileManager.default.removeItem(at: legacyPlist)
    }
}
