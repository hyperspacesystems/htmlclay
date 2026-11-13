import XCTest
@testable import PreviewCore

@MainActor
final class CoreTests: XCTestCase {
    func testEveryPersonaCountsAddUp() {
        for persona in Persona.allCases {
            let snapshot = SampleData.snapshot(for: persona)
            let sum = SettingsPane.allCases.map { snapshot.count(in: $0) }.reduce(0, +)
            XCTAssertEqual(sum, snapshot.totalCount, persona.rawValue)
        }
    }

    func testSyncedWithTeamsCountsMatchTheDesign() {
        let s = SampleData.snapshot(for: .syncedWithTeams)
        XCTAssertEqual(s.count(in: .folders), 2)
        XCTAssertEqual(s.count(in: .programs), 1)
        XCTAssertEqual(s.totalCount, 3)
    }

    func testPersonaShapes() {
        XCTAssertEqual(SampleData.snapshot(for: .newPerson), HostSnapshot())
        let five = SampleData.snapshot(for: .fiveDocuments)
        XCTAssertEqual(five.documents.count, 5)
        XCTAssertEqual(five.documents.filter(\.fileMissing).count, 1)
        XCTAssertTrue(five.folders.isEmpty)
        let trusted = SampleData.snapshot(for: .trustedFolder)
        XCTAssertEqual(trusted.children(of: "notes").count, 2)
        XCTAssertEqual(trusted.count(in: .programs), 1)
        let served = SampleData.snapshot(for: .servedFolder)
        XCTAssertFalse(served.syncConnected)
        XCTAssertTrue(served.folders.contains { $0.served && !$0.synced })
        XCTAssertTrue(SampleData.snapshot(for: .localUser).localMoveOffer)
    }

    func testIDsAreUniqueAndReferencesResolve() {
        for persona in Persona.allCases {
            let s = SampleData.snapshot(for: persona)
            XCTAssertEqual(Set(s.folders.map(\.id)).count, s.folders.count, persona.rawValue)
            XCTAssertEqual(Set(s.waiting.map(\.id)).count, s.waiting.count, persona.rawValue)
            for f in s.folders { if let p = f.parentID { XCTAssertNotNil(s.folder(p), persona.rawValue) } }
            for f in s.folders where f.synced { XCTAssertTrue(f.served && f.origin.canSync, f.id) }
            for r in s.requests { XCTAssertTrue(s.waiting.contains { $0.subjectID == r.id }, r.id) }
        }
    }

    func testSnapshotsRoundTripAsJSON() throws {
        for persona in Persona.allCases {
            let snapshot = SampleData.snapshot(for: persona)
            let data = try JSONEncoder().encode(snapshot)
            XCTAssertEqual(try JSONDecoder().decode(HostSnapshot.self, from: data), snapshot)
        }
    }

    func testStopServingAlsoStopsSync() {
        let before = SampleData.snapshot(for: .syncedWithTeams)
        let after = SampleReducer.apply(.setServed(folderID: "panphora", on: false), to: before)
        XCTAssertEqual(after.folder("panphora")?.served, false)
        XCTAssertEqual(after.folder("panphora")?.synced, false)
    }

    func testSyncOnlyForServedAccountFolders() {
        let s = SampleData.snapshot(for: .syncedWithTeams)
        XCTAssertEqual(SampleReducer.apply(.setSynced(folderID: "prototypes", on: true), to: s), s)
        let off = SampleReducer.apply(.setServed(folderID: "panphora", on: false), to: s)
        XCTAssertEqual(SampleReducer.apply(.setSynced(folderID: "panphora", on: true), to: off), off)
    }

    func testStopTrustingRemovesFolderAndItsWaitingItemsButKeepsChildren() {
        let s = SampleData.snapshot(for: .syncedWithTeams)
        let after = SampleReducer.apply(.stopTrusting(folderID: "design-team"), to: s)
        XCTAssertNil(after.folder("design-team"))
        XCTAssertEqual(after.count(in: .folders), 0)
        let t = SampleData.snapshot(for: .trustedFolder)
        let t2 = SampleReducer.apply(.stopTrusting(folderID: "notes"), to: t)
        XCTAssertNotNil(t2.folder("journal"))
        XCTAssertNil(t2.folder("journal")?.parentID)
    }

    func testMergeChangesOnlyTheChosenChild() {
        let t = SampleData.snapshot(for: .trustedFolder)
        let after = SampleReducer.apply(.mergeIntoParent(folderID: "journal"), to: t)
        XCTAssertNil(after.folder("journal"))
        XCTAssertNotNil(after.folder("clips"))
        XCTAssertEqual(after.folder("notes")?.documentCount, 10)
        XCTAssertEqual(SampleReducer.apply(.mergeIntoParent(folderID: "notes"), to: t), t)
    }

