import AppKit

@MainActor
enum Browser {
    static func openBundledPage(_ name: String) -> Bool {
        guard let page = Bundle.main.url(forResource: name, withExtension: "html"),
              let probe = URL(string: "https://example.com"),
              let browser = NSWorkspace.shared.urlForApplication(toOpen: probe) else { return false }
        NSWorkspace.shared.open([page], withApplicationAt: browser, configuration: NSWorkspace.OpenConfiguration())
        return true
    }

    static var defaultBrowserName: String? {
        guard let probe = URL(string: "https://example.com") else { return nil }
        return NSWorkspace.shared.urlForApplication(toOpen: probe)?.deletingPathExtension().lastPathComponent
    }
}
