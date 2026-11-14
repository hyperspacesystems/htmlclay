import Foundation

/// The panel and menu bar decisions, away from the views: sample data shows every
/// control, and the real host shows the ones it says it can run.
public enum PanelControls {
    /// Whether a recent document's menu item is shown: the real host shows the
    /// ones its can list allows, and sample data shows them all.
    public static func showsDocumentAction(_ type: String, in snapshot: HostSnapshot) -> Bool {
        snapshot.isSample || snapshot.hostCan(type)
    }

    /// Exporting as a zip has no host action yet, so only sample data offers it.
    public static func showsExport(isSample: Bool) -> Bool { isSample }

    /// Install an update, or restart to finish one.
    public static func showsUpdate(can: Bool) -> Bool { can }

    /// Pause or resume syncing.
    public static func showsSyncPause(can: Bool) -> Bool { can }

    /// Answer a program request on the panel.
    public static func showsAnswer(can: Bool) -> Bool { can }

    /// The app log has no host action yet, so only sample data offers it.
    public static func showsAppLog(isSample: Bool) -> Bool { isSample }
}
