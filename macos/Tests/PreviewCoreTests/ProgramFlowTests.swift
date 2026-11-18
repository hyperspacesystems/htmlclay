import XCTest
@testable import PreviewCore

@MainActor
private final class ProgramHost: HostClient {
    private(set) var recorded: [HostAction] = []
    var checkAnswer: Result<HostCheckAnswer, HostRefusal> = .success(HostCheckAnswer())
    var outcome: HostOutcome = .done(HostSnapshot())
    private var current: HostSnapshot

    init(_ snapshot: HostSnapshot = HostSnapshot()) { current = snapshot }

    func load() async throws -> HostSnapshot { current }

    func perform(_ action: HostAction) async -> HostOutcome {
        recorded.append(action)
        if case let .done(next) = outcome {
            current = next
            return .done(next)
        }
        return outcome
    }

    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { checkAnswer }

    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription { HostSubscription {} }
}

@MainActor
private final class PickOnce: FolderChooser {
    var path: String

    init(path: String) { self.path = path }

    func first(in snapshot: HostSnapshot) -> String? { path }
    func choose(after current: String?, in snapshot: HostSnapshot) async -> String? { path }
    func chooseToTrust(after current: String?, in snapshot: HostSnapshot) async -> String? { path }
    func chooseFile() async -> String? { path }
    func chooseProgram() async -> String? { path }
}

@MainActor
final class ProgramFlowTests: XCTestCase {
    private let resize = Program(id: "p1", name: "resize", path: "/tmp/bin/resize")

    override func tearDown() async throws {
        await LiveHost.stopAll()
    }

    private func snapshot(programs: [Program] = [], can: [String: Bool] = [:]) -> HostSnapshot {
        HostSnapshot(programs: programs, host: HostInfo(can: can))
    }

    private func waitUntil(_ message: String, timeout: TimeInterval = 10, _ check: () -> Bool) async {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if check() { return }
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertTrue(check(), message)
    }

    func testTheAddSheetSendsTheCodesItShowed() async throws {
        let host = ProgramHost()
        host.checkAnswer = .success(HostCheckAnswer(path: "/tmp/resize", warnings: [
            HostWarning(code: "in-temp", text: ""),
            HostWarning(code: "not-executable", text: ""),
        ]))
        let model = AppModel(host: host, initial: snapshot())
        model.folderChooser = PickOnce(path: "/tmp/resize")
        let sheet = ProgramSheetModel(kind: .add, model: model)

        let chose = await sheet.chooseAnother()
        XCTAssertTrue(chose)
        XCTAssertEqual(sheet.acknowledged, ["in-temp", "not-executable"])
        XCTAssertEqual(sheet.warningLines, [
            "It's in the temporary folder, where installers leave files.",
            "It isn't marked as runnable.",
        ])
        XCTAssertTrue(sheet.ready)

        let confirmed = await sheet.confirm()
        XCTAssertTrue(confirmed)
        XCTAssertEqual(host.recorded, [.addProgram(name: "resize", path: "/tmp/resize",
                                                  acknowledged: ["in-temp", "not-executable"])])
        let wire = try XCTUnwrap(HostActionWire.wire(host.recorded[0]))
        XCTAssertEqual(wire.type, "program.add")
        XCTAssertEqual(wire.args["acknowledged"], .strings(["in-temp", "not-executable"]))
    }

    func testAFileTheHostRefusesNeverLooksReady() async {
        let host = ProgramHost()
        host.checkAnswer = .failure(HostRefusal(code: "file-missing", text: "That file isn't there any more."))
        let model = AppModel(host: host, initial: snapshot())
        model.folderChooser = PickOnce(path: "/tmp/gone")
        let sheet = ProgramSheetModel(kind: .add, model: model)

        let chose = await sheet.chooseAnother()
        XCTAssertTrue(chose)
        XCTAssertEqual(sheet.problem, "That file isn't there any more.")
        XCTAssertFalse(sheet.ready)
        let confirmed = await sheet.confirm()
        XCTAssertFalse(confirmed, "a refused relink must keep the sheet open")
        XCTAssertTrue(host.recorded.isEmpty)
    }

