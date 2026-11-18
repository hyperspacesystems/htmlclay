import AppKit
import PreviewCore
import AppShell

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    static let bundleID = Bundle.main.bundleIdentifier ?? AppIdentity.candidateBundleID
    static let identity = AppIdentity.of(bundleID: Bundle.main.bundleIdentifier)

    private var relay: RelayHostClient!
    private var model: AppModel!
    private var windows: WindowManager!
    private var statusItem: StatusItemController!
    private var alerts: AppAlerts!
    private var notifier: HostNotifier!
    private var process: HostProcess?
    private var coordinator: TerminationCoordinator?
    private var updates: UpdateCoordinator?
    private var updateNowObserver: NSObjectProtocol?
    private var updateAllowed = false
    private var loginItems: LoginItemReconciler?
    private var hostReadyOnce = false
    /// The downloaded version last told to the host; nil until a host hears it.
    private var reportedPrepared: String?
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
        if Self.identity.usesCandidateHome {
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
        }

        relay = RelayHostClient()
        model = AppModel(host: relay, initial: relay.last)
        model.folderChooser = OpenPanelFolderChooser()
        model.fileTypes = MacFileTypes()
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
        let info = Bundle.main.infoDictionary ?? [:]
        let version = info["CFBundleShortVersionString"] as? String ?? ""
        let bundlePath = Bundle.main.bundlePath
        let allowed = UpdateGate.allows(feedURL: info["SUFeedURL"] as? String, publicKey: info["SUPublicEDKey"] as? String,
                                        bundlePath: bundlePath, writable: FileManager.default.isWritableFile(atPath: bundlePath),
                                        version: version)
        updateAllowed = allowed
        updates = UpdateCoordinator(driver: allowed ? SparkleDriver.make() : nil,
                                    host: HostCLI(binary: Self.hostBinary(), environment: Self.identity.hostEnvironment(candidateHome: home.map(CandidateHome.environment) ?? [:])),
                                    presenter: alerts, appPID: getpid(), version: version)
        statusItem.updates = updates
        model.updater = updates
        updates?.onChange = { [weak self] in
            self?.writeState()
            self?.model.updaterChanged()
            self?.reportPrepared()
        }
        if let made = LoginItems.make(identity: Self.identity, candidateHome: home) {
            loginItems = LoginItemReconciler(service: made.service, legacyPlist: made.legacyPlist, defaults: .standard)
        }
        updateNowObserver = DistributedNotificationCenter.default().addObserver(forName: UpdateNowWire.request, object: nil, queue: .main) { [weak self] note in
            guard let object = note.object as? String else { return }
            MainActor.assumeIsolated {
                guard let self,
                      let (reply, line) = UpdateNowWire.answer(object: object, ownBundlePath: Bundle.main.bundlePath,
                                                             decide: { self.updates?.updateNow($0) ?? HostCLI.starting }) else { return }
                FileHandle.standardError.write(Data("HTML Clay: update-now \(object.split(separator: "\n").dropFirst().first ?? ""): \(line)\n".utf8))
                DistributedNotificationCenter.default().postNotificationName(Notification.Name(reply), object: line, userInfo: nil, deliverImmediately: true)
            }
        }
        NSApp.mainMenu = MainMenu.build(settingsTarget: self, settingsAction: #selector(openSettings(_:)),
                                        openTarget: self, openAction: #selector(openAFile(_:)))
        _ = relay.subscribe { [weak self] snapshot in
            self?.writeState()
            if self?.hostReadyOnce == true { self?.loginItems?.update(wanted: snapshot.preferences.openAtLogin) }
        }
        relay.onRestart = { [model] in model?.hostRestarted() }
        relay.onRestartRequested = { [weak self] in self?.startHost() }
        notifier = HostNotifier(windows: windows, model: model)
        relay.onNotice = { [notifier] notice in notifier?.show(notice) }
        relay.onRestartToUpdate = { [weak self] in self?.updates?.restartToUpdate() }

        let welcomeDone = UserDefaults.standard.bool(forKey: WindowManager.welcomeDoneKey)
        if profile == nil, !welcomeDone, !CommandLine.arguments.contains("--no-welcome") { windows.showWelcome() }

        startHost()
    }

    /// Tells the host what Sparkle has downloaded, so its browser pages offer
    /// Update; sent again to every new host.
    private func reportPrepared() {
        let prepared = updates?.prepared ?? ""
        guard prepared != reportedPrepared else { return }
        reportedPrepared = prepared
        Task { _ = await relay.perform(.updatePrepared(prepared)) }
    }

    private func startHost() {
        if Self.identity.usesCandidateHome, home == nil { return }
        guard HostRestartPolicy.shouldStart(current: process?.state) else { return }
        let candidateEnvironment = home.map(CandidateHome.environment) ?? [:]
        let launch = HostLaunch(binary: Self.hostBinary(), environment: Self.identity.hostEnvironment(candidateHome: candidateEnvironment))
        let process = HostProcess(launch: launch)
        self.process = process
        coordinator = TerminationCoordinator(host: relay, process: process, presenter: alerts)
        process.onReady = { [weak self, weak process] _, client in
            guard let self, let process, self.process === process else { return }
            Task {
                await self.relay.attach(client)
                self.hostReadyOnce = true
                self.loginItems?.update(wanted: self.relay.last.preferences.openAtLogin)
                await self.confirmUpdate(environment: launch.environment)
                await self.updates?.start(allowed: self.updateAllowed)
                self.reportedPrepared = nil
                self.reportPrepared()
                self.opener?.hostReady()
                self.writeState()
                await self.seedProfile()
            }
        }
        process.onState = { [weak self, weak process] state in
            guard let self, let process, self.process === process else { return }
            switch state {
            case let .failed(failure):
                if failure.code == "already-running", let other = HandOff.otherRunning() {
                    HandOff.give(self.opener?.queued ?? [], to: other)
                    exit(0)
                }
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

    private func confirmUpdate(environment: [String: String]) async {
        let host = HostCLI(binary: Self.hostBinary(), environment: environment)
        do {
            let answer = try await host.confirm()
            if answer != "confirmed" {
                FileHandle.standardError.write(Data("HTML Clay: update-confirm answered \(answer)\n".utf8))
            }
        } catch {
            FileHandle.standardError.write(Data("HTML Clay: update-confirm failed: \(error)\n".utf8))
        }
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
            let ok: Bool
            if let updates {
                ok = await updates.terminate { force in await coordinator.quit(force: force) }
            } else {
                ok = await coordinator.quit()
            }
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
            "updateGate": updates?.gate ?? "",
            "sparkle": updates?.sparkle ?? "",
            "prepared": updates?.prepared ?? "",
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
