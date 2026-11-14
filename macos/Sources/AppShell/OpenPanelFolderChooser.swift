import AppKit
import PreviewCore

/// The real app's chooser: a new sheet starts empty, and Choose… opens a folder panel.
@MainActor
public final class OpenPanelFolderChooser: FolderChooser {
    public init() {}

    public func first(in snapshot: HostSnapshot) -> String? { nil }

    public func choose(after current: String?, in snapshot: HostSnapshot) async -> String? {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        guard panel.runModal() == .OK else { return nil }
        return panel.url?.path
    }
}
