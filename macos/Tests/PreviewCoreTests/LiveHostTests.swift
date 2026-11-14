import XCTest
@testable import PreviewCore
import Darwin

private enum LiveHostFailure: Error, CustomStringConvertible {
    case start(String)

    var description: String {
        switch self {
        case let .start(message): return message
        }
    }
}

@MainActor
private final class FixedFolderChooser: FolderChooser {
    private let path: String?

    init(_ path: String?) { self.path = path }

    func first(in snapshot: HostSnapshot) -> String? { path }

    func choose(after current: String?, in snapshot: HostSnapshot) async -> String? { path }

    func chooseToTrust(after current: String?, in snapshot: HostSnapshot) async -> String? { path }

    func chooseFile() async -> String? { path }
}

@MainActor
private final class ScriptedQuitPresenter: QuitPresenter {
    private(set) var asked: [[String]] = []
    private var answers: [QuitChoice]

    init(_ answers: [QuitChoice]) { self.answers = answers }

    func askAboutUnsaved(_ lines: [String]) async -> QuitChoice {
        asked.append(lines)
        return answers.isEmpty ? .cancel : answers.removeFirst()
    }
}

@MainActor
private final class LiveHost {
    let home: URL
    let base: URL
    let ready: HostReady
    let client: LoopbackHostClient
    let shellPort: Int
    let process: HostProcess

    private static let requireVariable = "HTMLCLAY_REQUIRE_LIVE_HOST"
    private static var running: [LiveHost] = []

    static var binary: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent(".build/host-for-tests/htmlclay")
    }

    static var stubBinary: URL {
        binary.deletingLastPathComponent().appendingPathComponent("stubservice")
    }

    static func start(environment extra: [String: String] = [:]) async throws -> LiveHost {
        let binary = Self.binary
        guard FileManager.default.isExecutableFile(atPath: binary.path) else {
            if ProcessInfo.processInfo.environment[Self.requireVariable] == "1" {
                throw LiveHostFailure.start("the live host is missing: run scripts/build-host-for-tests.sh")
            }
            throw XCTSkip("run scripts/build-host-for-tests.sh to build \(binary.path)")
        }

        let homePath = try Self.makeHome()
        let basePath = homePath + "/c"
        let configDir = basePath + "/htmlclay"
        try FileManager.default.createDirectory(atPath: configDir, withIntermediateDirectories: true)
        let port = try Self.freeLoopbackPort()
        let config = "{\"startOnLogin\": false, \"port\": \(port), \"updates\": \"off\"}\n"
        try Data(config.utf8).write(to: URL(fileURLWithPath: configDir + "/config.json"))

        var environment = [
            "HOME": homePath,
            "HTMLCLAY_CONFIG_BASE": basePath,
            "HTMLCLAY_OPEN_LOG": homePath + "/open.log",
        ]
        for (key, value) in extra { environment[key] = value }
        let process = HostProcess(launch: HostLaunch(binary: binary, environment: environment))
        var handed: (ready: HostReady, client: LoopbackHostClient)?
        process.onReady = { ready, client in handed = (ready, client) }

        let state = await process.start()
        guard case let .running(ready) = state, let handed else {
            _ = await process.stop(timeout: 5)
            try? FileManager.default.removeItem(atPath: homePath)
            throw LiveHostFailure.start("no READY line from \(binary.path): \(state); stderr:\n\(process.stderrTail)")
        }
        guard let shellPort = URL(string: ready.shell)?.port else {
            _ = await process.stop(timeout: 5)
            try? FileManager.default.removeItem(atPath: homePath)
            throw LiveHostFailure.start("READY = \(ready.shell), wanted a shell on 127.0.0.1")
        }

        let host = LiveHost(home: URL(fileURLWithPath: homePath),
                            base: URL(fileURLWithPath: basePath),
                            ready: ready,
                            client: handed.client,
                            shellPort: shellPort,
                            process: process)
        running.append(host)
        return host
    }

    private init(home: URL, base: URL, ready: HostReady, client: LoopbackHostClient, shellPort: Int, process: HostProcess) {
        self.home = home
        self.base = base
        self.ready = ready
        self.client = client
        self.shellPort = shellPort
        self.process = process
    }

    var secret: String { process.currentSecret ?? "" }

    @discardableResult
    func stop() async -> Bool {
        let exited = await process.stop(timeout: 10)
        try? FileManager.default.removeItem(at: home)
        return exited
    }

    static func stopAll() async {
        let hosts = running
        running = []
        for host in hosts { await host.stop() }
    }

    private static func makeHome() throws -> String {
        var template = Array("/tmp/hc-XXXXXXXX".utf8CString)
        let made = template.withUnsafeMutableBufferPointer { buffer -> String? in
            guard let path = mkdtemp(buffer.baseAddress!) else { return nil }
            return String(cString: path)
        }
        guard let made else { throw LiveHostFailure.start("mkdtemp failed: \(errno)") }
        guard let resolved = made.withCString({ realpath($0, nil) }) else {
            try? FileManager.default.removeItem(atPath: made)
            throw LiveHostFailure.start("realpath failed for \(made): \(errno)")
        }
        defer { free(resolved) }
        return String(cString: resolved)
    }

    private static func freeLoopbackPort() throws -> Int {
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        guard fd >= 0 else { throw LiveHostFailure.start("socket: \(errno)") }
        defer { close(fd) }
        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = 0
        address.sin_addr.s_addr = inet_addr("127.0.0.1")
        let bound = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        guard bound == 0 else { throw LiveHostFailure.start("bind: \(errno)") }
        var actual = sockaddr_in()
        var length = socklen_t(MemoryLayout<sockaddr_in>.size)
        let named = withUnsafeMutablePointer(to: &actual) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                getsockname(fd, $0, &length)
            }
        }
        guard named == 0 else { throw LiveHostFailure.start("getsockname: \(errno)") }
        return Int(UInt16(bigEndian: actual.sin_port))
    }
}

