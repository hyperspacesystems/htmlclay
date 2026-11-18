import XCTest
@testable import PreviewCore

@MainActor
final class M3ExportedContractTests: XCTestCase {
    private static let transcript = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .appendingPathComponent("Fixtures/v1/events.v1.ndjson")

    private let notesPath = "/Users/sam/notes"
    private let writingPath = "/Users/sam/writing"
    private let draftsPath = "/Users/sam/writing/drafts"
    private let allowedDocument = "/Users/sam/writing/todo.htmlclay"
    private let askingDocument = "/Users/sam/writing/drafts/essay.htmlclay"
    private let offered = [RequestAnswer.deny, RequestAnswer.allowDocument,
                           RequestAnswer.allowAllTrusted, RequestAnswer.chooseFile]

    private func wire() throws -> [WireSnapshot] {
        let data = try Data(contentsOf: Self.transcript)
        let snapshots = try data.split(separator: 0x0A).compactMap { line in
            try HostWireCoding.decoder().decode(WireEvent.self, from: Data(line)).snapshot
        }
        XCTAssertEqual(snapshots.map(\.rev), [0, 1, 3, 5, 7])
        return snapshots
    }

    private func shown() throws -> [HostSnapshot] {
        try wire().map { HostSnapshotAdapter.snapshot(from: $0) }
    }

    private func folder(_ path: String, in snapshot: HostSnapshot) throws -> Folder {
        try XCTUnwrap(snapshot.folders.first { $0.fullPath == path || $0.path == path }, path)
    }

    private func wireFolder(_ path: String, in snapshot: WireSnapshot) throws -> WireFolder {
        try XCTUnwrap(snapshot.folders.first { $0.path == path }, path)
    }

    func testARequestCarriesItsAnswers() throws {
        let sent = try wire()
        let snapshots = try shown()
        XCTAssertEqual(sent.map(\.requests.count), [0, 1, 1, 1, 0])
        XCTAssertEqual(snapshots.map(\.requests.count), [0, 1, 1, 1, 0])

        for index in 1...3 {
            let onWire = try XCTUnwrap(sent[index].requests.first)
            XCTAssertEqual(onWire.answers, offered, "rev \(sent[index].rev)")

            let request = try XCTUnwrap(snapshots[index].requests.first)
            XCTAssertEqual(request.kind, .found)
            XCTAssertEqual(request.id, onWire.id)
            XCTAssertEqual(request.revision, 1)
            XCTAssertEqual(request.program, "search")
            XCTAssertEqual(request.path, "/Users/sam/apps/bin/search")
            XCTAssertEqual(request.document, askingDocument)
            XCTAssertEqual(request.answers, offered)
            XCTAssertEqual(RequestControls.answers(request, isSample: false), offered)
            XCTAssertEqual(RequestControls.options(request, isSample: false),
                           [RequestAnswer.allowDocument, RequestAnswer.allowAllTrusted])
            XCTAssertTrue(RequestControls.showsAllTrusted(request, isSample: false))
            XCTAssertNotNil(RequestControls.choice(RequestAnswer.chooseFile, in: request, isSample: false))
            XCTAssertNil(RequestControls.choice(RequestAnswer.allowOnce, in: request, isSample: false))

            let answer = try XCTUnwrap(HostActionWire.wire(RequestFlow.action(RequestAnswer.deny, request: request)))
            XCTAssertEqual(answer.args, ["id": .string(request.id),
                                         "revision": .int(1),
                                         "answer": .string(RequestAnswer.deny)])
        }
        XCTAssertEqual(Set(snapshots[1...3].compactMap { $0.requests.first?.id }).count, 1)
    }

    func testAWaitingRequestNamesItsDocumentNotItsPath() throws {
        let snapshots = try shown()
        XCTAssertEqual(snapshots.map(\.waiting.count), [0, 1, 1, 1, 0])
        for snapshot in snapshots[1...3] {
            let request = try XCTUnwrap(snapshot.requests.first)
            let item = try XCTUnwrap(snapshot.waiting.first)
            XCTAssertEqual(item.kind, .programRequest)
            XCTAssertEqual(item.subjectID, request.id)
            XCTAssertEqual(item.title, "essay.htmlclay wants to run search")
            XCTAssertEqual(RequestControls.documentName(request), "essay.htmlclay")
            XCTAssertEqual(RequestControls.optionTitle(RequestAnswer.allowDocument, in: request),
                           Strings.Request.only("essay.htmlclay"))
            XCTAssertEqual(snapshot.count(in: .programs), 1)
        }
    }

