import XCTest
import AppKit
import SwiftUI
import PreviewCore
@testable import PreviewUI

@MainActor
final class ProblemStateTests: XCTestCase {
    func testOverlaysToggle() {
        let host = SampleHost(persona: .syncedWithTeams)
        XCTAssertEqual(PanelView.topSections(host.current), [.openDashboard, .waiting, .syncLine])
        host.toggle(.keyRefused)
        XCTAssertTrue(host.isOn(.keyRefused))
        XCTAssertEqual(host.current.count(in: .sync), 1)
        XCTAssertEqual(PanelView.topSections(host.current), [.openDashboard, .keyRefused, .waiting])
        host.toggle(.keyRefused)
        XCTAssertFalse(host.isOn(.keyRefused))
        XCTAssertEqual(host.current.count(in: .sync), 0)
        host.toggle(.offline)
        XCTAssertEqual(host.current.sync.offlineChanges, 8)
        XCTAssertEqual(PanelView.topSections(host.current).last, .offline)
        host.toggle(.hostStopped)
        XCTAssertEqual(PanelView.topSections(host.current).first, .hostStopped)
        XCTAssertFalse(PanelView.topSections(host.current).contains(.openDashboard))

        let fresh = SampleHost(persona: .newPerson)
        XCTAssertFalse(fresh.canToggle(.keyRefused))
        fresh.toggle(.keyRefused)
        XCTAssertTrue(fresh.current.waiting.isEmpty)
    }

    func testUpdateFinishes() async throws {
        let host = SampleHost(persona: .newPerson)
        host.toggle(.updateReady)
        XCTAssertTrue(PanelView.topSections(host.current).contains(.update))
        _ = await host.perform(.startUpdate)
        XCTAssertTrue(PanelView.topSections(host.current).contains(.finishing))
        _ = await host.perform(.finishUpdate)
        XCTAssertEqual(host.current.update.current, "2.1.0")
        XCTAssertNil(host.current.update.ready)
        XCTAssertFalse(PanelView.topSections(host.current).contains(.update))
        XCTAssertFalse(PanelView.topSections(host.current).contains(.finishing))
    }

    func testTeamFolderProblems() async throws {
        let host = SampleHost(persona: .syncedWithTeams)
        host.toggle(.teamFolderMissing)
        let missing = try XCTUnwrap(host.current.folder(SampleData.overlayTeamID))
        XCTAssertTrue(missing.missingOnDisk)
        XCTAssertFalse(missing.synced)
        XCTAssertEqual(FolderPage.problem(for: missing), .missing)
        XCTAssertEqual(host.current.count(in: .folders), 3)
        _ = await host.perform(.setSynced(folderID: SampleData.overlayTeamID, on: true))
        XCTAssertFalse(try XCTUnwrap(host.current.folder(SampleData.overlayTeamID)).synced)

        host.toggle(.teamFolderMissing)
        XCTAssertTrue(try XCTUnwrap(host.current.folder(SampleData.overlayTeamID)).synced)
        XCTAssertEqual(host.current.count(in: .folders), 2)

        host.toggle(.teamViewer)
        XCTAssertEqual(FolderPage.problem(for: try XCTUnwrap(host.current.folder(SampleData.overlayTeamID))), .viewer)

        let panphora = try XCTUnwrap(host.current.folder("panphora"))
        let status = FolderPage.syncStatus(panphora, snapshot: host.current)
        XCTAssertEqual(status.dot, .green)
        XCTAssertEqual(status.line, "Up to date · 2 minutes ago")
        host.toggle(.offline)
        let offline = FolderPage.syncStatus(panphora, snapshot: host.current)
        XCTAssertEqual(offline.dot, .muted)
        XCTAssertEqual(offline.line, "Offline, 8 changes waiting")
    }

    func testHistoryRoute() {
        _ = NSApplication.shared
        XCTAssertEqual(FolderHistoryPage.symbol(.uploaded), Symbols.uploaded)
        let host = SampleHost(persona: .syncedWithTeams)
        for overlay in PreviewOverlay.allCases {
            host.toggle(overlay)
        }
        let model = AppModel(host: host, initial: host.current)
        let history = NSHostingView(rootView: KilnRoot { SettingsView(route: .folderHistory("panphora")) }.environment(model))
        history.layoutSubtreeIfNeeded()
        XCTAssertGreaterThanOrEqual(history.fittingSize.width, 382)
        XCTAssertGreaterThan(history.fittingSize.height, 120)
        let panel = NSHostingView(rootView: KilnRoot { PanelView() }.environment(model))
        panel.layoutSubtreeIfNeeded()
        XCTAssertGreaterThanOrEqual(panel.fittingSize.width, 382)
        XCTAssertGreaterThan(panel.fittingSize.height, 120)
    }
}
