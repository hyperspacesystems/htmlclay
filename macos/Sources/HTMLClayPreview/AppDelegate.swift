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
        model.folderChooser = SampleFolderChooser()
        alerts = AppAlerts(model: model)
        windows = WindowManager(model: model)
        statusItem = StatusItemController(model: model, windows: windows)
        previewMenu = PreviewMenuController(host: host, model: model, windows: windows, alerts: alerts)
        NSApp.mainMenu = MainMenu.build(settingsTarget: self, settingsAction: #selector(openSettings(_:)))
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