    func testAnsweringARequestClearsItEverywhere() {
        let s = SampleData.snapshot(for: .syncedWithTeams)
        let after = SampleReducer.apply(.answerRequest(waitingID: "w-req-todo", allow: true), to: s)
        XCTAssertEqual(after.count(in: .programs), 0)
        XCTAssertEqual(after.totalCount, 2)
        XCTAssertTrue(after.requests.isEmpty)
        XCTAssertEqual(after.program("todo-search")?.allowedDocuments, ["todo.htmlclay"])
        XCTAssertEqual(SampleReducer.apply(.answerRequest(waitingID: "w-req-todo", allow: true), to: after), after)
        let denied = SampleReducer.apply(.answerRequest(waitingID: "w-req-todo", allow: false), to: s)
        XCTAssertEqual(denied.program("todo-search")?.allowedDocuments, [])
    }

    func testOnlyBlockingKindsEarnTheDot() {
        let earning: Set<WaitingItem.Kind> = [.conflict, .programRequest, .syncKeyRefused, .folderMissing, .serviceFailed]
        for kind in WaitingItem.Kind.allCases { XCTAssertEqual(kind.earnsDot, earning.contains(kind), kind.rawValue) }
    }

    func testDotClearsWhenSeenAndReturnsForANewItem() async throws {
        let host = SampleHost(persona: .trustedFolder)
        let model = AppModel(host: host, initial: host.current)
        XCTAssertTrue(model.showsDot)
        model.markWaitingSeen()
        XCTAssertFalse(model.showsDot)
        XCTAssertEqual(model.snapshot.totalCount, 1, "counts stay after seeing")
        host.pretend(.syncedWithTeams)
        XCTAssertTrue(model.showsDot)
    }

