import XCTest
@testable import PreviewCore

final class HostActionWireTests: XCTestCase {
    private static let fixtures = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .appendingPathComponent("Fixtures/v1")

    private static let goArgumentNames: Set<String> = [
        "id", "ids", "path", "name", "route", "key", "value", "on", "allow", "paused", "force",
        "programID", "documentID", "revision", "answer", "username", "folderIDs", "acknowledged",
        "destination", "keep", "choice", "port", "tool", "model", "command", "confirmed", "effort",
        "templateID", "version",
    ]

    private static let everyAction: [HostAction] = [
        .setServed(folderID: "f1", on: true),
        .setServed(folderID: "f1", on: false),
        .setSynced(folderID: "f1", on: true),
        .setFraming(folderID: "f1", on: true),
        .stopTrusting(folderID: "f1"),
        .mergeIntoParent(folderID: "f1"),
        .answer(requestID: "r1", revision: 3, answer: "choose-file", path: "/tmp/resize"),
        .setSyncPaused(true),
        .setOpenAtLogin(true),
        .setAIEditing(false),
        .setAppearance(.dark),
        .setYourName("Sam"),
        .removeProgramAccess(programID: "p1", document: "d1"),
        .connectSample,
        .disconnect,
        .setUpTeamFolder(folderID: "f1"),
        .resolveConflict(id: "s1:901:1790000000000", path: "board.html", keep: "theirs"),
        .resolveRemoval(id: "0dd3adfe2b553e75", choice: "restore"),
        .setProgramAllTrusted(programID: "p1", on: true),
        .setUseName(true),
        .setAutoUpdate(false),
        .trustFolder(name: "Folder", path: "/tmp/folder"),
        .serveFolder(path: "/tmp/folder", name: "folder"),
        .addProgram(name: "Editor", path: "/tmp/Editor.app", acknowledged: ["not-executable"]),
        .relinkProgram(programID: "p1", path: "/tmp/Editor.app", acknowledged: ["just-arrived"]),
        .relinkFolder(folderID: "f1", path: "/tmp/Moved", acknowledged: ["different-folder"]),
        .askProgramAgain(programID: "p1", documentID: "/tmp/todo.htmlclay"),
        .removeProgram(programID: "p1"),
        .revealProgram(programID: "p1"),
        .startUpdate,
        .updatePrepared("2.1.0"),
        .finishUpdate,
        .restartHost,
        .restartService,
        .moveFromLocal(folderIDs: ["f1", "f2"]),
        .quitLocal,
        .markSeen(waitingIDs: ["w1"]),
        .openDocument(id: "d1"),
        .revealDocument(id: "d1"),
        .forgetDocument(id: "d1"),
        .exportDocument(id: "d1"),
        .recoverDocument(id: "d1", destination: "/tmp/recovered.html"),
        .openVersions(id: "d1"),
        .openRecoveredChanges,
        .revealLog,
        .setPort(1313),
        .setPort(1313, confirmed: true),
        .removeAllProgramAccess(programID: "p1"),
        .removeReadAccess(id: "a1"),
        .setAIEditEffort(tool: "codex", effort: "high"),
        .setProgramOn(programID: "file-search", on: true),
        .openProgramExample(programID: "file-search"),
        .setAIEditTool("claude"),
        .setAIEditModel(tool: "claude", model: "claude-opus-5-5"),
        .setAIEditCustom(command: "ollama run qwen2.5-coder:14b"),
        .checkForUpdates,
        .makeDefaultFileType,
        .setHTMLOpenWith(true),
        .stopOpeningFiles,
        .openFile(path: "/tmp/file.html"),
        .createFromTemplate(templateID: "kanban"),
        .openDashboard(route: "/folders"),
        .revealFolder(id: "f1"),
        .copyAddress(folderID: "f1"),
        .answer(requestID: "r1", revision: 3, answer: "allow"),
        .showRequest(requestID: "r1"),
        .connect(username: "sam", key: "abc123"),
        .revealSyncLog,
        .quit(force: true),
    ]

    private func goActionTypes() throws -> Set<String> {
        let data = try Data(contentsOf: Self.fixtures.appendingPathComponent("actions.json"))
        let raw = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        let actions = try XCTUnwrap(raw["actions"] as? [[String: Any]])
        return Set(try actions.map { try XCTUnwrap($0["type"] as? String) })
    }

    func testEveryGoActionHasASwiftCase() throws {
        let go = try goActionTypes()
        var swift: Set<String> = []
        for action in Self.everyAction {
            if let wire = HostActionWire.wire(action) { swift.insert(wire.type) }
        }
        XCTAssertEqual(go.subtracting(swift), [], "Go actions with no Swift case")
        XCTAssertEqual(swift.subtracting(go), [], "Swift actions the host does not know")
    }

    func testArgumentNamesGoKnows() {
        for action in Self.everyAction {
            guard let wire = HostActionWire.wire(action) else { continue }
            let unknown = Set(wire.args.keys).subtracting(Self.goArgumentNames)
            XCTAssertEqual(unknown, [], "\(wire.type) sends unknown args")
        }
    }

    func testBodies() throws {
        let served = try XCTUnwrap(HostActionWire.wire(.setServed(folderID: "f1", on: true)))
        XCTAssertEqual(String(decoding: try HostActionWire.body(id: "a1", served), as: UTF8.self),
                       #"{"args":{"id":"f1"},"id":"a1","type":"folder.serve"}"#)
        let appearance = try XCTUnwrap(HostActionWire.wire(.setAppearance(.dark)))
        XCTAssertEqual(String(decoding: try HostActionWire.body(id: "a1", appearance), as: UTF8.self),
                       #"{"args":{"key":"appearance","value":"dark"},"id":"a1","type":"prefs.set"}"#)
        let update = try XCTUnwrap(HostActionWire.wire(.startUpdate))
        XCTAssertEqual(String(decoding: try HostActionWire.body(id: "a1", update), as: UTF8.self),
                       #"{"id":"a1","type":"update.start"}"#)
    }

    func testSampleOnlyActionsDoNotMap() {
        XCTAssertNil(HostActionWire.wire(.connectSample))
        XCTAssertNil(HostActionWire.wire(.finishUpdate))
        XCTAssertNil(HostActionWire.wire(.restartHost))
    }
}
