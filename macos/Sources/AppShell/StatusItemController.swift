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
            item.menu = buildOneMenu(rows: oneMenuRows())
            item.button?.performClick(nil)
            item.menu = nil
        } else if panel?.isVisible == true {
            closePanel()
        } else {
            openPanel()
        }
    }

    /// The one menu: the status item and the panel's gear both show these rows.
    private func oneMenuRows() -> [MenuRow] {
        MenuModel.rows(MenuInput(model.snapshot), phase: updatePhase)
    }

    private var updatePhase: MenuUpdatePhase {
        MenuUpdatePhase(checking: updates?.checking ?? false, ready: updates?.prepared ?? "",
                        downloading: updates?.downloading ?? "")
    }

    func buildOneMenu(rows: [MenuRow]) -> NSMenu {
        let menu = NSMenu()
        for row in rows {
            if row.id == MenuModel.separator {
                menu.addItem(.separator())
            } else {
                menu.addItem(entry(for: row))
            }
        }
        return menu
    }

    private func entry(for row: MenuRow) -> NSMenuItem {
        let title = oneMenuTitle(for: row)
        let menuItem = NSMenuItem(title: title, action: nil, keyEquivalent: oneMenuKey(for: row.id))
        menuItem.target = self
        if let children = row.children {
            let submenu = NSMenu()
            for child in children {
                let childItem = NSMenuItem(title: oneMenuTitle(for: child), action: child.enabled ? #selector(openMenuChild(_:)) : nil, keyEquivalent: "")
                childItem.target = self
                childItem.representedObject = child.id
                childItem.isEnabled = child.enabled
                submenu.addItem(childItem)
            }
            menuItem.submenu = submenu
            menuItem.isEnabled = row.enabled
            return menuItem
        }
        guard row.enabled, let action = oneMenuAction(for: row.id) else {
            menuItem.isEnabled = false
            return menuItem
        }
        menuItem.action = action
        return menuItem
    }

    private func oneMenuAction(for id: String) -> Selector? {
        switch id {
        case "status": return #selector(restartToUpdate)
        case "pauseSync": return #selector(toggleSync)
        case "openDashboard": return #selector(openDashboard)
        case "openFile": return #selector(openAFile)
        case "recoveredChanges": return #selector(openRecoveredChanges)
        case "settings": return #selector(openSettings)
        case "checkForUpdates": return #selector(checkForUpdates)
        case "quit": return #selector(quit)
        default: return nil
        }
    }

    private func oneMenuKey(for id: String) -> String {
        switch id {
        case "settings": return ","
        case "quit": return "q"
        default: return ""
        }
    }

    private func oneMenuTitle(for row: MenuRow) -> String {
        switch row.id {
        case "status":
            switch row.kind {
            case "ready": return Strings.Menu.restartToUpdate(UpdateState.short(updatePhase.ready))
            case "checking": return Strings.Updates.checking
            case "downloading": return Strings.Updates.downloading(UpdateState.short(updatePhase.downloading))
            default: return Strings.Menu.version(UpdateState.short(model.snapshot.update.current))
            }
        case "syncLine": return Strings.Panel.syncLine(model.snapshot.sync)
        case "pauseSync": return model.snapshot.sync.paused ? Strings.Panel.resumeSyncing : Strings.Panel.pauseSyncing
        case "needsYou": return Strings.Menu.needsYou(model.snapshot.totalCount)
        case "openDashboard": return Strings.Panel.openDashboard
        case "openFile": return Strings.Panel.openAFile
        case "recent": return Strings.Panel.recentDocuments
        case "noRecent": return Strings.Menu.noRecent
        case "recoveredChanges": return Strings.Menu.recoveredChanges
        case "settings": return Strings.Menu.settings
        case "checkForUpdates": return Strings.Menu.checkForUpdates
        case "quit": return Strings.Menu.quit
        default:
            if let item = model.snapshot.waiting.first(where: { $0.id == row.id }) { return item.title }
            if let document = model.snapshot.documents.first(where: { $0.id == row.id }) { return document.name }
            return ""
        }
    }

    @objc private func openDashboard() {
        closePanel()
        windows.openBundledDashboard()
    }
    @objc private func openSettings() { closePanel(); windows.showSettings(.general) }

    @objc private func openAFile() {
        closePanel()
        model.chooseAFile()
    }

    @objc private func openMenuChild(_ sender: NSMenuItem) {
        guard let id = sender.representedObject as? String else { return }
        closePanel()
        if let item = model.snapshot.waiting.first(where: { $0.id == id }) {
            if item.kind == .programRequest {
                windows.showRequest(item)
            } else {
                windows.showSettings(item.pane)
            }
            return
        }
        guard let document = model.snapshot.documents.first(where: { $0.id == id }) else { return }
        if model.isSample {
            model.say(Strings.Preview.wouldOpen(document.name))
        } else {
            model.send(.openDocument(id: document.id))
        }
    }

    @objc private func openRecoveredChanges() {
        closePanel()
        model.send(.openRecoveredChanges)
    }

    /// The menu items target this controller, which AppKit disables for actions it cannot answer,
    /// so Quit needs its own method rather than NSApplication.terminate(_:).
    @objc private func quit() { NSApp.terminate(nil) }

    @objc private func checkForUpdates() {
        closePanel()
        updates?.checkNow()
    }

    /// The panel's gear shows the same rows as the status item, at the click and inside the open
    /// panel. Closing the panel first left the menu with no key window in an inactive app, so
    /// macOS dismissed it at once. Each row's action closes the panel; dismissing the menu keeps it.
    private func showOneMenu() {
        guard let panel, let content = panel.contentView else { return }
        let point = content.convert(panel.convertPoint(fromScreen: NSEvent.mouseLocation), from: nil)
        buildOneMenu(rows: oneMenuRows()).popUp(positioning: nil, at: point, in: content)
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
            openMenu: { [weak self] in self?.showOneMenu() },
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
            _ = model.updaterRev
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