    func testARefusalKeepsTheSheetOpenWithItsFile() async throws {
        let host = ProgramHost(snapshot(programs: [resize]))
        host.outcome = .refused(HostRefusal(code: "needs-confirmation", text: "That file changed. Look at it again."))
        let model = AppModel(host: host, initial: snapshot(programs: [resize]))
        model.folderChooser = PickOnce(path: "/tmp/bin/resize-2")
        let sheet = ProgramSheetModel(kind: .relink(programID: "p1"), model: model)

        XCTAssertEqual(sheet.path, "/tmp/bin/resize")
        let chose = await sheet.chooseAnother()
        XCTAssertTrue(chose)
        let confirmed = await sheet.confirm()
        XCTAssertFalse(confirmed, "a refused relink must keep the sheet open")
        XCTAssertEqual(sheet.problem, "That file changed. Look at it again.")
        XCTAssertEqual(sheet.path, "/tmp/bin/resize-2")
        XCTAssertFalse(sheet.saving)
        XCTAssertEqual(host.recorded, [.relinkProgram(programID: "p1", path: "/tmp/bin/resize-2", acknowledged: [])])
    }

    func testAllowAgainSendsTheDocumentsPath() async throws {
        let host = ProgramHost(snapshot(programs: [resize]))
        let model = AppModel(host: host, initial: snapshot(programs: [resize]))

        let asked = await ProgramFlow.allowAgain("/tmp/work/todo.htmlclay", program: resize, model: model)
        XCTAssertTrue(asked)
        XCTAssertEqual(host.recorded, [.askProgramAgain(programID: "p1", documentID: "/tmp/work/todo.htmlclay")])
        let wire = try XCTUnwrap(HostActionWire.wire(host.recorded[0]))
        XCTAssertEqual(wire.type, "program.askAgain")
        XCTAssertEqual(wire.args, ["programID": .string("p1"), "documentID": .string("/tmp/work/todo.htmlclay")])
    }

    func testRemovingAllAccessTakesEveryDocumentAndTheBroadGrant() async throws {
        let broad = Program(id: "p1", name: "resize", path: "/tmp/bin/resize",
                            allowedDocuments: ["/tmp/a.htmlclay", "/tmp/b.htmlclay"], allTrustedFolders: true)
        let host = ProgramHost(snapshot(programs: [broad]))
        let model = AppModel(host: host, initial: snapshot(programs: [broad]))

        let removed = await ProgramFlow.removeAllAccess(broad, model: model)
        XCTAssertTrue(removed)
        XCTAssertEqual(host.recorded, [.removeAllProgramAccess(programID: "p1")])
        let wire = try XCTUnwrap(HostActionWire.wire(host.recorded[0]))
        XCTAssertEqual(wire.type, "program.removeAllAccess")
        XCTAssertEqual(wire.args, ["programID": .string("p1")])
    }

    func testRemoveAllAccessShowsOnlyWhenThereIsAccessToRemove() {
        let unused = Program(id: "p1", name: "resize", path: "/tmp/bin/resize")
        let oneDocument = Program(id: "p1", name: "resize", path: "/tmp/bin/resize", allowedDocuments: ["/tmp/a.htmlclay"])
        let broad = Program(id: "p1", name: "resize", path: "/tmp/bin/resize", allTrustedFolders: true)
        let live = snapshot(programs: [unused])
        XCTAssertFalse(ProgramControls.showsRemoveAllAccess(unused, live), "Remove all access showed with nothing to remove")
        XCTAssertTrue(ProgramControls.showsRemoveAllAccess(oneDocument, live))
        XCTAssertTrue(ProgramControls.showsRemoveAllAccess(broad, live))
        XCTAssertFalse(ProgramControls.showsRemoveAllAccess(broad, snapshot(can: ["program.removeAllAccess": false])))
    }

    func testTheControlEnumsReadTheHostsCanList() {
        let nothing = snapshot(can: [
            "program.add": false, "program.relink": false, "program.askAgain": false,
            "program.remove": false, "program.removeAccess": false, "program.reveal": false,
            "program.setAllTrusted": false,
        ])
        XCTAssertFalse(ProgramControls.canAdd(nothing))
        XCTAssertFalse(ProgramControls.canRelink(nothing))
        XCTAssertFalse(ProgramControls.canAskAgain(nothing))
        XCTAssertFalse(ProgramControls.canRemove(nothing))
        XCTAssertFalse(ProgramControls.canRemoveAccess(nothing))
        XCTAssertFalse(ProgramControls.canReveal(nothing))
        XCTAssertFalse(ProgramControls.canSetAllTrusted(nothing))

        let everything = snapshot(can: ["program.add": true, "program.relink": true, "program.askAgain": true])
        XCTAssertTrue(ProgramControls.canAdd(everything))
        XCTAssertTrue(ProgramControls.canRelink(everything))
        XCTAssertTrue(ProgramControls.canAskAgain(everything))

        let sample = snapshot()
        XCTAssertTrue(ProgramControls.canAdd(sample))
        XCTAssertTrue(ProgramControls.canReveal(sample))
    }

