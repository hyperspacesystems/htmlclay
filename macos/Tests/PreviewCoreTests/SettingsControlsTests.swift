import XCTest
@testable import PreviewCore

@MainActor
final class SettingsControlsTests: XCTestCase {
    // Every type in appcore.ActionClasses, the way the host's can list holds them.
    private static let knownTypes = [
        "document.open", "document.reveal", "document.forget", "dashboard.open",
        "folder.reveal", "folder.copyAddress", "waiting.markSeen", "prefs.set",
        "sync.setPaused", "update.start", "service.restart", "host.quit",
        "folder.stopServing", "folder.stopTrusting", "folder.merge", "program.removeAccess",
        "sync.disconnect", "folder.trust", "folder.serve", "file.open", "request.answer",
        "program.add", "sync.connect", "folder.setUpTeam", "move.start",
        "program.setAllTrusted", "folder.setSynced",
    ]

    private func sampleModel(_ persona: Persona = .fiveDocuments) -> AppModel {
        let host = SampleHost(persona: persona)
        return AppModel(host: host, initial: host.current)
    }

    private func realHostModel() -> AppModel {
        var snapshot = SampleData.snapshot(for: .servedFolder)
        var can = Dictionary(uniqueKeysWithValues: Self.knownTypes.map { ($0, false) })
        can[HostActionType.prefsSet] = true
        snapshot.host.can = can
        return AppModel(host: SampleHost(persona: .servedFolder), initial: snapshot)
    }

    func testSampleDataShowsEveryPaneAndControl() {
        let model = sampleModel()
        XCTAssertTrue(model.isSample)
        for pane in SettingsPane.allCases {
            XCTAssertTrue(SettingsControls.showsPane(pane, in: model.snapshot), pane.rawValue)
        }
        let routes: [SettingsRoute] = [.pane(.general), .pane(.folders), .pane(.programs),
                                       .pane(.sync), .pane(.yourName), .pane(.updates),
                                       .pane(.advanced), .folder("notes"), .folderHistory("notes"),
                                       .program("search-sh")]
        for route in routes {
            XCTAssertEqual(SettingsControls.visibleRoute(route, in: model.snapshot), route)
        }
        XCTAssertTrue(SettingsControls.showsOpenAtLogin(isSample: model.isSample))
        XCTAssertTrue(SettingsControls.showsAccountChange(isSample: model.isSample))
        XCTAssertTrue(SettingsControls.showsUpdateControls(can: model.can(HostActionType.updateStart)))
        XCTAssertTrue(SettingsControls.showsTroubleshooting(isSample: model.isSample))
        XCTAssertTrue(SettingsControls.showsServiceRestart(can: model.can(HostActionType.serviceRestart)))
    }

    func testTheRealHostShowsOnlyWhatItCanDo() {
        let model = realHostModel()
        XCTAssertFalse(model.isSample)

        XCTAssertFalse(SettingsControls.showsPane(.programs, in: model.snapshot))
        XCTAssertFalse(SettingsControls.showsPane(.sync, in: model.snapshot))
        for pane in [SettingsPane.general, .folders, .yourName, .updates, .advanced] {
            XCTAssertTrue(SettingsControls.showsPane(pane, in: model.snapshot), pane.rawValue)
        }
        XCTAssertEqual(SettingsControls.visibleRoute(.pane(.programs), in: model.snapshot), .pane(.general))
        XCTAssertEqual(SettingsControls.visibleRoute(.program("search-sh"), in: model.snapshot), .pane(.general))
        XCTAssertEqual(SettingsControls.visibleRoute(.pane(.sync), in: model.snapshot), .pane(.general))
        XCTAssertEqual(SettingsControls.visibleRoute(.folder("notes"), in: model.snapshot), .folder("notes"))

        XCTAssertFalse(SettingsControls.showsOpenAtLogin(isSample: model.isSample))
        XCTAssertFalse(SettingsControls.showsAccountChange(isSample: model.isSample))
        XCTAssertFalse(SettingsControls.showsUpdateControls(can: model.can(HostActionType.updateStart)))
        XCTAssertFalse(SettingsControls.showsTroubleshooting(isSample: model.isSample))
        XCTAssertFalse(SettingsControls.showsServiceRestart(can: model.can(HostActionType.serviceRestart)))
    }

    func testAPaneShowsWhenTheHostRunsAnyOfItsActions() {
        var snapshot = SampleData.snapshot(for: .servedFolder)
        var can = Dictionary(uniqueKeysWithValues: Self.knownTypes.map { ($0, false) })
        can[HostActionType.programAdd] = true
        snapshot.host.can = can
        XCTAssertTrue(SettingsControls.showsPane(.programs, in: snapshot))
        XCTAssertFalse(SettingsControls.showsPane(.sync, in: snapshot))

        can[HostActionType.programAdd] = false
        can[HostActionType.syncSetPaused] = true
        snapshot.host.can = can
        XCTAssertFalse(SettingsControls.showsPane(.sync, in: snapshot))

        can[HostActionType.syncConnect] = true
        snapshot.host.can = can
        XCTAssertTrue(SettingsControls.showsPane(.sync, in: snapshot))
    }
}
