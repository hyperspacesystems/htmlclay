import AppKit
import UniformTypeIdentifiers
import PreviewCore

/// The real app's chooser: a new sheet starts empty, and Choose… opens a folder panel.
@MainActor
public final class OpenPanelFolderChooser: FolderChooser {
    public init() {}

    public func first(in snapshot: HostSnapshot) -> String? { nil }

    public func choose(after current: String?, in snapshot: HostSnapshot) async -> String? { pickFolder() }

    public func chooseToTrust(after current: String?, in snapshot: HostSnapshot) async -> String? { pickFolder() }

    public func chooseFile() async -> String? { pickFile() }

    public func chooseProgram() async -> String? { pickProgram() }

    private func pickProgram() -> String? {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = false
        panel.canChooseFiles = true
        panel.allowsMultipleSelection = false
        guard panel.runModal() == .OK else { return nil }
        return panel.url?.path
    }

    private func pickFile() -> String? {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = false
        panel.canChooseFiles = true
        panel.allowsMultipleSelection = false
        panel.allowedContentTypes = ["html", "htm", "htmlclay"].compactMap { UTType(filenameExtension: $0) }
        guard panel.runModal() == .OK else { return nil }
        return panel.url?.path
    }

    private func pickFolder() -> String? {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        guard panel.runModal() == .OK else { return nil }
        return panel.url?.path
    }
}