@MainActor
final class LiveHostTests: XCTestCase {
    override func tearDown() async throws {
        await LiveHost.stopAll()
    }

    private func makeFolder(_ url: URL) throws {
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        try Data("<!doctype html><title>\(url.lastPathComponent)</title><p>hello".utf8)
            .write(to: url.appendingPathComponent("index.html"))
    }

    private func waitUntil(_ message: String, timeout: TimeInterval = 5, _ check: () -> Bool) async {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if check() { return }
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertTrue(check(), message)
    }

    private func openLog(_ url: URL) -> [String] {
        guard let text = try? String(contentsOf: url, encoding: .utf8) else { return [] }
        return text.split(separator: "\n").map(String.init)
    }

    private func done(_ outcome: HostOutcome, _ message: String) throws -> HostSnapshot {
        guard case let .done(snapshot) = outcome else {
            XCTFail(message)
            throw LiveHostFailure.start(message)
        }
        return snapshot
    }

    private struct FileState: Equatable {
        var bytes: Data
        var modified: Date?
    }

    private func fileState(_ url: URL) throws -> FileState? {
        guard FileManager.default.fileExists(atPath: url.path) else { return nil }
        let attributes = try FileManager.default.attributesOfItem(atPath: url.path)
        return FileState(bytes: try Data(contentsOf: url), modified: attributes[.modificationDate] as? Date)
    }

    func testLoadShowsTheRealHost() async throws {
        let host = try await LiveHost.start()

        let snapshot = try await host.client.load()
        XCTAssertEqual(snapshot.epoch, host.ready.epoch)
        XCTAssertEqual(snapshot.host.can["folder.trust"], true)
        XCTAssertEqual(snapshot.folders, [])
    }

    func testTrustServeAndStopThroughTheStream() async throws {
        let host = try await LiveHost.start()
        let notes = host.home.appendingPathComponent("notes")
        try makeFolder(notes)

        let witness = LoopbackHostClient(port: host.shellPort, secret: host.secret)
        var seen: [HostSnapshot] = []
        let subscription = host.client.subscribe { seen.append($0) }
        defer { subscription.cancel() }

        let trustedSnapshot = try done(await witness.perform(.trustFolder(name: "notes", path: notes.path)),
                                       "trusting notes was refused")
        let trusted = try XCTUnwrap(trustedSnapshot.folders.first { $0.fullPath == notes.path })
        XCTAssertEqual(trusted.fullPath, notes.path)

        let servedSnapshot = try done(await witness.perform(.serveFolder(path: notes.path, name: "notes")),
                                      "serving notes was refused")
        let served = try XCTUnwrap(servedSnapshot.folders.first { $0.fullPath == notes.path })
        XCTAssertTrue(served.served)
        XCTAssertTrue(served.address.contains("notes.htmlclay.localhost"))

        await waitUntil("the live stream never showed notes served") {
            seen.contains { $0.folders.first { $0.fullPath == notes.path }?.served == true }
        }

        let stoppedSnapshot = try done(await witness.perform(.setServed(folderID: served.id, on: false)),
                                       "stopping notes was refused")
        XCTAssertEqual(stoppedSnapshot.folders.first { $0.id == served.id }?.served, false)

        let revisions = seen.map(\.rev)
        XCTAssertEqual(revisions, revisions.sorted())
    }

