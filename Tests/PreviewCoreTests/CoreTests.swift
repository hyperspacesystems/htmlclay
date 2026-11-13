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
}
