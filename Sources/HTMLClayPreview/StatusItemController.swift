import AppKit
import SwiftUI
import Observation
import PreviewCore
import PreviewUI

final class PanelWindow: NSPanel {
    override var canBecomeKey: Bool { true }
}

@MainActor
final class StatusItemController: NSObject, NSWindowDelegate {
    private let model: AppModel
    private let windows: WindowManager
    private let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
    private var panel: PanelWindow?
    private var outsideClicks: Any?

    init(model: AppModel, windows: WindowManager) {
        self.model = model
        self.windows = windows
        super.init()
        item.button?.target = self
        item.button?.action = #selector(clicked(_:))
        item.button?.sendAction(on: [.leftMouseUp, .rightMouseUp])
        refreshIcon()
    }

    private func refreshIcon() {
        let name = model.showsDot ? "tray-icon-alert@2x" : "tray-icon@2x"
        let image = Bundle.main.url(forResource: name, withExtension: "png").flatMap { NSImage(contentsOf: $0) }
            ?? NSImage(systemSymbolName: "circle.fill", accessibilityDescription: nil)
        image?.size = NSSize(width: 18, height: 18)
        image?.isTemplate = true
        item.button?.image = image
        withObservationTracking { _ = model.showsDot } onChange: { [weak self] in
            DispatchQueue.main.async { self?.refreshIcon() }
        }
    }

    @objc private func clicked(_ sender: Any?) {
        let event = NSApp.currentEvent
        let secondary = event?.type == .rightMouseUp || event?.modifierFlags.contains(.control) == true
        model.markWaitingSeen()
        if secondary {
            closePanel()
            item.menu = buildMenu()
            item.button?.performClick(nil)
            item.menu = nil
        } else if panel?.isVisible == true {
            closePanel()
        } else {
            openPanel()
        }
    }

    private func buildMenu() -> NSMenu {
        let menu = NSMenu()
        add(Strings.Panel.openDashboard, #selector(openDashboard), to: menu)
        if model.snapshot.totalCount > 0 {
            menu.addItem(.separator())
            add(Strings.Menu.needsYou(model.snapshot.totalCount), #selector(openDashboard), to: menu)
        }
        menu.addItem(.separator())
        add(Strings.Menu.settings, #selector(openSettings), to: menu, key: ",")
        let quit = NSMenuItem(title: Strings.Menu.quit, action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        menu.addItem(quit)
        return menu
    }

    private func add(_ title: String, _ action: Selector, to menu: NSMenu, key: String = "") {
        let entry = NSMenuItem(title: title, action: action, keyEquivalent: key)
        entry.target = self
        menu.addItem(entry)
    }

    @objc private func openDashboard() { closePanel() }
    @objc private func openSettings() { closePanel(); windows.showSettings(.general) }

    private func openPanel() {
        guard let button = item.button, let anchor = button.window else { return }
        let actions = PanelActions(
            openDashboard: { [weak self] in self?.openDashboard() },
            openSettings: { [weak self] pane in self?.closePanel(); self?.windows.showSettings(pane) })
        let hosting = NSHostingView(rootView: KilnRoot { PanelView(actions: actions) }
            .environment(model)
            .clipShape(RoundedRectangle(cornerRadius: 12)))
        let size = hosting.fittingSize
        let panel = PanelWindow(contentRect: NSRect(origin: .zero, size: size), styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
        panel.isOpaque = false
        panel.backgroundColor = .clear
        panel.hasShadow = true
        panel.level = .popUpMenu
        panel.collectionBehavior = [.transient, .moveToActiveSpace, .fullScreenAuxiliary]
        panel.isReleasedWhenClosed = false
        panel.contentView = hosting
        panel.delegate = self
        let visible = (anchor.screen ?? NSScreen.main)?.visibleFrame ?? .zero
        var x = anchor.frame.midX - size.width / 2
        x = min(max(x, visible.minX + 8), visible.maxX - size.width - 8)
        panel.setFrameOrigin(NSPoint(x: x, y: anchor.frame.minY - size.height - 6))
        panel.makeKeyAndOrderFront(nil)
        self.panel = panel
        button.highlight(true)
        outsideClicks = NSEvent.addGlobalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown]) { [weak self] _ in
            DispatchQueue.main.async { self?.closePanel() }
        }
    }

    private func closePanel() {
        if let outsideClicks { NSEvent.removeMonitor(outsideClicks) }
        outsideClicks = nil
        panel?.orderOut(nil)
        panel = nil
        item.button?.highlight(false)
    }

    func windowDidResignKey(_ notification: Notification) { closePanel() }
}

@MainActor
final class PreviewMenuController: NSObject {
    private let host: SampleHost
    private let model: AppModel
    private let windows: WindowManager
    private let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)

    init(host: SampleHost, model: AppModel, windows: WindowManager) {
        self.host = host
        self.model = model
        self.windows = windows
        super.init()
        item.button?.title = Strings.Preview.menuTitle
        rebuild()
    }

    private func rebuild() {
        let menu = NSMenu()
        let header = NSMenuItem(title: Strings.Preview.pretend, action: nil, keyEquivalent: "")
        header.isEnabled = false
        menu.addItem(header)
        for persona in Persona.allCases {
            let entry = NSMenuItem(title: Strings.Preview.persona(persona), action: #selector(pretend(_:)), keyEquivalent: "")
            entry.target = self
            entry.representedObject = persona.rawValue
            entry.state = persona == host.persona ? .on : .off
            menu.addItem(entry)
        }
        menu.addItem(.separator())
        let welcome = NSMenuItem(title: Strings.Welcome.title, action: #selector(showWelcome), keyEquivalent: "")
        welcome.target = self
        menu.addItem(welcome)
        let request = NSMenuItem(title: Strings.Request.allow, action: #selector(showRequest), keyEquivalent: "")
        request.target = self
        menu.addItem(request)
        item.menu = menu
    }

    @objc private func pretend(_ sender: NSMenuItem) {
        guard let raw = sender.representedObject as? String, let persona = Persona(rawValue: raw) else { return }
        host.pretend(persona)
        rebuild()
    }

    @objc private func showWelcome() { windows.showWelcome() }

    @objc private func showRequest() {
        let item = model.snapshot.waiting.first ?? WaitingItem(id: "sample", kind: .programRequest, pane: .programs, title: Strings.Request.title("todo.htmlclay"), earnsDot: true)
        windows.showRequest(item)
    }
}
