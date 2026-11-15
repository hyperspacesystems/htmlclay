import Foundation

/// The panel and menu bar decisions, away from the views: sample data shows every
/// control, and the real host shows the ones it says it can run.
public enum PanelControls {
    /// The panel's conflict rows: one per folder, in the order they first wait,
    /// named by the folder the item is about. The live host puts the conflict's
    /// kind in detail, so detail names the folder only when no folder matches.
    public static func conflictGroups(in snapshot: HostSnapshot) -> [(folder: String, count: Int)] {
        var order: [String] = []
        var counts: [String: Int] = [:]
        for item in snapshot.waiting where item.kind == .conflict && item.earnsDot {
            let folder = item.subjectID.flatMap { snapshot.folder($0)?.name } ?? item.detail
            if counts[folder] == nil { order.append(folder) }
            counts[folder, default: 0] += 1
        }
        return order.map { (folder: $0, count: counts[$0] ?? 0) }
    }

    /// Whether a recent document's menu item is shown: the real host shows the
    /// ones its can list allows, and sample data shows them all.
    public static func showsDocumentAction(_ type: String, in snapshot: HostSnapshot) -> Bool {
        snapshot.isSample || snapshot.hostCan(type)
    }

    /// Exporting as a zip: sample data offers it, and so does a real host whose
    /// can list runs document.export.
    public static func showsExport(isSample: Bool, can: Bool = false) -> Bool {
        isSample || can
    }

    /// Kept Copies… in the panel menu: sample data offers it, and so does a real
    /// host whose can list runs app.openKeptPage.
    public static func showsKeptCopies(isSample: Bool, can: Bool = false) -> Bool {
        isSample || can
    }

    /// Whether a refused recovery means the person should pick another name: only
    /// the host's destination-exists answer does, and no file is ever removed first.
    public static func asksAnotherDestination(for code: String) -> Bool {
        code == "destination-exists"
    }

    /// Install an update, or restart to finish one.
    public static func showsUpdate(can: Bool) -> Bool { can }

    /// Pause or resume syncing.
    public static func showsSyncPause(can: Bool) -> Bool { can }

    /// Answer a program request on the panel.
    public static func showsAnswer(can: Bool) -> Bool { can }

    /// The app log has no host action yet, so only sample data offers it.
    public static func showsAppLog(isSample: Bool) -> Bool { isSample }
}
