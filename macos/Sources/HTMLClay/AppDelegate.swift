import AppKit
import PreviewCore
import AppShell

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    static let bundleID = "com.htmlclay.HTMLClayCandidate"

    private var relay: RelayHostClient!
    private var model: AppModel!
    private var windows: WindowManager!
    private var statusItem: StatusItemController!
    private var alerts: AppAlerts!
    private var process: HostProcess?
    private var home: URL?
    private var queuedOpens: [URL] = []

    func applicationDidFinishLaunching(_ notification: Notification) {
        let home: URL
        do {
            home = try CandidateHome.resolve()
            try CandidateHome.prepare(home)
        } catch {
            FileHandle.standardError.write(Data("HTML Clay Candidate: \(error)\n".utf8))
            exit(1)
        }
        self.home = home

        relay = RelayHostClient()
        model = AppModel(host: relay, initial: relay.last)
        model.folderChooser = OpenPanelFolderChooser()
        alerts = AppAlerts(model: model)
        windows = WindowManager(model: model)
        statusItem = StatusItemController(model: model, windows: windows)
        NSApp.mainMenu = MainMenu.build(settingsTarget: self, settingsAction: #selector(openSettings(_:)))
        _ = relay.subscribe { [weak self] _ in self?.writeState() }
        relay.onRestart = { [model] in model?.hostRestarted() }
        relay.onRestartRequested = { [weak self] in self?.startHost() }

        let welcomeDone = UserDefaults.standard.bool(forKey: WindowManager.welcomeDoneKey)
        if !welcomeDone, !CommandLine.arguments.contains("--no-welcome") { windows.showWelcome() }

        startHost()
    }

    private func startHost() {
        guard let home else { return }
        let launch = HostLaunch(binary: Self.hostBinary(), environment: CandidateHome.environment(home))
        let process = HostProcess(launch: launch)
        self.process = process
        process.onReady = { [weak self] _, client in
            guard let self else { return }
            Task {
                await self.relay.attach(client)
                self.flushQueuedOpens()
                self.writeState()
            }
        }
        process.onState = { [weak self] state in
            guard let self else { return }
            switch state {
            case let .failed(failure):
                if failure.code == "port-taken" { self.alerts.portTaken() }
                self.relay.hostGone()
            case .stopped:
                self.relay.hostGone()
            case .starting, .running:
                break
            }
            self.writeState()
        }
        writeState()
        Task { _ = await process.start() }
    }

    private static func hostBinary() -> URL {
        #if DEBUG
        if let override = ProcessInfo.processInfo.environment["HTMLCLAY_HOST_BIN"], override.hasPrefix("/") {
            return URL(fileURLWithPath: override)
        }
        #endif
        return Bundle.main.bundleURL.appendingPathComponent("Contents/MacOS/htmlclay-host")
    }

    private func flushQueuedOpens() {
        guard !queuedOpens.isEmpty else { return }
        let urls = queuedOpens
        queuedOpens = []
        for url in urls { model.send(.openFile(path: url.path)) }
    }

    func application(_ application: NSApplication, open urls: [URL]) {
        for url in urls where url.isFileURL {
            if model != nil, case .running = process?.state {
                model.send(.openFile(path: url.path))
                queuedOpens.removeAll { $0 == url }
            } else if !queuedOpens.contains(url) {
                queuedOpens.append(url)
            }
        }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        Task { [process] in
            _ = await process?.stop(timeout: 10)
            NSApp.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }

    @objc func openSettings(_ sender: Any?) { windows.showSettings(.general) }

    private func writeState() {
        guard let home else { return }
        let state: String
        switch process?.state {
        case .starting: state = "starting"
        case .running: state = "running"
        case .failed: state = "failed"
        case .stopped: state = "stopped"
        case .none: state = "starting"
        }
        var failure = ""
        if case let .failed(reason)? = process?.state { failure = reason.code }
        let body: [String: Any] = [
            "state": state,
            "epoch": relay?.last.epoch ?? "",
            "rev": relay?.last.rev ?? 0,
            "folders": relay?.last.folders.count ?? 0,
            "failure": failure,
        ]
        guard let data = try? JSONSerialization.data(withJSONObject: body, options: [.sortedKeys]) else { return }
        try? data.write(to: home.appendingPathComponent("candidate-state.json"), options: .atomic)
    }
}
