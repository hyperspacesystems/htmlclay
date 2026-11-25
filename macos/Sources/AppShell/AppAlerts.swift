import AppKit
import PreviewCore

@MainActor
public final class AppAlerts: UpdatePresenter {
    private let model: AppModel

    public init(model: AppModel) { self.model = model }

    public func finishingUp(files: Int) -> Bool {
        let alert = make(Strings.Quit.finishingTitle, Strings.Quit.uploading(files), buttons: [Strings.Quit.quitNow])
        let timer = Timer(timeInterval: 4, repeats: false) { _ in MainActor.assumeIsolated { NSApp.abortModal() } }
        RunLoop.main.add(timer, forMode: .modalPanel)
        let response = alert.runModal()
        timer.invalidate()
        return response == .alertFirstButtonReturn
    }

    public func cantSave() {
        let document = SampleData.cantSaveDocument
        let alert = make(Strings.Quit.cantSaveTitle(document), Strings.Quit.cantSaveMessage,
                         buttons: [Strings.Quit.quitAnyway, Strings.Quit.retry, Strings.Quit.showDocument])
        alert.buttons.first?.hasDestructiveAction = true
        switch alert.runModal() {
        case .alertFirstButtonReturn: model.say(Strings.Quit.wouldQuit)
        case .alertSecondButtonReturn: model.say(Strings.Quit.wouldRetrySave(document))
        default: model.say(Strings.Preview.wouldOpen(document))
        }
    }

    public func cantOpen(name: String, text: String) {
        let message = text.isEmpty ? Strings.Open.cantOpenFallback : text
        _ = make(Strings.Open.cantOpenTitle(name), message, buttons: [Strings.Open.ok]).runModal()
    }

    public func portTaken(port: Int) {
        let alert = make(Strings.Quit.portTakenTitle(port), Strings.Quit.portTakenMessage, buttons: [Strings.Menu.quit])
        if alert.runModal() == .alertFirstButtonReturn { NSApp.terminate(nil) }
    }

    public func portTaken() {
        let process = SampleData.portProcess
        let alert = make(Strings.Quit.portTitle(Strings.Advanced.portNumber, process: process), Strings.Quit.portMessage,
                         buttons: [Strings.Quit.usePort(SampleData.freePort), Strings.Quit.quitProcess(process), Strings.Menu.quit])
        switch alert.runModal() {
        case .alertFirstButtonReturn: model.say(Strings.Quit.wouldUsePort(SampleData.freePort))
        case .alertSecondButtonReturn: model.say(Strings.Quit.wouldQuitProcess(process))
        default: model.say(Strings.Quit.wouldQuit)
        }
    }

    public func askAboutUnsaved(_ lines: [String]) async -> QuitChoice {
        NSApp.activate(ignoringOtherApps: true)
        let alert = NSAlert()
        alert.messageText = Strings.Quit.unsavedTitle
        alert.informativeText = Strings.Quit.unsavedBody(lines)
        let names = [Strings.Quit.checkAndQuit, Strings.Quit.discardAndQuit, Strings.Quit.keepOpen]
        for (index, name) in names.enumerated() {
            let button = alert.addButton(withTitle: name)
            switch index {
            case 0: button.keyEquivalent = "\r"
            case 2: button.keyEquivalent = "\u{1b}"
            default: button.keyEquivalent = ""
            }
        }
        alert.buttons[1].hasDestructiveAction = true
        switch alert.runModal() {
        case .alertFirstButtonReturn: return .retry
        case .alertSecondButtonReturn: return .quitAnyway
        default: return .cancel
        }
    }

    public func askAboutRunningWork() async -> Bool {
        let alert = make(Strings.Updates.runningWorkTitle, Strings.Updates.runningWorkBody,
                         buttons: [Strings.Updates.restartAnyway, Strings.Quit.cancel])
        alert.buttons[0].hasDestructiveAction = true
        alert.buttons[1].keyEquivalent = "\u{1b}"
        return alert.runModal() == .alertFirstButtonReturn
    }

    public func updateNotStarted() {
        _ = make(Strings.Updates.notStartedTitle, Strings.Updates.notStartedBody, buttons: [Strings.Open.ok]).runModal()
    }

    public func restartFailed() {
        _ = make(Strings.Updates.restartFailedTitle, Strings.Updates.restartFailedBody, buttons: [Strings.Open.ok]).runModal()
    }

    public func checkEnded(_ result: UpdateCheckResult) {
        switch result {
        case .latest:
            _ = make(Strings.Updates.latestAlertTitle, Strings.Updates.latestAlertBody(UpdateState.short(model.snapshot.update.current)), buttons: [Strings.Open.ok]).runModal()
        case .failed:
            _ = make(Strings.Updates.failedAlertTitle, Strings.Updates.failedAlertBody, buttons: [Strings.Open.ok]).runModal()
        }
    }

    private func make(_ title: String, _ message: String, buttons: [String]) -> NSAlert {
        NSApp.activate(ignoringOtherApps: true)
        let alert = NSAlert()
        alert.messageText = title
        alert.informativeText = message
        for name in buttons {
            alert.addButton(withTitle: name).keyEquivalent = ""
        }
        return alert
    }
}
