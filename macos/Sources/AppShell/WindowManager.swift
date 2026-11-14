import AppKit
import SwiftUI
import Observation
import PreviewCore
import PreviewUI

final class RequestPanel: NSPanel {
    override func cancelOperation(_ sender: Any?) { close() }
}

@MainActor
public final class WindowManager: NSObject, NSWindowDelegate {
    private let model: AppModel
    private var settings: NSWindow?
    private var welcome: NSWindow?
    private var request: RequestPanel?
    private var notice: NSPanel?

    public init(model: AppModel) {
        self.model = model
        super.init()
        applyAppearance(model.snapshot.preferences.appearance)
        observeAppearance()
    }

    public static let welcomeDoneKey = "welcomeDone"

    static func menuBarIcon() -> NSImage? {
        guard let url = Bundle.main.url(forResource: "tray-icon@2x", withExtension: "png"),
              let image = NSImage(contentsOf: url) else { return nil }
        image.size = NSSize(width: 18, height: 18)
        image.isTemplate = true
        return image
    }

    public static func film() -> String? {
        guard let url = Bundle.main.url(forResource: "icon-film", withExtension: "html") else { return nil }
        return try? String(contentsOf: url, encoding: .utf8)
    }

    func openBundledDashboard() {
        if !Browser.openBundledPage("dashboard-preview") { model.say(Strings.Preview.dashboardMissing) }
    }

    func applyAppearance(_ appearance: Appearance) {
        switch appearance {
        case .system: NSApp.appearance = nil
        case .light: NSApp.appearance = NSAppearance(named: .aqua)
        case .dark: NSApp.appearance = NSAppearance(named: .darkAqua)
        }
    }

    private func observeAppearance() {
        withObservationTracking {
            _ = model.snapshot.preferences.appearance
        } onChange: { [weak self] in
            DispatchQueue.main.async {
                guard let self else { return }
                self.applyAppearance(self.model.snapshot.preferences.appearance)
                self.observeAppearance()
            }
        }
    }

    public func showSettings(_ pane: SettingsPane) {
        showSettings(route: .pane(pane))
    }

    func showSettings(route: SettingsRoute) {
        model.route = route
        if settings == nil {
            settings = makeWindow(KilnRoot {
                SettingsView(actions: SettingsActions(
                    openDashboard: { [weak self] in self?.openBundledDashboard() },
                    showWelcome: { [weak self] in self?.showWelcome() },
                    answer: { [weak self] item in self?.showRequest(item) }
                ))
            }.environment(model), settingsStyle: true)
        }
        if let window = settings, !window.styleMask.contains(.fullScreen), let content = window.contentView {
            let fit = content.fittingSize
            settings?.setContentSize(NSSize(width: 840, height: min(max(490, fit.height), 640)))
        }
        present(settings)
    }

    public func showWelcome() {
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

    public func showRequest(_ item: WaitingItem) {
        let hosting = NSHostingView(rootView: KilnRoot {
            RequestView(itemID: item.id, actions: RequestActions(
                answer: { [weak self] answered, allow in self?.answer(answered, allow: allow) },
                close: { [weak self] in self?.request?.close() }))
        }.environment(model).ignoresSafeArea())
        hosting.sizingOptions = [.minSize, .intrinsicContentSize, .maxSize]
        if let panel = request {
            let top = panel.frame.maxY
            panel.contentView = hosting
            panel.setContentSize(hosting.fittingSize)
            panel.setFrameOrigin(NSPoint(x: panel.frame.minX, y: top - panel.frame.height))
            present(panel)
            return
        }
        let panel = RequestPanel(contentRect: .zero, styleMask: [.titled, .closable, .fullSizeContentView], backing: .buffered, defer: false)
        panel.title = Strings.Request.windowTitle
        panel.titlebarAppearsTransparent = true
        panel.titleVisibility = .hidden
        panel.level = .floating
        panel.hidesOnDeactivate = false
        panel.isReleasedWhenClosed = false
        panel.delegate = self
        panel.contentView = hosting
        panel.setContentSize(hosting.fittingSize)
        panel.center()
        request = panel
        present(panel)
    }

    private func answer(_ item: WaitingItem, allow: Bool) {
        let next = RequestQueue.next(after: item.id, in: model.snapshot)
        model.send(.answerRequest(waitingID: item.id, allow: allow))
        if let next { showRequest(next) } else { request?.close() }
    }

    public func showNotificationFallback(_ note: SampleNotification) {
        let hosting = NSHostingView(rootView: KilnRoot { NotificationCard(note: note) }
            .environment(model)
            .clipShape(RoundedRectangle(cornerRadius: 13)))
        let size = hosting.fittingSize
        let panel = NSPanel(contentRect: NSRect(origin: .zero, size: size), styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
        panel.isOpaque = false
        panel.backgroundColor = .clear
        panel.hasShadow = true
        panel.level = .statusBar
        panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        panel.isReleasedWhenClosed = false
        panel.contentView = hosting
        let visible = NSScreen.main?.visibleFrame ?? .zero
        panel.setFrameOrigin(NSPoint(x: visible.maxX - size.width - 18, y: visible.maxY - size.height - 12))
        notice?.orderOut(nil)
        notice = panel
        panel.orderFrontRegardless()
        DispatchQueue.main.asyncAfter(deadline: .now() + 6) { [weak self, weak panel] in
            panel?.orderOut(nil)
            if let self, self.notice === panel { self.notice = nil }
        }
    }

    private func makeWindow<Content: View>(_ root: Content, settingsStyle: Bool = false) -> NSWindow {
        var mask: NSWindow.StyleMask = [.titled, .closable, .miniaturizable, .fullSizeContentView]
        if settingsStyle { mask.insert(.resizable) }
        let window = NSWindow(contentRect: .zero, styleMask: mask, backing: .buffered, defer: false)
        window.titlebarAppearsTransparent = true
        window.titleVisibility = .hidden
        window.isMovableByWindowBackground = false
        if settingsStyle {
            window.toolbar = NSToolbar(identifier: "settings")
            window.toolbarStyle = .unified
            window.collectionBehavior.insert(.fullScreenPrimary)
            window.contentMinSize = NSSize(width: 840, height: 490)
        }
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

    public func windowWillClose(_ notification: Notification) {
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