    func testAProgramCarriesItsFacts() throws {
        let sent = try wire()
        for (index, snapshot) in try shown().enumerated() {
            XCTAssertEqual(sent[index].programs.filter { !($0.builtin ?? false) }.count, 1)
            let program = try XCTUnwrap(snapshot.programs.first { !$0.isBuiltin })
            XCTAssertEqual(program.id.count, 32)
            XCTAssertEqual(program.name, "search")
            XCTAssertEqual(program.path, "/Users/sam/apps/bin/search")
            XCTAssertFalse(program.fileMissing)
            XCTAssertEqual(program.allowedDocuments, [allowedDocument])
            XCTAssertFalse(program.allTrustedFolders)
            XCTAssertEqual(program.removedDocuments, [])
            XCTAssertEqual(ProgramControls.fileName(program), "search")
            XCTAssertEqual(program.allowedDocuments.map(ProgramControls.documentLabel), ["todo.htmlclay"])
            XCTAssertEqual(snapshot.program(program.id), program)
        }
        XCTAssertEqual(Set(sent.compactMap { $0.programs.first { !($0.builtin ?? false) }?.id }).count, 1)
    }

    func testARecentDocumentIsListed() throws {
        let sent = try wire()
        for (index, snapshot) in try shown().enumerated() {
            XCTAssertEqual(snapshot.documents.count, 1)
            let document = try XCTUnwrap(snapshot.documents.first)
            let writing = try folder(writingPath, in: snapshot)
            XCTAssertEqual(document.name, "todo.htmlclay")
            XCTAssertEqual(document.place, "~/writing")
            XCTAssertEqual(document.folderID, writing.id)
            XCTAssertFalse(document.openNow)
            XCTAssertFalse(document.fileMissing)
            XCTAssertEqual(document.url, String())
            XCTAssertNotNil(document.openedAt)
            XCTAssertEqual(document.id, sent[index].documents.first?.id)
            XCTAssertEqual(document.id.count, 16)
            XCTAssertNotNil(sent[index].documents.first?.openedAt)
        }
        XCTAssertEqual(Set(sent.compactMap { $0.documents.first?.id }).count, 1)
    }

    func testAFolderKeepsOneID() throws {
        let sent = try wire()
        var ids: [String: Set<String>] = [:]
        for snapshot in sent {
            XCTAssertEqual(snapshot.folders.map(\.path).sorted(), [notesPath, writingPath, draftsPath].sorted())
            for folder in snapshot.folders {
                XCTAssertEqual(folder.id.count, 16, folder.path)
                ids[folder.path, default: []].insert(folder.id)
            }
        }
        XCTAssertEqual(ids.count, 3)
        for (path, seen) in ids {
            XCTAssertEqual(seen.count, 1, path)
        }
        XCTAssertEqual(Set(ids.values.flatMap { $0 }).count, 3)

        XCTAssertFalse(try wireFolder(notesPath, in: sent[1]).served)
        XCTAssertTrue(try wireFolder(notesPath, in: sent[2]).served)
        let snapshots = try shown()
        XCTAssertEqual(try folder(notesPath, in: snapshots[1]).id, try folder(notesPath, in: snapshots[2]).id)
        XCTAssertEqual(try folder(notesPath, in: snapshots[2]).id, try wireFolder(notesPath, in: sent[2]).id)
    }

    func testAServedFolderSaysWhetherOtherSitesMayFrameIt() throws {
        let sent = try wire()
        let snapshots = try shown()
        XCTAssertEqual(try sent.map { try wireFolder(notesPath, in: $0).allowsFraming },
                       [false, false, false, true, true])
        XCTAssertEqual(try sent.map { try wireFolder(notesPath, in: $0).can.setFraming },
                       [false, false, true, true, true])
        XCTAssertEqual(try snapshots.map { try folder(notesPath, in: $0).allowsFraming },
                       [false, false, false, true, true])

        let served = try folder(notesPath, in: snapshots[2])
        XCTAssertTrue(served.served)
        XCTAssertEqual(FolderControls.framing(folder: served, pending: nil,
                                              hostCan: snapshots[2].hostCan(HostActionType.setFraming)),
                       FolderToggle(value: false, enabled: true))
        let framed = try folder(notesPath, in: snapshots[3])
        XCTAssertEqual(FolderControls.framing(folder: framed, pending: nil,
                                              hostCan: snapshots[3].hostCan(HostActionType.setFraming)),
                       FolderToggle(value: true, enabled: true))
        let unserved = try folder(writingPath, in: snapshots[3])
        XCTAssertFalse(FolderControls.framing(folder: unserved, pending: nil,
                                              hostCan: snapshots[3].hostCan(HostActionType.setFraming)).enabled)
        for snapshot in sent {
            XCTAssertEqual(try wireFolder(writingPath, in: snapshot).allowsFraming, false)
            XCTAssertEqual(try wireFolder(draftsPath, in: snapshot).allowsFraming, false)
        }
    }

    func testAFolderInsideATrustedFolderNamesTheFolderAroundIt() throws {
        let sent = try wire()
        for (index, snapshot) in try shown().enumerated() {
            let writing = try folder(writingPath, in: snapshot)
            let drafts = try folder(draftsPath, in: snapshot)
            let notes = try folder(notesPath, in: snapshot)
            XCTAssertEqual(try wireFolder(draftsPath, in: sent[index]).parentID, writing.id)
            XCTAssertEqual(drafts.parentID, writing.id)
            XCTAssertNil(writing.parentID)
            XCTAssertNil(notes.parentID)
            XCTAssertEqual(snapshot.children(of: writing.id).map(\.id), [drafts.id])
            XCTAssertEqual(snapshot.children(of: notes.id), [])
            XCTAssertTrue(FolderControls.showsMerge(child: drafts, hostCan: snapshot.hostCan(HostActionType.merge)))
        }
    }