    func testNoDotForItemsThatCanWait() {
        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: host.current)
        XCTAssertEqual(model.snapshot.totalCount, 1)
        XCTAssertFalse(model.showsDot)
    }

    func testRouteLeavesARemovedFolder() async throws {
        let host = SampleHost(persona: .trustedFolder)
        let model = AppModel(host: host, initial: host.current)
        model.route = .folder("journal")
        _ = try await host.perform(.mergeIntoParent(folderID: "journal"))
        XCTAssertEqual(model.route, .pane(.folders))
    }

    func testPretendKeepsAppearance() async throws {
        let host = SampleHost(persona: .newPerson)
        _ = try await host.perform(.setAppearance(.dark))
        host.pretend(.servedFolder)
        XCTAssertEqual(host.current.preferences.appearance, .dark)
    }

    func testTrustingAFolderAdoptsTrustedFoldersInside() {
        var s = SampleReducer.apply(.stopTrusting(folderID: "notes"), to: SampleData.snapshot(for: .trustedFolder))
        XCTAssertEqual(TrustRules.trustedInside("~/Documents/Notes", in: s), 2)
        s = SampleReducer.apply(.trustFolder(name: "Notes", path: "~/Documents/Notes"), to: s)
        XCTAssertEqual(s.folder("notes")?.documentCount, 4)
        XCTAssertEqual(s.children(of: "notes").map(\.id), ["journal", "clips"])
        s = SampleReducer.apply(.trustFolder(name: "Documents", path: "~/Documents"), to: s)
        XCTAssertEqual(s.folder("notes")?.parentID, "documents")
        XCTAssertEqual(s.folder("journal")?.parentID, "notes")
        XCTAssertEqual(SampleReducer.apply(.trustFolder(name: "Documents", path: "~/Documents"), to: s), s)
    }

    func testServeFolderServesAnExistingFolderOrAddsOne() {
        let served = SampleReducer.apply(.serveFolder(path: "~/Documents/Notes", name: "my-notes"), to: SampleData.snapshot(for: .trustedFolder))
        XCTAssertEqual(served.folder("notes")?.served, true)
        XCTAssertEqual(served.folder("notes")?.address, "my-notes.htmlclay.localhost:1212")
        let added = SampleReducer.apply(.serveFolder(path: "~/Sites/blog", name: "blog"), to: HostSnapshot())
        XCTAssertEqual(added.folders.map(\.id), ["blog"])
        XCTAssertEqual(added.folders.first?.served, true)
    }

    func testServeAndTrustRules() {
        let teams = SampleData.snapshot(for: .syncedWithTeams)
        XCTAssertEqual(ServeRules.pathProblem("~/hyperclay/panphora/apps", folderID: nil, in: teams), "This is inside panphora, which is already served.")
        XCTAssertEqual(ServeRules.nameProblem("prototypes", folderID: nil, in: teams), "“prototypes” is already taken. Try another name.")
        XCTAssertNil(ServeRules.nameProblem("prototypes", folderID: "prototypes", in: teams))
        XCTAssertFalse(ServeRules.canServe(path: "~/Sites/blog", name: "", folderID: nil, in: teams))
        XCTAssertTrue(ServeRules.canServe(path: "~/Sites/blog", name: "blog", folderID: nil, in: teams))
        XCTAssertEqual(ServeRules.startName(folderID: "notes", in: teams), "notes")
        XCTAssertEqual(ServeRules.firstPick(in: SampleData.snapshot(for: .servedFolder)), 1)
        XCTAssertEqual(ServeRules.pathProblem("~/HyperclayLocal/panphora", folderID: nil, in: SampleData.snapshot(for: .localUser)), "Hyperclay Local is still serving this folder. Move from Hyperclay Local first.")
        XCTAssertEqual(TrustRules.firstPick(in: HostSnapshot())?.path, "~/Documents")
        XCTAssertEqual(TrustRules.pick(after: "~/Documents", in: HostSnapshot())?.path, "~/Documents/Notes")
        XCTAssertEqual(TrustRules.pick(after: "~/Documents", in: SampleData.snapshot(for: .trustedFolder))?.path, "~/Documents/Recipes")
    }

    func testConnectTeamAndProgramRules() {
        XCTAssertTrue(ConnectRules.accepts(SampleData.syncKey))
        XCTAssertFalse(ConnectRules.accepts("hcsk_1"))
        XCTAssertFalse(ConnectRules.accepts("abc123456789"))

        let teams = SampleData.snapshot(for: .syncedWithTeams)
        guard let marketing = teams.folder("marketing") else { return XCTFail("missing marketing") }
        let paths = TeamSetupRules.samplePaths(for: marketing)
        XCTAssertEqual(paths, ["~/hyperclay/Marketing", "~/Documents", "~/hyperclay/panphora/marketing"])
        XCTAssertNil(TeamSetupRules.problem(paths[0], in: teams))
        XCTAssertEqual(TeamSetupRules.problem(paths[1], in: teams), "This folder isn't empty. Choose an empty one.")
        XCTAssertEqual(TeamSetupRules.problem(paths[2], in: teams), "It's inside panphora.")
        XCTAssertEqual(TeamSetupRules.address(for: marketing), "marketing.htmlclay.localhost:1212")

        XCTAssertEqual(ProgramRules.defaultName(for: "~/bin/export-csv.py"), "export-csv")
        XCTAssertEqual(ProgramRules.nameProblem("Export CSV", in: teams), .invalid)
        XCTAssertEqual(ProgramRules.nameProblem("todo-search", in: teams), .taken)
        XCTAssertNil(ProgramRules.nameProblem("export-csv", in: teams))

        let added = SampleReducer.apply(.addProgram(name: "export-csv", path: "~/bin/export-csv.py"), to: teams)
        XCTAssertEqual(added.program("export-csv")?.allowedDocuments, [])
        XCTAssertEqual(SampleReducer.apply(.addProgram(name: "export-csv", path: "~/bin/export-csv.py"), to: added).programs.filter { $0.name == "export-csv" }.count, 1)

        var refused = teams
        refused.waiting.append(WaitingItem(id: "w-key-refused", kind: .syncKeyRefused, title: "Sync key refused"))
        XCTAssertFalse(SampleReducer.apply(.connectSample, to: refused).waiting.contains { $0.kind == .syncKeyRefused })
    }

    func testMoveFromLocalAndQuitRule() {
        let local = SampleData.snapshot(for: .localUser)
        let moved = SampleReducer.apply(.moveFromLocal(folderIDs: ["panphora", "design-team"]), to: local)
        XCTAssertFalse(moved.localMoveOffer)
        XCTAssertEqual(moved.folders.map(\.id), ["panphora", "design-team"])
        XCTAssertTrue(moved.sync.connected)
        XCTAssertEqual(moved.waiting.filter { $0.kind == .conflict }.count, 2)
        XCTAssertEqual(SampleReducer.apply(.moveFromLocal(folderIDs: ["panphora", "design-team"]), to: moved), moved)
        XCTAssertTrue(SampleReducer.apply(.moveFromLocal(folderIDs: ["panphora"]), to: local).waiting.isEmpty)

        XCTAssertEqual(MoveRules.ordered(["design-team", "panphora"]), ["panphora", "design-team"])
        XCTAssertFalse(MoveRules.canMove(blockers: 1, checked: ["panphora"], key: "k"))
        XCTAssertFalse(MoveRules.canMove(blockers: 0, checked: [], key: "k"))
        XCTAssertFalse(MoveRules.canMove(blockers: 0, checked: ["panphora"], key: String()))
        XCTAssertTrue(MoveRules.canMove(blockers: 0, checked: ["panphora"], key: "k"))

        let syncing = SampleData.snapshot(for: .syncedWithTeams)
        XCTAssertEqual(syncing.filesStillUploading, 5)
        var paused = syncing
        paused.sync.paused = true
        XCTAssertEqual(paused.filesStillUploading, 0)
        var offline = syncing
        offline.sync.offlineChanges = 8
        XCTAssertEqual(offline.filesStillUploading, 0)
        XCTAssertEqual(HostSnapshot().filesStillUploading, 0)

        for note in SampleNotification.allCases {
            XCTAssertFalse(Strings.Notify.title(note).isEmpty)
            XCTAssertFalse(Strings.Notify.body(note).isEmpty)
        }
    }
}
