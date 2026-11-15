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
    private var notifier: HostNotifier!
    private var process: HostProcess?
    private var coordinator: TerminationCoordinator?
    private var quitting = false
    private var home: URL?
    private var opener: FileOpener?
    /// Files AppKit asks us to open before there is an opener (a cold launch to open a file).
    private var pendingOpens: [String] = []
    private var profile: Persona?
    private var seedStarted = false
    private var seeded = false
    private var seedError: String?

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

        do {
            profile = try CandidateHome.profile(CommandLine.arguments)
            if profile != nil { try CandidateHome.requireFresh(home) }
        } catch {
            FileHandle.standardError.write(Data("HTML Clay Candidate: \(error)\n".utf8))
            exit(2)
        }

        relay = RelayHostClient()
        model = AppModel(host: relay, initial: relay.last)
        model.folderChooser = OpenPanelFolderChooser()
        alerts = AppAlerts(model: model)
        let opener = FileOpener(model: model)
        opener.onRefused = { [weak self] name, refusal in self?.alerts.cantOpen(name: name, text: refusal.text) }
        self.opener = opener
        model.fileOpener = opener
        let opens = pendingOpens
        pendingOpens = []
        opener.open(opens)
        windows = WindowManager(model: model)
        statusItem = StatusItemController(model: model, windows: windows)
        NSApp.mainMenu = MainMenu.build(settingsTarget: self, settingsAction: #selector(openSettings(_:)),
                                        openTarget: self, openAction: #selector(openAFile(_:)))
        _ = relay.subscribe { [weak self] _ in self?.writeState() }
        relay.onRestart = { [model] in model?.hostRestarted() }
        relay.onRestartRequested = { [weak self] in self?.startHost() }
        notifier = HostNotifier(windows: windows, model: model)
        relay.onNotice = { [notifier] notice in notifier?.show(notice) }

        let welcomeDone = UserDefaults.standard.bool(forKey: WindowManager.welcomeDoneKey)
        if profile == nil, !welcomeDone, !CommandLine.arguments.contains("--no-welcome") { windows.showWelcome() }

        startHost()
    }

    private func startHost() {
        guard let home else { return }
        guard HostRestartPolicy.shouldStart(current: process?.state) else { return }
        let launch = HostLaunch(binary: Self.hostBinary(), environment: CandidateHome.environment(home))
        let process = HostProcess(launch: launch)
        self.process = process
        coordinator = TerminationCoordinator(host: relay, process: process, presenter: alerts)
        process.onReady = { [weak self, weak process] _, client in
            guard let self, let process, self.process === process else { return }
            Task {
                await self.relay.attach(client)
                self.opener?.hostReady()
                self.writeState()
                await self.seedProfile()
            }
        }
        process.onState = { [weak self, weak process] state in
            guard let self, let process, self.process === process else { return }
            switch state {
            case let .failed(failure):
                if failure.code == "port-taken", let port = failure.port { self.alerts.portTaken(port: port) }
                self.opener?.hostGone()
                self.relay.hostGone()
            case .stopped:
                self.opener?.hostGone()
                self.relay.hostGone()
            case .starting:
                self.opener?.hostGone()
            case .running:
                break
            }
            self.writeState()
        }
        writeState()
        Task { _ = await process.start() }
    }

    /// Runs the persona's real actions over the relay once, on the first ready host.
    private func seedProfile() async {
        guard let profile, let home, !seedStarted else { return }
        seedStarted = true
        do {
            try await ProfileSeeder.seed(profile, home: home, host: relay)
            seeded = true
        } catch {
            seedError = "\(error)"
        }
        writeState()
    }

    private static func hostBinary() -> URL {
        #if DEBUG
        if let override = ProcessInfo.processInfo.environment["HTMLCLAY_HOST_BIN"], override.hasPrefix("/") {
            return URL(fileURLWithPath: override)
        }
        #endif
        return Bundle.main.bundleURL.appendingPathComponent("Contents/MacOS/htmlclay-host")
    }

    func application(_ application: NSApplication, open urls: [URL]) {
        let paths = urls.filter(\.isFileURL).map(\.path)
        guard let opener else {
            for path in paths where !pendingOpens.contains(path) { pendingOpens.append(path) }
            return
        }
        opener.open(paths)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard let coordinator else {
            NSApp.reply(toApplicationShouldTerminate: true)
            return .terminateLater
        }
        guard !quitting else { return .terminateCancel }
        quitting = true
        Task {
            let ok = await coordinator.quit()
            if !ok { quitting = false }
            NSApp.reply(toApplicationShouldTerminate: ok)
        }
        return .terminateLater
    }

    @objc func openSettings(_ sender: Any?) { windows.showSettings(.general) }

    @objc func openAFile(_ sender: Any?) { model.chooseAFile() }

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
        var body: [String: Any] = [
            "state": state,
            "epoch": relay?.last.epoch ?? "",
            "rev": relay?.last.rev ?? 0,
            "folders": relay?.last.folders.count ?? 0,
            "failure": failure,
        ]
        if let profile {
            body["profile"] = profile.rawValue
            body["seeded"] = seeded
            if let seedError { body["seedError"] = seedError }
        }
        guard let data = try? JSONSerialization.data(withJSONObject: body, options: [.sortedKeys]) else { return }
        try? data.write(to: home.appendingPathComponent("candidate-state.json"), options: .atomic)
    }
}
