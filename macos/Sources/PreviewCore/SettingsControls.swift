import Foundation

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