    func testRefusalComesBackTyped() async throws {
        let host = try await LiveHost.start()
        let notes = host.home.appendingPathComponent("notes")
        let blog = host.home.appendingPathComponent("blog")
        try makeFolder(notes)
        try makeFolder(blog)

        _ = try done(await host.client.perform(.serveFolder(path: notes.path, name: "notes")),
                     "serving notes was refused")

        guard case let .refused(refusal) = await host.client.perform(.serveFolder(path: blog.path, name: "notes")) else {
            XCTFail("expected a refusal for the taken name")
            return
        }
        XCTAssertEqual(refusal.code, "name-taken")
        XCTAssertFalse(refusal.text.isEmpty)
    }

    func testCheckAnswersTheTrustSheet() async throws {
        let host = try await LiveHost.start()
        let notes = host.home.appendingPathComponent("notes")
        try makeFolder(notes)

        guard case let .failure(refusal) = await host.client.check(.trustFolder(name: "", path: host.home.path)) else {
            XCTFail("expected the home folder to be refused")
            return
        }
        XCTAssertEqual(refusal.code, "cannot-trust")

        guard case let .success(answer) = await host.client.check(.trustFolder(name: "notes", path: notes.path)) else {
            XCTFail("expected notes to be checkable")
            return
        }
        XCTAssertEqual(answer.path, notes.path)
    }

    func testTheServeSheetAsksTheHostAndKeepsARefusal() async throws {
        let host = try await LiveHost.start()
        let notes = host.home.appendingPathComponent("notes")
        let blog = host.home.appendingPathComponent("blog")
        try makeFolder(notes)
        try makeFolder(blog)

        let model = AppModel(host: host.client, initial: try await host.client.load())
        model.folderChooser = FixedFolderChooser(notes.path)

        let first = ServeSheetModel(folderID: nil, model: model)
        XCTAssertEqual(first.path, notes.path)
        XCTAssertEqual(first.name, "notes")
        await first.check()
        XCTAssertTrue(first.address.contains("notes.htmlclay.localhost"), "address = \(first.address)")
        XCTAssertNil(first.pathProblem)
        XCTAssertNil(first.nameProblem)
        let firstServed = await first.serve()
        XCTAssertTrue(firstServed)
        let served = try XCTUnwrap(model.snapshot.folders.first { $0.fullPath == notes.path })
        XCTAssertTrue(served.served)

        model.folderChooser = FixedFolderChooser(blog.path)
        let second = ServeSheetModel(folderID: nil, model: model)
        XCTAssertEqual(second.path, blog.path)
        second.name = "notes"
        await second.check()
        XCTAssertNotNil(second.nameProblem, "the host said nothing about the taken name")
        XCTAssertNil(second.pathProblem)
        let secondServed = await second.serve()
        XCTAssertFalse(secondServed)
        XCTAssertEqual(second.name, "notes")

        _ = try done(await model.submit(.setServed(folderID: served.id, on: false)),
                     "stopping notes was refused")
        XCTAssertEqual(model.snapshot.folders.first { $0.id == served.id }?.served, false)

        let known = ServeSheetModel(folderID: served.id, model: model)
        XCTAssertFalse(known.nameEditable)
        XCTAssertEqual(known.name, "notes")
        let resumedServe = await known.serve()
        XCTAssertTrue(resumedServe)
        let resumed = try XCTUnwrap(model.snapshot.folders.first { $0.id == served.id })
        XCTAssertTrue(resumed.served)
        XCTAssertEqual(resumed.address, served.address)
        XCTAssertTrue(resumed.address.contains("notes.htmlclay.localhost"), "address = \(resumed.address)")
    }

