import AppKit
import SwiftUI
import Observation
import PreviewCore
import PreviewUI
import AppShell

@MainActor
final class PreviewMenuController: NSObject, NSMenuDelegate {
    private let host: SampleHost
    private let model: AppModel
    private let windows: WindowManager
    private let alerts: AppAlerts
    private let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
    private lazy var notifier = SampleNotifier(windows: windows)

    init(host: SampleHost, model: AppModel, windows: WindowManager, alerts: AppAlerts) {
        self.host = host
        self.model = model
        self.windows = windows
        self.alerts = alerts
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
        let requestMenu = NSMenu()
        for kind in ProgramRequest.Kind.allCases {
            let entry = NSMenuItem(title: Strings.Preview.requestKind(kind), action: #selector(showRequest(_:)), keyEquivalent: "")
            entry.target = self
            entry.representedObject = kind.rawValue
            requestMenu.addItem(entry)
        }
        let request = NSMenuItem(title: Strings.Preview.showRequest, action: nil, keyEquivalent: "")
        request.submenu = requestMenu
        menu.addItem(request)
        menu.addItem(.separator())
        let overlayMenu = NSMenu()
        overlayMenu.autoenablesItems = false
        overlayMenu.delegate = self
        for overlay in PreviewOverlay.allCases {
            let entry = NSMenuItem(title: Strings.Preview.overlay(overlay), action: #selector(toggleOverlay(_:)), keyEquivalent: "")
            entry.target = self
            entry.representedObject = overlay.rawValue
            overlayMenu.addItem(entry)
        }
        let pretendMore = NSMenuItem(title: Strings.Preview.alsoPretend, action: nil, keyEquivalent: "")
        pretendMore.submenu = overlayMenu
        menu.addItem(pretendMore)
        menu.addItem(.separator())
        addPreviewItem(Strings.Preview.quitWhileUploading, #selector(quitWhileUploading), to: menu)
        addPreviewItem(Strings.Preview.saveCantFinish, #selector(cantSave), to: menu)
        addPreviewItem(Strings.Preview.portTaken, #selector(portTaken), to: menu)
        let notes = NSMenu()
        for note in SampleNotification.allCases {
            let entry = NSMenuItem(title: Strings.Notify.title(note), action: #selector(sendNotification(_:)), keyEquivalent: "")
            entry.target = self
            entry.representedObject = note.rawValue
            notes.addItem(entry)
        }
        let noteParent = NSMenuItem(title: Strings.Preview.sendNotification, action: nil, keyEquivalent: "")
        noteParent.submenu = notes
        menu.addItem(noteParent)
        item.menu = menu
    }

    private func addPreviewItem(_ title: String, _ action: Selector, to menu: NSMenu) {
        let entry = NSMenuItem(title: title, action: action, keyEquivalent: "")
        entry.target = self
        menu.addItem(entry)
    }

    func menuNeedsUpdate(_ menu: NSMenu) {
        for entry in menu.items {
            guard let raw = entry.representedObject as? String, let overlay = PreviewOverlay(rawValue: raw) else { continue }
            entry.state = host.isOn(overlay) ? .on : .off
            entry.isEnabled = host.canToggle(overlay)
        }
    }

    @objc private func toggleOverlay(_ sender: NSMenuItem) {
        guard let raw = sender.representedObject as? String, let overlay = PreviewOverlay(rawValue: raw) else { return }
        host.toggle(overlay)
    }

    @objc private func pretend(_ sender: NSMenuItem) {
        guard let raw = sender.representedObject as? String, let persona = Persona(rawValue: raw) else { return }
        host.pretend(persona)
        model.resetPreviewState()
        rebuild()
    }

    @objc private func showWelcome() { windows.showWelcome() }

    @objc private func quitWhileUploading() {
        _ = alerts.finishingUp(files: max(model.snapshot.sync.syncingFiles, 3))
        model.say(Strings.Quit.wouldQuit)
    }

    @objc private func cantSave() { alerts.cantSave() }

    @objc private func portTaken() { alerts.portTaken() }

    @objc private func sendNotification(_ sender: NSMenuItem) {
        guard let raw = sender.representedObject as? String, let note = SampleNotification(rawValue: raw) else { return }
        notifier.send(note)
    }

    @objc private func showRequest(_ sender: NSMenuItem) {
        guard let raw = sender.representedObject as? String, let kind = ProgramRequest.Kind(rawValue: raw) else { return }
        windows.showRequest(host.addSampleRequest(kind))
    }
}