    /// copy.md section 5: a built-in row says what it does, then who may run it.
    func testTheBuiltinSubtitleFollowsItsSwitchAndItsDocuments() {
        var program = Program(id: "file-search", name: "file-search", path: String(), isBuiltin: true)
        XCTAssertEqual(ProgramControls.builtinSubtitle(program), Strings.Programs.searchOffSub)

        program.isOn = true
        XCTAssertEqual(ProgramControls.builtinSubtitle(program),
                       Strings.Programs.searchDoes + Strings.Programs.builtinDot + Strings.Programs.noneYetOn)

        program.allowedDocuments = ["File Search.htmlclay"]
        XCTAssertEqual(ProgramControls.builtinSubtitle(program),
                       Strings.Programs.searchDoes + Strings.Programs.builtinDot + Strings.Programs.mayRun("File Search.htmlclay"))

        program.allowedDocuments = ["a.htmlclay", "b.htmlclay"]
        XCTAssertEqual(ProgramControls.builtinSubtitle(program),
                       Strings.Programs.searchDoes + Strings.Programs.builtinDot + Strings.Programs.mayRunCount(2))

        program.allTrustedFolders = true
        XCTAssertEqual(ProgramControls.builtinSubtitle(program),
                       Strings.Programs.searchDoes + Strings.Programs.builtinDot + Strings.Programs.allTrusted)

        XCTAssertEqual(ProgramControls.programTitle("file-search"), Strings.Programs.fileSearch)
        XCTAssertEqual(ProgramControls.programTitle("file-convert"), Strings.Programs.fileConverter)
        XCTAssertEqual(ProgramControls.programTitle("folder-watch"), Strings.Programs.folderWatcher)
        XCTAssertEqual(ProgramControls.programTitle("todo-search"), "todo-search")
    }

    func testABuiltinProgramRequestOffersTwoAnswers() {
        let request = ProgramRequest(id: "r1", kind: .builtinProgram, document: "/Users/sam/notes/todo.htmlclay",
                                     program: "file-search", folder: "Notes",
                                     answers: [RequestAnswer.deny, RequestAnswer.allowDocument])
        XCTAssertEqual(RequestControls.choice(RequestAnswer.deny, in: request, isSample: false)?.title,
                       Strings.Request.dontAllow)
        XCTAssertEqual(RequestControls.choice(RequestAnswer.allowDocument, in: request, isSample: false)?.title,
                       Strings.Request.allowForThisDocument)
        XCTAssertNil(RequestControls.choice(RequestAnswer.allowAllTrusted, in: request, isSample: false))
        XCTAssertNil(RequestControls.choice(RequestAnswer.chooseFile, in: request, isSample: false))
        XCTAssertEqual(RequestControls.documentName(request), "todo.htmlclay")

        let sample = ProgramRequest(id: "s1", kind: .builtinProgram, document: "todo.htmlclay",
                                    program: "file-search", folder: "Notes")
        XCTAssertEqual(RequestControls.answers(sample, isSample: true),
                       [RequestAnswer.deny, RequestAnswer.allowDocument])
    }

    func testTheWideningSwitchShowsTheNewestValue() {
        XCTAssertFalse(ProgramControls.allTrusted(resize, pending: nil))
        XCTAssertTrue(ProgramControls.allTrusted(resize, pending: .setProgramAllTrusted(programID: "p1", on: true)))
        let broad = Program(id: "p1", name: "resize", path: "/tmp/bin/resize", allTrustedFolders: true)
        XCTAssertFalse(ProgramControls.allTrusted(broad, pending: .setProgramAllTrusted(programID: "p1", on: false)))
        XCTAssertEqual(AppModel.allTrustedSlot("p1"), AppModel.slot(of: .setProgramAllTrusted(programID: "p1", on: true)))
    }