    func testTheTrustSheetRefusesTheHomeFolder() async throws {
        let host = try await LiveHost.start()
        let model = AppModel(host: host.client, initial: try await host.client.load())

        let sheet = TrustSheetModel(name: host.home.lastPathComponent, path: host.home.path, model: model)
        await sheet.check()

        XCTAssertNotNil(sheet.problem)
        XCTAssertFalse(sheet.ready)
        let trusted = await sheet.trust()
        XCTAssertFalse(trusted)
        XCTAssertTrue(model.snapshot.folders.isEmpty, "the home folder was trusted anyway")
    }

    func testTheTrustSheetKnowsAPersonalFolderAndWhatIsInsideIt() async throws {
        let host = try await LiveHost.start()
        let documents = host.home.appendingPathComponent("Documents")
        let outer = host.home.appendingPathComponent("outer")
        let inner = outer.appendingPathComponent("inner")
        try makeFolder(documents)
        try makeFolder(inner)

        let model = AppModel(host: host.client, initial: try await host.client.load())
        model.folderChooser = FixedFolderChooser(inner.path)

        let personal = TrustSheetModel(name: "Documents", path: documents.path, model: model)
        await personal.check()
        XCTAssertTrue(personal.isMainFolder, "the host did not call Documents a personal folder")
        XCTAssertNil(personal.problem)

        _ = try done(await model.submit(.trustFolder(name: "inner", path: inner.path)),
                     "trusting inner was refused")

        let sheet = TrustSheetModel(name: "outer", path: outer.path, model: model)
        await sheet.check()
        XCTAssertEqual(sheet.insideNames, ["inner"])
        XCTAssertFalse(sheet.isMainFolder)

        let trusted = await sheet.trust()
        XCTAssertTrue(trusted)
        let trustedOuter = try XCTUnwrap(model.snapshot.folders.first { $0.fullPath == outer.path })
        XCTAssertEqual(trustedOuter.name, "outer")
    }

    func testRevealAndCopyAnswerAndLandInTheOpenLog() async throws {
        let host = try await LiveHost.start()
        let notes = host.home.appendingPathComponent("notes")
        try makeFolder(notes)

        _ = try done(await host.client.perform(.trustFolder(name: "notes", path: notes.path)),
                     "trusting notes was refused")
        let servedSnapshot = try done(await host.client.perform(.serveFolder(path: notes.path, name: "notes")),
                                      "serving notes was refused")
        let served = try XCTUnwrap(servedSnapshot.folders.first { $0.fullPath == notes.path })

        _ = try done(await host.client.perform(.revealFolder(id: served.id)),
                     "revealing notes was refused")
        _ = try done(await host.client.perform(.copyAddress(folderID: served.id)),
                     "copying the address of notes was refused")

        let log = host.home.appendingPathComponent("open.log")
        await waitUntil("the open log never got the reveal and the copy") {
            guard let text = try? String(contentsOf: log, encoding: .utf8) else { return false }
            return text.contains("reveal ") && text.contains("copy http")
        }
    }

    func testThePanelOpensFilesDocumentsAndTheDashboardOnTheRealHost() async throws {
        let host = try await LiveHost.start()
        let notes = host.home.appendingPathComponent("notes")
        try FileManager.default.createDirectory(at: notes, withIntermediateDirectories: true)
        let file = notes.appendingPathComponent("a.htmlclay")
        try Data("<!doctype html><title>a</title><p>hello".utf8).write(to: file)

        let opened = try done(await host.client.perform(.openFile(path: file.path)),
                              "opening a.htmlclay was refused")
        let document = try XCTUnwrap(opened.documents.first { $0.name == "a.htmlclay" },
                                     "the snapshot does not list a.htmlclay")

        let log = host.home.appendingPathComponent("open.log")
        await waitUntil("the open log never got a URL naming a.htmlclay") {
            openLog(log).contains { $0.contains("a.htmlclay") }
        }

        // The panel shows Open on the real host and hides what it cannot do.
        let model = AppModel(host: host.client, initial: opened)
        XCTAssertFalse(model.isSample)
        XCTAssertTrue(PanelControls.showsDocumentAction(HostActionType.documentOpen, in: model.snapshot))
        XCTAssertTrue(PanelControls.showsDocumentAction(HostActionType.documentReveal, in: model.snapshot))
        XCTAssertFalse(PanelControls.showsExport(isSample: model.isSample))

        _ = try done(await host.client.perform(.revealDocument(id: document.id)),
                     "showing the document in Finder was refused")
        let revealed = file.resolvingSymlinksInPath().path
        await waitUntil("the open log never got a reveal line for a.htmlclay") {
            openLog(log).contains { $0 == "reveal " + revealed || ($0.hasPrefix("reveal ") && $0.hasSuffix("/notes/a.htmlclay")) }
        }

        _ = try done(await host.client.perform(.openDocument(id: document.id)),
                     "opening the document was refused")
        await waitUntil("the open log did not get a second URL naming a.htmlclay") {
            openLog(log).filter { $0.contains("a.htmlclay") }.count >= 2
        }

        _ = try done(await host.client.perform(.openDashboard(route: String())),
                     "opening the dashboard was refused")
        await waitUntil("the open log never got the dashboard") {
            openLog(log).contains { $0.contains("dashboard.htmlclay.localhost") }
        }

        // Forgetting has no host effect yet, so the host says not-yet and the panel
        // hides Remove from Recent there.
        guard case let .refused(forgetRefusal) = await host.client.perform(.forgetDocument(id: document.id)) else {
            return XCTFail("the host forgot the document without an effect for it")
        }
        XCTAssertEqual(forgetRefusal.code, "not-yet")
        XCTAssertFalse(model.can(HostActionType.documentForget))
        let next = try done(await host.client.perform(.setAIEditing(false)),
                            "asking the host for a fresh snapshot was refused")
        XCTAssertTrue(next.documents.contains { $0.id == document.id },
                      "the document left Recent with no effect to forget it")
    }

