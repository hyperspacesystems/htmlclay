import Foundation

/// The dot kinds a status row offers: green for good, amber for in between,
/// red for stopped, blue for working, muted for off.
public enum StatusDotKind: CaseIterable {
    case green, amber, blue, red, muted
}

/// The settings window's decisions, away from the views: sample data shows every
/// pane and control, and the real host shows the ones it says it can run.
public enum SettingsControls {
    private static let programActions = [
        HostActionType.programAdd,
        HostActionType.programSetAllTrusted,
        HostActionType.programRemoveAccess,
    ]

    private static let syncActions = [
        HostActionType.syncConnect,
        HostActionType.syncDisconnect,
    ]

    /// Open at Login moves to the Mac app in milestone 7; until then the host the
    /// Mac app started refuses it.
    public static func showsOpenAtLogin(isSample: Bool) -> Bool { isSample }

    /// The account row's Change link opens hyperclay.com, which has no host action.
    public static func showsAccountChange(isSample: Bool) -> Bool { isSample }

    /// The automatic-update switch, What's New, and the restart row: the host the
    /// Mac app starts never runs its updater.
    public static func showsUpdateControls(can: Bool) -> Bool {
        PanelControls.showsUpdate(can: can)
    }

    /// The logs, the file-type removal and Show in Finder have no host action yet.
    public static func showsTroubleshooting(isSample: Bool) -> Bool { isSample }

    /// The port every address on this Mac uses: sample data shows the designed one.
    public static func port(isSample: Bool, port: Int) -> String {
        isSample ? Strings.Advanced.portNumber : String(port)
    }

    /// The live service row's dot and text, from the host's `host.service`.
    public static func serviceStatus(_ service: String) -> (dot: StatusDotKind, text: String) {
        switch service {
        case "running": return (.green, Strings.Advanced.running)
        case "starting", "restarting": return (.amber, Strings.Advanced.starting)
        case "failed": return (.red, Strings.Advanced.stopped)
        default: return (.muted, Strings.Advanced.off)
        }
    }

    /// The dashboard address without its scheme and trailing slash.
    public static func dashboardAddress(_ url: String) -> String {
        var address = url
        for prefix in ["http://", "https://"] where address.hasPrefix(prefix) {
            address.removeFirst(prefix.count)
            break
        }
        while address.hasSuffix("/") { address.removeLast() }
        return address
    }

    /// The storage totals: the host does not send them yet, so only sample data shows them.
    public static func showsStorageTotals(isSample: Bool) -> Bool { isSample }

    /// Restarting the live service.
    public static func showsServiceRestart(can: Bool) -> Bool { can }

    /// Whether the sidebar lists a pane: the ones whose actions the host cannot run
    /// at all are hidden, and the ones that only display settings always show.
    public static func showsPane(_ pane: SettingsPane, in snapshot: HostSnapshot) -> Bool {
        switch pane {
        case .programs: return snapshot.isSample || any(programActions, snapshot)
        case .sync: return snapshot.isSample || any(syncActions, snapshot)
        default: return true
        }
    }

    /// The pane the settings window opens on: a route into a hidden pane shows General.
    public static func visibleRoute(_ route: SettingsRoute, in snapshot: HostSnapshot) -> SettingsRoute {
        showsPane(route.pane, in: snapshot) ? route : .pane(.general)
    }

    private static func any(_ types: [String], _ snapshot: HostSnapshot) -> Bool {
        types.contains { snapshot.hostCan($0) }
    }
}
