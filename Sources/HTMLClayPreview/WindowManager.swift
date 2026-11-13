import AppKit
import SwiftUI
import PreviewCore
import PreviewUI

@MainActor
final class WindowManager: NSObject, NSWindowDelegate {
    private let model: AppModel
    private var settings: NSWindow?
    private var welcome: NSWindow?
    private var request: NSPanel?

    init(model: AppModel) {
        self.model = model
        super.init()
    }

    static let welcomeDoneKey = "welcomeDone"

    static func menuBarIcon() -> NSImage? {
        guard let url = Bundle.main.url(forResource: "tray-icon@2x", withExtension: "png"),
              let image = NSImage(contentsOf: url) else { return nil }
        image.size = NSSize(width: 18, height: 18)
        image.isTemplate = true
        return image
    }

    static func film() -> String? {
        guard let url = Bundle.main.url(forResource: "icon-film", withExtension: "html") else { return nil }
        return try? String(contentsOf: url, encoding: .utf8)
    }

    func openDashboard() {
        if !Browser.openBundledPage("dashboard-preview") { model.say(Strings.Preview.dashboardMissing) }
    }

    func showSettings(_ pane: SettingsPane) {
        model.settingsPane = pane
        if settings == nil {
            settings = makeWindow(KilnRoot { SettingsView() }.environment(model))
        }
        present(settings)
    }

    func showWelcome() {
        welcome?.close()
        welcome = makeWindow(KilnRoot {
            WelcomeView(film: Self.film(),
                        menuBarIcon: Self.menuBarIcon(),
                        done: { [weak self] in
                            UserDefaults.standard.set(true, forKey: Self.welcomeDoneKey)
                            self?.welcome?.close()
                        },
                        openSettings: { [weak self] in self?.welcome?.close(); self?.showSettings(.general) })
        }.environment(model))
        present(welcome)
    }

    func showRequest(_ item: WaitingItem) {
        request?.close()
        let panel = NSPanel(contentRect: .zero, styleMask: [.titled, .closable, .fullSizeContentView], backing: .buffered, defer: false)
        panel.title = Strings.App.name
        panel.level = .floating
        panel.hidesOnDeactivate = false
        panel.isReleasedWhenClosed = false
        panel.delegate = self
        panel.contentView = NSHostingView(rootView: KilnRoot {
            RequestView(item: item) { [weak self] allow in
                self?.model.send(.answerRequest(waitingID: item.id, allow: allow))
                self?.request?.close()
            }
        }.environment(model))
        panel.setContentSize(panel.contentView?.fittingSize ?? NSSize(width: 460, height: 200))
        panel.center()
        request = panel
        present(panel)
    }

    private func makeWindow<Content: View>(_ root: Content) -> NSWindow {
        let window = NSWindow(contentRect: .zero, styleMask: [.titled, .closable, .miniaturizable, .fullSizeContentView], backing: .buffered, defer: false)
        window.titlebarAppearsTransparent = true
        window.titleVisibility = .hidden
        window.isMovableByWindowBackground = true
        window.isReleasedWhenClosed = false
        window.delegate = self
        let hosting = NSHostingView(rootView: root.ignoresSafeArea())
        window.contentView = hosting
        window.setContentSize(hosting.fittingSize)
        window.center()
        return window
    }

    private func present(_ window: NSWindow?) {
        NSApp.setActivationPolicy(.regular)
        NSApp.activate(ignoringOtherApps: true)
        window?.makeKeyAndOrderFront(nil)
    }

    func windowWillClose(_ notification: Notification) {
        let closing = notification.object as? NSWindow
        DispatchQueue.main.async { [weak self] in
            guard let self else { return }
            if closing === self.welcome { self.welcome = nil }
            if closing === self.request { self.request = nil }
            let open = [self.settings, self.welcome, self.request].compactMap { $0 }.contains { $0.isVisible }
            if !open { NSApp.setActivationPolicy(.accessory) }
        }
    }
}