    func testTheSettingsLandOnTheRealHost() async throws {
        let host = try await LiveHost.start()

        let dark = try done(await host.client.perform(.setAppearance(.dark)),
                            "setting the appearance was refused")
        XCTAssertEqual(dark.preferences.appearance, .dark)
        let named = try done(await host.client.perform(.setAIEditing(false)),
                             "turning AI editing off was refused")
        XCTAssertFalse(named.preferences.aiEditing)
        _ = try done(await host.client.perform(.setYourName("Ada")),
                     "setting the name was refused")
        let sharing = try done(await host.client.perform(.setUseName(true)),
                               "turning the name on was refused")
        XCTAssertEqual(sharing.preferences.yourName, "Ada")
        XCTAssertTrue(sharing.preferences.useName)

        guard case let .refused(loginRefusal) = await host.client.perform(.setOpenAtLogin(true)) else {
            return XCTFail("the host wrote the login item the Mac app owns")
        }
        XCTAssertEqual(loginRefusal.code, "not-yet")
        let model = AppModel(host: host.client, initial: sharing)
        XCTAssertFalse(SettingsControls.showsOpenAtLogin(isSample: model.isSample))

        let reloaded = try await host.client.load()
        XCTAssertEqual(reloaded.preferences.appearance, .dark)
        XCTAssertEqual(reloaded.preferences.yourName, "Ada")
        XCTAssertTrue(reloaded.preferences.useName)
        XCTAssertFalse(reloaded.preferences.aiEditing)
    }

    func testAFolderInsideAServedFolderFollowsTheHostsSwitch() async throws {
        let host = try await LiveHost.start()
        let outer = host.home.appendingPathComponent("notes")
        let inner = outer.appendingPathComponent("inner")
        try makeFolder(outer)
        try makeFolder(inner)

        _ = try done(await host.client.perform(.trustFolder(name: "notes", path: outer.path)),
                     "trusting notes was refused")
        _ = try done(await host.client.perform(.serveFolder(path: outer.path, name: "notes")),
                     "serving notes was refused")
        let trusted = try done(await host.client.perform(.trustFolder(name: "inner", path: inner.path)),
                               "trusting inner was refused")
        let inside = try XCTUnwrap(trusted.folders.first { $0.fullPath == inner.path })
        XCTAssertFalse(inside.served)

        // The host's can is what the page's switch shows and obeys, here and now.
        let toggle = FolderControls.served(folder: inside, pending: nil)
        XCTAssertEqual(toggle.value, inside.served)
        XCTAssertEqual(toggle.enabled, inside.can?.serve ?? true)
        XCTAssertEqual(toggle.note, (inside.can?.serve ?? true) ? nil : inside.why.serve?.text)

        // The switch already says what serving would be told: it is off, with the reason.
        XCTAssertFalse(inside.can?.serve ?? true, "the host offered to serve a folder inside a served one")
        XCTAssertFalse(toggle.enabled)
        XCTAssertEqual(inside.why.serve?.code, "overlaps-served")
        XCTAssertFalse((toggle.note ?? "").isEmpty, "the served switch has no note")

        // Serving it is still refused when it is asked for.
        guard case let .refused(refusal) = await host.client.perform(.serveFolder(path: inner.path, name: "inner")) else {
            return XCTFail("the host served a folder inside a served folder")
        }
        XCTAssertEqual(refusal.code, "overlaps-served")
    }