    func testADocumentTheHostListsByPathShowsByItsName() {
        XCTAssertEqual(ProgramControls.documentLabel("/tmp/work/todo.htmlclay"), "todo.htmlclay")
        XCTAssertEqual(ProgramControls.documentLabel("todo.htmlclay"), "todo.htmlclay")
        XCTAssertEqual(ProgramControls.fileName(resize), "resize")
        XCTAssertEqual(ProgramControls.fileName(Program(id: "p2", name: "clay-search", path: String())), "clay-search")
    }

    func testTheSampleSheetKeepsItsFirstFileAndCycles() async {
        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: host.current)
        let sheet = ProgramSheetModel(kind: .add, model: model)

        XCTAssertEqual(sheet.path, SampleData.programFiles[0])
        XCTAssertEqual(sheet.name, ProgramRules.defaultName(for: SampleData.programFiles[0]))
        let chose = await sheet.chooseAnother()
        XCTAssertTrue(chose)
        XCTAssertEqual(sheet.path, SampleData.programFiles[1])
        XCTAssertTrue(sheet.warningLines.isEmpty)
        XCTAssertEqual(sheet.nameProblem, .taken)
        let refused = await sheet.confirm()
        XCTAssertFalse(refused, "a taken name must keep the sheet open")

        sheet.setName("clay-resize")
        XCTAssertNil(sheet.nameProblem)
        let confirmed = await sheet.confirm()
        XCTAssertTrue(confirmed)
        XCTAssertEqual(host.current.programs.first { $0.name == "clay-resize" }?.path, SampleData.programFiles[1])
    }

    func testALiveAddedProgramAppearsAndGoesAway() async throws {
        let host = try await LiveHost.start()
        let path = try makeExecutable(in: host, named: "resize")
        let model = AppModel(host: host.client, initial: try await host.client.load())
        XCTAssertTrue(ProgramControls.userPrograms(model.snapshot).isEmpty, "the live host started with programs")

        model.folderChooser = PickOnce(path: path)
        let sheet = ProgramSheetModel(kind: .add, model: model)
        let chose = await sheet.chooseAnother()
        XCTAssertTrue(chose)
        XCTAssertFalse(sheet.acknowledged.isEmpty, "the add sheet showed no warning for a file that just arrived")
        guard await sheet.confirm() else {
            XCTFail("the live host refused the add: \(sheet.problem ?? String())")
            return
        }
        let added = try XCTUnwrap(model.snapshot.programs.first { $0.name == "resize" },
                                  "the added program never reached the snapshot")
        XCTAssertEqual(added.path, path)
        XCTAssertFalse(added.allTrustedFolders)

        guard case .done = await model.submit(.removeProgram(programID: added.id)) else {
            XCTFail("the live host refused the removal: \(model.lastRefusal?.code ?? String())")
            return
        }
        XCTAssertFalse(model.snapshot.programs.contains { $0.id == added.id }, "the removed program stayed in the snapshot")
    }

    func testALiveRelinkAcknowledgesTheWarningsTheSheetShowed() async throws {
        let host = try await LiveHost.start()
        let first = try makeExecutable(in: host, named: "resize")
        let second = try makeExecutable(in: host, named: "exports")
        let model = AppModel(host: host.client, initial: try await host.client.load())
        model.folderChooser = PickOnce(path: first)
        let addSheet = ProgramSheetModel(kind: .add, model: model)
        let chose = await addSheet.chooseAnother()
        XCTAssertTrue(chose)
        guard await addSheet.confirm() else {
            XCTFail("the live host refused the first add: \(addSheet.problem ?? String())")
            return
        }
        let program = try XCTUnwrap(model.snapshot.programs.first { $0.name == "resize" })

        model.folderChooser = PickOnce(path: second)
        let sheet = ProgramSheetModel(kind: .relink(programID: program.id), model: model)
        XCTAssertEqual(sheet.path, first)
        let picked = await sheet.chooseAnother()
        XCTAssertTrue(picked)
        XCTAssertFalse(sheet.acknowledged.isEmpty, "the relink sheet showed no warning for a file that just arrived")
        guard await sheet.confirm() else {
            XCTFail("the live host refused the relink: \(sheet.problem ?? String())")
            return
        }
        XCTAssertEqual(model.snapshot.program(program.id)?.path, second, "the relink did not take")
    }

    private func makeExecutable(in host: LiveHost, named name: String) throws -> String {
        let folder = host.home.appendingPathComponent("bin")
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        let path = folder.appendingPathComponent(name)
        try Data("#!/bin/sh\nexit 0\n".utf8).write(to: path)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: path.path)
        return path.path
    }
}
