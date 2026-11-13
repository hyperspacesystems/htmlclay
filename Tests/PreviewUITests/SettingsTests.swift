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
        _ = try await host.perform(.setServed(folderID: "panphora", on: false))
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
        _ = try await host.perform(.removeProgramAccess(programID: "search-sh", document: "notes.htmlclay"))
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
}