    func testAMissingFolderDisablesItsServedSwitchWithTheHostsReason() async throws {
        let host = try await LiveHost.start()
        let notes = host.home.appendingPathComponent("notes")
        try makeFolder(notes)

        let trusted = try done(await host.client.perform(.trustFolder(name: "notes", path: notes.path)),
                               "trusting notes was refused")
        let folder = try XCTUnwrap(trusted.folders.first { $0.fullPath == notes.path })
        try FileManager.default.removeItem(at: notes)

        let next = try done(await host.client.perform(.setAIEditing(false)),
                            "asking the host for a fresh snapshot was refused")
        let gone = try XCTUnwrap(next.folders.first { $0.fullPath == notes.path })
        XCTAssertTrue(gone.missingOnDisk)

        let toggle = FolderControls.served(folder: gone, pending: nil)
        XCTAssertFalse(toggle.enabled)
        XCTAssertEqual(toggle.value, false)
        let why = try XCTUnwrap(gone.why.serve)
        XCTAssertFalse(why.text.isEmpty)
        XCTAssertEqual(toggle.note, why.text)
        XCTAssertEqual(folder.id, gone.id)
    }

    func testAFileOpenerOpensInHomeAndReportsRefusals() async throws {
        let host = try await LiveHost.start()
        let notes = host.home.appendingPathComponent("notes")
        try makeFolder(notes)
        let document = notes.appendingPathComponent("a.htmlclay")
        try Data("<!doctype html><title>a</title><p>hello".utf8).write(to: document)
        let log = host.home.appendingPathComponent("open.log")

        let model = AppModel(host: host.client, initial: try await host.client.load())
        let opener = FileOpener(model: model)
        var refusals: [(String, HostRefusal)] = []
        opener.onRefused = { refusals.append(($0, $1)) }
        opener.hostReady()

        opener.open([document.path])
        await waitUntil("the open log never named a.htmlclay") {
            self.openLog(log).contains { $0.contains("a.htmlclay") }
        }

        let outside = FileManager.default.temporaryDirectory.appendingPathComponent("hc-outside-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: outside, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: outside) }
        let stranger = outside.appendingPathComponent("outside.htmlclay")
        try Data("<!doctype html><title>outside</title>".utf8).write(to: stranger)

        opener.open([stranger.path])
        await waitUntil("the file outside home was never refused") { refusals.count == 1 }
        XCTAssertEqual(refusals.first?.0, "outside.htmlclay")
        XCTAssertEqual(refusals.first?.1.code, "file-outside-home")
        XCTAssertFalse(refusals.first?.1.text.isEmpty ?? true, "the refusal to open a file outside home has no text")

        opener.open([notes.appendingPathComponent("missing.htmlclay").path])
        await waitUntil("the missing file was never refused") { refusals.count == 2 }
        XCTAssertEqual(refusals.last?.0, "missing.htmlclay")
        XCTAssertEqual(refusals.last?.1.code, "file-missing")
    }

    func testWrongSecretIsRefused() async throws {
        let host = try await LiveHost.start()
        let stranger = LoopbackHostClient(port: host.shellPort, secret: String(repeating: "0", count: 64))

        guard case let .refused(refusal) = await stranger.perform(.setAIEditing(false)) else {
            XCTFail("expected the wrong secret to be refused")
            return
        }
        XCTAssertEqual(refusal.code, "host-error")

        _ = try done(await host.client.perform(.setAIEditing(false)),
                     "the right secret stopped working after the wrong one")
    }

    func testClosingStdinEndsTheStream() async throws {
        let host = try await LiveHost.start()

        var endings: [String] = []
        var seen = 0
        host.client.onEnded = { endings.append($0) }
        let subscription = host.client.subscribe { _ in seen += 1 }
        defer { subscription.cancel() }

        await waitUntil("the stream never delivered a snapshot after load") { seen > 0 }
        let exited = await host.stop()

        await waitUntil("onEnded never heard quit", timeout: 10) { !endings.isEmpty }
        XCTAssertEqual(endings.first, "quit")
        XCTAssertTrue(exited, "the host did not exit after stdin closed")
    }

    func testNothingWrittenOutsideTheTestHome() async throws {
        let real = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support/htmlclay/config.json")
        let before = try fileState(real)

        let host = try await LiveHost.start()
        let notes = host.home.appendingPathComponent("notes")
        try makeFolder(notes)

        _ = try done(await host.client.perform(.trustFolder(name: "notes", path: notes.path)),
                     "trusting notes was refused")
        let servedSnapshot = try done(await host.client.perform(.serveFolder(path: notes.path, name: "notes")),
                                      "serving notes was refused")
        let served = try XCTUnwrap(servedSnapshot.folders.first { $0.fullPath == notes.path })
        _ = try done(await host.client.perform(.setServed(folderID: served.id, on: false)),
                     "stopping notes was refused")

        let after = try fileState(real)
        XCTAssertEqual(before, after)
    }

    private func waitForService(_ host: LiveHost, _ message: String, timeout: TimeInterval = 30) async {
        let deadline = Date().addingTimeInterval(timeout)
        var state = ""
        while Date() < deadline {
            state = (try? await host.client.load())?.host.service ?? ""
            if state == "running" { return }
            try? await Task.sleep(nanoseconds: 50_000_000)
        }
        XCTFail("\(message) (host.service = \(state))")
    }

    private func stubService() throws -> URL {
        let stub = LiveHost.stubBinary
        guard FileManager.default.isExecutableFile(atPath: stub.path) else {
            if ProcessInfo.processInfo.environment["HTMLCLAY_REQUIRE_LIVE_HOST"] == "1" {
                throw LiveHostFailure.start("the stub service is missing: run scripts/build-host-for-tests.sh")
            }
            throw XCTSkip("run scripts/build-host-for-tests.sh to build \(stub.path)")
        }
        return stub
    }

    private func holdFile() throws -> URL {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("hc-hold-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        addTeardownBlock { try? FileManager.default.removeItem(at: dir) }
        return dir.appendingPathComponent("hold")
    }

    private func startServingNotes(_ stub: URL, hold: URL) async throws -> LiveHost {
        let host = try await LiveHost.start(environment: [
            "HTMLCLAY_SERVICE_CMD": stub.path,
            "HTMLCLAY_STUB_HOLD": hold.path,
        ])
        let notes = host.home.appendingPathComponent("notes")
        try makeFolder(notes)
        _ = try done(await host.client.perform(.trustFolder(name: "notes", path: notes.path)),
                     "trusting notes was refused")
        _ = try done(await host.client.perform(.serveFolder(path: notes.path, name: "notes")),
                     "serving notes was refused")
        await waitForService(host, "the stub service never started")
        return host
    }

    func testQuitAsksBeforeLosingEditsInAServedFolder() async throws {
        let stub = try stubService()
        let hold = try holdFile()
        try Data().write(to: hold)

        let host = try await startServingNotes(stub, hold: hold)

        let cancel = ScriptedQuitPresenter([.cancel])
        let keeper = TerminationCoordinator(host: host.client, process: host.process, presenter: cancel)
        let kept = await keeper.quit()

        XCTAssertFalse(kept, "Quit was allowed while a served folder had unsaved edits")
        let named = cancel.asked.first ?? []
        XCTAssertTrue(named.contains { $0.contains("unsaved.htmlclay") },
                      "the unsaved-edits dialog did not name the document: \(named)")
        XCTAssertTrue(host.process.isRunning, "Cancel stopped the host")

        let anyway = ScriptedQuitPresenter([.quitAnyway])
        let leaver = TerminationCoordinator(host: host.client, process: host.process, presenter: anyway)
        let left = await leaver.quit()

        XCTAssertTrue(left, "Quit Anyway did not let the app quit")
        XCTAssertNil(host.process.pid, "quit() answered true while the host was still running")

        let fresh = try await startServingNotes(stub, hold: try holdFile())
        let quiet = ScriptedQuitPresenter([.cancel])
        let plain = TerminationCoordinator(host: fresh.client, process: fresh.process, presenter: quiet)
        let quit = await plain.quit()

        XCTAssertTrue(quit, "Quit with nothing unsaved was refused")
        XCTAssertEqual(quiet.asked, [], "the person was asked with nothing unsaved")
        XCTAssertNil(fresh.process.pid, "quit() answered true while the host was still running")
    }
}
