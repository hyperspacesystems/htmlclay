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

    func testFolderRowShowsEveryActiveLevelPill() {
        let snapshot = SampleData.snapshot(for: .syncedWithTeams)
        guard let panphora = snapshot.folder("panphora") else { return XCTFail("missing panphora") }
        XCTAssertEqual(FolderRow.pills(for: panphora), [.trusted, .served, .synced])
        guard let notes = snapshot.folder("notes") else { return XCTFail("missing notes") }
        XCTAssertEqual(FolderRow.pills(for: notes), [.trusted])
    }
}