    func testTheHostOffersWhatTheMilestoneBuilt() throws {
        let built: [HostAction] = [
            .answer(requestID: "r", revision: 1, answer: RequestAnswer.deny),
            .showRequest(requestID: "r"),
            .addProgram(name: "search", path: "/bin/search"),
            .relinkProgram(programID: "p", path: "/bin/search"),
            .removeProgram(programID: "p"),
            .removeProgramAccess(programID: "p", document: "d"),
            .revealProgram(programID: "p"),
            .setProgramAllTrusted(programID: "p", on: true),
            .askProgramAgain(programID: "p", documentID: "d"),
            .openDocument(id: "d"),
            .forgetDocument(id: "d"),
            .exportDocument(id: "d"),
            .recoverDocument(id: "d", destination: "/tmp/d"),
            .openVersions(id: "d"),
            .openRecoveredChanges,
            .openDashboard(route: String()),
            .relinkFolder(folderID: "f", path: "/tmp/f"),
            .setFraming(folderID: "f", on: true),
            .mergeIntoParent(folderID: "f"),
        ]
        let types = try built.map { try XCTUnwrap(HostActionWire.wire($0)).type }
        XCTAssertEqual(Set(types).count, built.count)
        let sent = try wire()
        for (index, snapshot) in try shown().enumerated() {
            for type in types {
                XCTAssertEqual(sent[index].host.can[type], true, "\(type) at rev \(snapshot.rev)")
                XCTAssertTrue(snapshot.hostCan(type), type)
            }
            XCTAssertTrue(SettingsControls.showsPane(.programs, in: snapshot))
            XCTAssertTrue(PanelControls.showsAnswer(can: snapshot.hostCan(HostActionType.requestAnswer)))
            XCTAssertTrue(PanelControls.showsExport(isSample: false, can: snapshot.hostCan(HostActionType.documentExport)))
            XCTAssertTrue(snapshot.hostCan(HostActionType.appOpenRecoveredChanges))
            XCTAssertTrue(FolderControls.canRelink(snapshot))
            XCTAssertFalse(snapshot.isSample)
        }
    }

    func testThePreviewFindsAProgramThroughTheRelinkSheet() async throws {
        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: host.current)
        model.folderChooser = SampleFolderChooser()
        let missing = try XCTUnwrap(host.current.programs.first { $0.fileMissing })
        let files = SampleData.programFiles
        let current = try XCTUnwrap(files.firstIndex(of: missing.path))
        let next = files[(current + 1) % files.count]

        ProgramFlow.chooseFile(missing, model: model)

        let deadline = Date().addingTimeInterval(2)
        while model.sheet == nil && Date() < deadline {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(model.sheet, .relinkProgram(programID: missing.id, path: next))

        let sheet = ProgramSheetModel(kind: .relink(programID: missing.id), model: model, path: next)
        await sheet.check()
        let relinked = await sheet.confirm()
        XCTAssertTrue(relinked)
        let found = try XCTUnwrap(host.current.program(missing.id))
        XCTAssertEqual(found.path, next)
        XCTAssertFalse(found.fileMissing)
    }

    func testThePreviewAllowsADocumentAgain() async throws {
        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: host.current)
        let program = try XCTUnwrap(host.current.programs.first { !$0.allowedDocuments.isEmpty && !$0.allTrustedFolders })
        let document = program.allowedDocuments[0]
        _ = await model.submit(.removeProgramAccess(programID: program.id, document: document))
        let removed = try XCTUnwrap(host.current.program(program.id))
        XCTAssertEqual(removed.removedDocuments, [document])

        let asked = await ProgramFlow.allowAgain(document, program: removed, model: model)

        XCTAssertTrue(asked)
        let allowed = try XCTUnwrap(host.current.program(program.id))
        XCTAssertEqual(allowed.removedDocuments, [])
        XCTAssertTrue(allowed.allowedDocuments.contains(document))
    }

    func testThePreviewRemovesADocumentFromRecent() async throws {
        let host = SampleHost(persona: .fiveDocuments)
        let model = AppModel(host: host, initial: host.current)
        let before = host.current.documents.map(\.id)
        let gone = try XCTUnwrap(before.first)

        let outcome = await model.submit(.forgetDocument(id: gone))

        guard case .done = outcome else { return XCTFail("the preview refused to forget a document") }
        XCTAssertEqual(host.current.documents.map(\.id), Array(before.dropFirst()))
        XCTAssertEqual(SampleReducer.apply(.forgetDocument(id: "nobody"), to: host.current), host.current)
    }
}
