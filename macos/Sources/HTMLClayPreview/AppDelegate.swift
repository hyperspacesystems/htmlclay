import AppKit
import PreviewCore
import PreviewUI
import AppShell

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var host: SampleHost!
    private var model: AppModel!
    private var windows: WindowManager!
    private var statusItem: StatusItemController!
    private var previewMenu: PreviewMenuController!
    private var alerts: AppAlerts!

    func applicationDidFinishLaunching(_ notification: Notification) {
        host = SampleHost(persona: .newPerson)
        model = AppModel(host: host, initial: host.current)
        alerts = AppAlerts(model: model)
        windows = WindowManager(model: model)
        statusItem = StatusItemController(model: model, windows: windows)
        previewMenu = PreviewMenuController(host: host, model: model, windows: windows, alerts: alerts)
        NSApp.mainMenu = MainMenu.build(target: self)
        let welcomeDone = UserDefaults.standard.bool(forKey: WindowManager.welcomeDoneKey)
        if !welcomeDone, !CommandLine.arguments.contains("--no-welcome") { windows.showWelcome() }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        let files = model.snapshot.filesStillUploading
        if files > 0 { _ = alerts.finishingUp(files: files) }
        return .terminateNow
    }

    @objc func openSettings(_ sender: Any?) { windows.showSettings(.general) }
}

@MainActor
enum MainMenu {
    static func build(target: AppDelegate) -> NSMenu {
        let main = NSMenu()
        let appItem = NSMenuItem()
        let appMenu = NSMenu()
        let settings = NSMenuItem(title: Strings.Menu.settings, action: #selector(AppDelegate.openSettings(_:)), keyEquivalent: ",")
        settings.target = target
        appMenu.addItem(settings)
        appMenu.addItem(.separator())
        appMenu.addItem(NSMenuItem(title: Strings.Menu.quit, action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q"))
        appItem.submenu = appMenu
        main.addItem(appItem)

        let editItem = NSMenuItem()
        let edit = NSMenu(title: "Edit")
        edit.addItem(NSMenuItem(title: "Undo", action: Selector(("undo:")), keyEquivalent: "z"))
        edit.addItem(NSMenuItem(title: "Cut", action: #selector(NSText.cut(_:)), keyEquivalent: "x"))
        edit.addItem(NSMenuItem(title: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c"))
        edit.addItem(NSMenuItem(title: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v"))
        edit.addItem(NSMenuItem(title: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a"))
        editItem.submenu = edit
        main.addItem(editItem)

        let windowItem = NSMenuItem()
        let window = NSMenu(title: "Window")
        window.addItem(NSMenuItem(title: "Close", action: #selector(NSWindow.performClose(_:)), keyEquivalent: "w"))
        window.addItem(NSMenuItem(title: "Minimize", action: #selector(NSWindow.performMiniaturize(_:)), keyEquivalent: "m"))
        windowItem.submenu = window
        main.addItem(windowItem)
        return main
    }
}
