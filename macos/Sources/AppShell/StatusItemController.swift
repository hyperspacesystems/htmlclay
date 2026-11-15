import AppKit
import SwiftUI
import Observation
import PreviewCore
import PreviewUI

final class PanelWindow: NSPanel {
    override var canBecomeKey: Bool { true }
}

@MainActor
public final class StatusItemController: NSObject, NSWindowDelegate {
    private let model: AppModel
    private let windows: WindowManager
    private let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
    private var panel: PanelWindow?
    private var outsideClicks: Any?
    public var updates: UpdateCoordinator?

    public init(model: AppModel, windows: WindowManager) {
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
        add(Strings.Panel.openAFile, #selector(openAFile), to: menu)
        if !model.snapshot.documents.isEmpty {
            let recent = NSMenu()
            for document in model.snapshot.documents.prefix(8) {
                let entry = NSMenuItem(title: document.name, action: #selector(openRecent(_:)), keyEquivalent: "")
                entry.target = self
                entry.representedObject = document.id
                recent.addItem(entry)
            }
            let parent = NSMenuItem(title: Strings.Panel.recentDocuments, action: nil, keyEquivalent: "")
            parent.submenu = recent
            menu.addItem(parent)
        }
        if model.snapshot.totalCount > 0 {
            menu.addItem(.separator())
            let needs = NSMenu()
            for item in model.snapshot.waiting {
                let entry = NSMenuItem(title: item.title, action: #selector(openWaitingItem(_:)), keyEquivalent: "")
                entry.target = self
                entry.representedObject = item.id
                needs.addItem(entry)
            }
            let parent = NSMenuItem(title: Strings.Menu.needsYou(model.snapshot.totalCount), action: nil, keyEquivalent: "")
            parent.submenu = needs
            menu.addItem(parent)
        }
        if model.snapshot.sync.connected {
            menu.addItem(.separator())
            let status = NSMenuItem(title: Strings.Panel.syncLine(model.snapshot.sync), action: nil, keyEquivalent: "")
            status.isEnabled = false
            menu.addItem(status)
            if PanelControls.showsSyncPause(can: model.can(HostActionType.syncSetPaused)) {
                add(model.snapshot.sync.paused ? Strings.Panel.resumeSyncing : Strings.Panel.pauseSyncing, #selector(toggleSync), to: menu)
            }
        }
        if updates?.prepared != nil {
            menu.addItem(.separator())
            add(Strings.Updates.restart, #selector(restartToUpdate), to: menu)
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

    @objc private func openDashboard() {
        closePanel()
        windows.openBundledDashboard()
    }
    @objc private func openSettings() { closePanel(); windows.showSettings(.general) }

    @objc private func openAFile() {
        closePanel()
        if model.isSample {
            model.say(Strings.Preview.wouldOpenFile)
        } else {
            model.chooseAFile()
        }
    }

    @objc private func openRecent(_ sender: NSMenuItem) {
        guard let id = sender.representedObject as? String,
              let document = model.snapshot.documents.first(where: { $0.id == id }) else { return }
        closePanel()
        if model.isSample {
            model.say(Strings.Preview.wouldOpen(document.name))
        } else {
            model.send(.openDocument(id: document.id))
        }
    }

    @objc private func openWaitingItem(_ sender: NSMenuItem) {
        guard let id = sender.representedObject as? String,
              let item = model.snapshot.waiting.first(where: { $0.id == id }) else { return }
        closePanel()
        if item.kind == .programRequest {
            windows.showRequest(item)
        } else {
            windows.showSettings(item.pane)
        }
    }

    @objc private func restartToUpdate() {
        closePanel()
        updates?.restartToUpdate()
    }

    @objc private func toggleSync() {
        model.send(.setSyncPaused(!model.snapshot.sync.paused))
    }

    private func openPanel() {
        guard let button = item.button, let anchor = button.window else { return }
        let actions = PanelActions(
            openDashboard: { [weak self] in self?.openDashboard() },
            openSettings: { [weak self] pane in self?.closePanel(); self?.windows.showSettings(pane) },
            answer: { [weak self] item in self?.closePanel(); self?.windows.showRequest(item) },
            quit: { NSApp.terminate(nil) })
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
        observeSize()
        outsideClicks = NSEvent.addGlobalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown]) { [weak self] _ in
            DispatchQueue.main.async { self?.closePanel() }
        }
    }

    private func observeSize() {
        withObservationTracking {
            _ = model.snapshot
            _ = model.note
        } onChange: { [weak self] in
            DispatchQueue.main.async { self?.sizePanel() }
        }
    }

    private func sizePanel() {
        guard let panel, let content = panel.contentView else { return }
        let size = content.fittingSize
        let top = panel.frame.maxY
        panel.setContentSize(size)
        panel.setFrameOrigin(NSPoint(x: panel.frame.minX, y: top - size.height))
        observeSize()
    }

    private func closePanel() {
        if let outsideClicks { NSEvent.removeMonitor(outsideClicks) }
        outsideClicks = nil
        panel?.orderOut(nil)
        panel = nil
        item.button?.highlight(false)
    }

    public func windowDidResignKey(_ notification: Notification) { closePanel() }
}
