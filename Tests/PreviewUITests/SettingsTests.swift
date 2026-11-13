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

    func testFolderRowShowsEveryActiveLevelPill() {
        let snapshot = SampleData.snapshot(for: .syncedWithTeams)
        guard let panphora = snapshot.folder("panphora") else { return XCTFail("missing panphora") }
        XCTAssertEqual(FolderRow.pills(for: panphora), [.trusted, .served, .synced])
        guard let notes = snapshot.folder("notes") else { return XCTFail("missing notes") }
        XCTAssertEqual(FolderRow.pills(for: notes), [.trusted])
    }
}
