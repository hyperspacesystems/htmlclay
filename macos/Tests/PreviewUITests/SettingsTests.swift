import XCTest
import AppKit
import SwiftUI
import PreviewCore
@testable import PreviewUI

@MainActor
final class SettingsTests: XCTestCase {
    func testSidebarBadgeCountsMatchTheSnapshot() {
        for persona in Persona.allCases {
            let host = SampleHost(persona: persona)
            let snapshot = host.current
            for pane in SettingsPane.allCases {
                let expected = snapshot.count(in: pane)
                XCTAssertEqual(SettingsSidebar.badgeCount(for: pane, snapshot: snapshot),
                               expected == 0 ? nil : expected,
                               "\(persona.rawValue) \(pane.rawValue)")
            }
        }
    }

    func testEverySettingsPaneLaysOutAtTheFrameWidth() {
        _ = NSApplication.shared
        for persona in Persona.allCases {
            let host = SampleHost(persona: persona)
            let model = AppModel(host: host, initial: host.current)
            for pane in SettingsPane.allCases {
                let hosting = NSHostingView(rootView: KilnRoot { SettingsView(pane: pane) }.environment(model))
                hosting.layoutSubtreeIfNeeded()
                XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 840, "\(persona.rawValue) \(pane.rawValue)")
            }
        }
    }

    func testFoldersPaneOrdersNestedLocalFoldersUnderTheirParent() {
        XCTAssertEqual(FoldersPane.orderedLocalFolders(SampleData.snapshot(for: .trustedFolder)),
                       ["notes", "journal", "clips"])
        XCTAssertEqual(FoldersPane.orderedLocalFolders(SampleData.snapshot(for: .servedFolder)),
                       ["notes", "prototypes"])
    }

    func testFolderPageKeepsItsLevelsAndLaysOutAfterStoppingServe() async throws {
        _ = NSApplication.shared
        let original = SampleData.snapshot(for: .syncedWithTeams)
        guard let panphora = original.folder("panphora"),
              let prototypes = original.folder("prototypes"),
              let notes = original.folder("notes") else { return XCTFail("missing sample folders") }
        XCTAssertEqual(FolderPage.levels(for: panphora).innerKind, .sync)
        XCTAssertTrue(FolderPage.levels(for: panphora).served)
        XCTAssertEqual(FolderPage.levels(for: prototypes).innerKind, .onlyOnThisMac)
        XCTAssertEqual(FolderPage.levels(for: notes).innerKind, .none)

        let host = SampleHost(persona: .syncedWithTeams)
        let model = AppModel(host: host, initial: host.current)
        _ = await host.perform(.setServed(folderID: "panphora", on: false))
        XCTAssertEqual(model.snapshot.folder("panphora")?.served, false)

        let hosting = NSHostingView(rootView: KilnRoot { SettingsView(route: .folder("panphora")) }.environment(model))
        hosting.layoutSubtreeIfNeeded()
        XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 840)
        XCTAssertEqual(model.route, .pane(.general))
    }

    func testProgramsPaneWords() async throws {
        _ = NSApplication.shared
        let served = SampleData.snapshot(for: .servedFolder)
        guard let search = served.program("search-sh"),
              let bluesky = served.program("bluesky-studio"),
              let claySearch = served.program("clay-search") else { return XCTFail("missing sample programs") }
        XCTAssertEqual(ProgramRow.subtitle(search), "notes.htmlclay may run it")
        XCTAssertEqual(ProgramRow.subtitle(bluesky), "All documents in trusted folders")
        XCTAssertEqual(ProgramRow.fileName(claySearch), "clay-search")
        XCTAssertEqual(Strings.Programs.fileMissing(1), "file missing · 1 document uses it")

        let trusted = SampleData.snapshot(for: .trustedFolder)
        let waiting = ProgramsPane.waitingRequests(trusted)
        XCTAssertEqual(waiting.map(\.id), ["w-req-todo"])
        if let item = waiting.first {
            XCTAssertEqual(ProgramsPane.waitingTitle(item, snapshot: trusted), "todo.htmlclay wants “todo-search”")
        }
        guard let todoSearch = trusted.program("todo-search") else { return XCTFail("missing todo-search") }
        XCTAssertEqual(ProgramRow.subtitle(todoSearch), "No document may run it until you allow one.")

        XCTAssertTrue(ProgramsPane.waitingRequests(SampleData.snapshot(for: .newPerson)).isEmpty)

        let host = SampleHost(persona: .servedFolder)
        let model = AppModel(host: host, initial: host.current)
        _ = await host.perform(.removeProgramAccess(programID: "search-sh", document: "notes.htmlclay"))
        XCTAssertEqual(host.current.program("search-sh")?.allowedDocuments, [])
        XCTAssertEqual(host.current.program("search-sh")?.removedDocuments, ["notes.htmlclay"])

        let hosting = NSHostingView(rootView: KilnRoot { SettingsView(route: .program("search-sh")) }.environment(model))
        hosting.layoutSubtreeIfNeeded()
        XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 840)
    }

    func testFolderRowShowsEveryActiveLevelPill() {
        let snapshot = SampleData.snapshot(for: .syncedWithTeams)
        guard let panphora = snapshot.folder("panphora") else { return XCTFail("missing panphora") }
        XCTAssertEqual(FolderRow.pills(for: panphora), [.trusted, .served, .synced])
        guard let notes = snapshot.folder("notes") else { return XCTFail("missing notes") }
        XCTAssertEqual(FolderRow.pills(for: notes), [.trusted])
    }

    func testSyncPane() async throws {
        _ = NSApplication.shared
        let snapshot = SampleData.snapshot(for: .syncedWithTeams)
        XCTAssertEqual(SyncPane.orderedFolders(snapshot).map(\.id), ["panphora", "design-team", "marketing"])
        guard let panphora = snapshot.folder("panphora"),
              let designTeam = snapshot.folder("design-team"),
              let marketing = snapshot.folder("marketing") else { return XCTFail("missing sample folders") }
        XCTAssertEqual(SyncPane.dot(for: panphora, snapshot: snapshot), .blue)
        XCTAssertEqual(SyncPane.dot(for: designTeam, snapshot: snapshot), .amber)
        XCTAssertEqual(SyncPane.dot(for: marketing, snapshot: snapshot), .muted)
        XCTAssertEqual(SyncPane.folderSubtitle(panphora, snapshot: snapshot), "~/hyperclay/panphora · syncing")
        XCTAssertEqual(SyncPane.folderSubtitle(marketing, snapshot: snapshot), "Not on this Mac")
        XCTAssertEqual(SyncPane.conflicts(designTeam, snapshot: snapshot), 2)
        XCTAssertEqual(SyncPane.syncingLine(snapshot.sync), "Syncing 5 files… · last finished 2 minutes ago")

        var paused = snapshot
        paused.sync.paused = true
        XCTAssertEqual(SyncPane.dot(for: panphora, snapshot: paused), .muted)
        XCTAssertEqual(SyncPane.syncingLine(paused.sync), "Syncing paused")

        let host = SampleHost(persona: .newPerson)
        XCTAssertFalse(host.current.sync.connected)
        _ = await host.perform(.connectSample)
        XCTAssertTrue(host.current.sync.connected)
        XCTAssertEqual(SyncPane.orderedFolders(host.current).map(\.id), ["panphora"])
        _ = await host.perform(.disconnect)
        XCTAssertFalse(host.current.sync.connected)

        for persona in [Persona.newPerson, .syncedWithTeams] {
            let other = SampleHost(persona: persona)
            let model = AppModel(host: other, initial: other.current)
            let hosting = NSHostingView(rootView: KilnRoot { SettingsView(pane: .sync) }.environment(model))
            hosting.layoutSubtreeIfNeeded()
            XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 840, persona.rawValue)
        }
    }

    func testNoPaneIsAStub() throws {
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let sources = root.appendingPathComponent("Sources")
        let files = (FileManager.default.enumerator(at: sources, includingPropertiesForKeys: nil)?.allObjects as? [URL] ?? []).filter { $0.pathExtension == "swift" }
        XCTAssertGreaterThan(files.count, 20)
        for file in files {
            let text = try String(contentsOf: file, encoding: .utf8)
            XCTAssertFalse(text.contains("PaneStub"), file.lastPathComponent)
            XCTAssertFalse(text.contains("notYet"), file.lastPathComponent)
        }
    }

    func testUtilityPanes() async throws {
        _ = NSApplication.shared
        XCTAssertEqual(YourNamePane.mode(SampleData.snapshot(for: .newPerson)), .off)
        XCTAssertEqual(YourNamePane.mode(SampleData.snapshot(for: .syncedWithTeams)), .account)

        let localHost = SampleHost(persona: .newPerson)
        _ = try await localHost.perform(.setUseName(true))
        XCTAssertEqual(YourNamePane.mode(localHost.current), .local)

        XCTAssertEqual(UpdatesPane.short("2.1.0"), "2.1")
        XCTAssertEqual(UpdatesPane.short("2.0.3"), "2.0")

        let updateHost = SampleHost(persona: .newPerson)
        _ = try await updateHost.perform(.setAutoUpdate(false))
        XCTAssertFalse(updateHost.current.update.automatic)

        for persona in [Persona.newPerson, .syncedWithTeams] {
            let host = SampleHost(persona: persona)
            let model = AppModel(host: host, initial: host.current)
            for pane in [SettingsPane.yourName, .updates, .advanced] {
                let hosting = NSHostingView(rootView: KilnRoot { SettingsView(pane: pane) }.environment(model))
                hosting.layoutSubtreeIfNeeded()
                XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 840, persona.rawValue + " " + pane.rawValue)
            }
        }
    }

    func testFolderSheets() {
        _ = NSApplication.shared
        XCTAssertEqual(FoldersPane.orderedLocalFolders(SampleReducer.apply(.trustFolder(name: "Documents", path: "~/Documents"), to: SampleData.snapshot(for: .trustedFolder))),
                       ["documents", "notes", "journal", "clips"])
        let sheets: [PreviewSheet] = [.serveFolder(folderID: nil), .serveFolder(folderID: "notes")] + SampleData.trustCandidates.map { .trustFolder(name: $0.name, path: $0.path) }
        for persona in [Persona.trustedFolder, .syncedWithTeams] {
            let host = SampleHost(persona: persona)
            let model = AppModel(host: host, initial: host.current)
            model.folderChooser = SampleFolderChooser()
            for sheet in sheets {
                let hosting = NSHostingView(rootView: KilnRoot { SettingsSheetView(sheet: sheet, actions: SettingsActions()) }.environment(model))
                hosting.layoutSubtreeIfNeeded()
                XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 410, persona.rawValue + " " + sheet.id)
                XCTAssertGreaterThan(hosting.fittingSize.height, 120, persona.rawValue + " " + sheet.id)
            }
        }
    }

    func testSyncSheetsLayOut() {
        _ = NSApplication.shared
        for persona in [Persona.newPerson, Persona.syncedWithTeams] {
            let host = SampleHost(persona: persona)
            let model = AppModel(host: host, initial: host.current)
            var sheets: [PreviewSheet] = [.connect, .accountFolder(username: "panphora"), .addProgram]
            if persona == .syncedWithTeams {
                sheets.append(.teamSetup(folderID: "marketing"))
            }
            for sheet in sheets {
                let hosting = NSHostingView(rootView: KilnRoot { SettingsSheetView(sheet: sheet, actions: SettingsActions()) }.environment(model))
                hosting.layoutSubtreeIfNeeded()
                XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 460, "\(persona.rawValue) \(sheet.id)")
                XCTAssertGreaterThan(hosting.fittingSize.height, 120, "\(persona.rawValue) \(sheet.id)")
            }
        }
        XCTAssertEqual(AccountFolderSheet.path("panphora"), "~/hyperclay/panphora")
    }

    func testMoveSheetsAndNotificationCardLayOut() {
        _ = NSApplication.shared
        let host = SampleHost(persona: .localUser)
        let model = AppModel(host: host, initial: host.current)
        for sheet in [PreviewSheet.moveFromLocal, PreviewSheet.movedFromLocal] {
            let hosting = NSHostingView(rootView: KilnRoot { SettingsSheetView(sheet: sheet, actions: SettingsActions()) }.environment(model))
            hosting.layoutSubtreeIfNeeded()
            XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 460, sheet.id)
            XCTAssertGreaterThan(hosting.fittingSize.height, 120, sheet.id)
        }
        for note in SampleNotification.allCases {
            let hosting = NSHostingView(rootView: KilnRoot { NotificationCard(note: note) }.environment(model))
            hosting.layoutSubtreeIfNeeded()
            XCTAssertGreaterThanOrEqual(hosting.fittingSize.width, 368, note.rawValue)
            XCTAssertGreaterThanOrEqual(hosting.fittingSize.height, 50, note.rawValue)
        }
    }
}
