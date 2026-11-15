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
        "program.setAllTrusted", "folder.setSynced", "document.recover", "conflict.resolve", "folder.resolveRemoval",
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

        XCTAssertTrue(SettingsControls.showsOpenAtLogin(isSample: model.isSample))
        XCTAssertFalse(SettingsControls.showsAccountChange(isSample: model.isSample))
        XCTAssertFalse(SettingsControls.showsUpdateControls(can: model.can(HostActionType.updateStart)))
        XCTAssertFalse(SettingsControls.showsTroubleshooting(isSample: model.isSample))
        XCTAssertFalse(SettingsControls.showsServiceRestart(can: model.can(HostActionType.serviceRestart)))
    }

    func testARealHostWithNoSnapshotYetIsNotSample() {
        let relay = RelayHostClient()
        let model = AppModel(host: relay, initial: relay.last)

        XCTAssertFalse(model.isSample)
        XCTAssertTrue(SettingsControls.showsOpenAtLogin(isSample: model.isSample))
        XCTAssertFalse(SettingsControls.showsAccountChange(isSample: model.isSample))
        XCTAssertFalse(SettingsControls.showsTroubleshooting(isSample: model.isSample))
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

    func testTheLiveServiceRowFollowsTheHostsServiceState() {
        let running = SettingsControls.serviceStatus("running")
        XCTAssertEqual(running.dot, .green)
        XCTAssertEqual(running.text, Strings.Advanced.running)

        for state in ["starting", "restarting"] {
            let row = SettingsControls.serviceStatus(state)
            XCTAssertEqual(row.dot, .amber, state)
            XCTAssertEqual(row.text, Strings.Advanced.starting, state)
        }

        let failed = SettingsControls.serviceStatus("failed")
        XCTAssertEqual(failed.dot, .red)
        XCTAssertEqual(failed.text, Strings.Advanced.stopped)

        let off = SettingsControls.serviceStatus("off")
        XCTAssertEqual(off.dot, .muted)
        XCTAssertEqual(off.text, Strings.Advanced.off)
    }

    func testTheDashboardAddressDropsTheSchemeAndTrailingSlash() {
        XCTAssertEqual(SettingsControls.dashboardAddress("http://htmlclay.localhost:1212/"),
                       "htmlclay.localhost:1212")
        XCTAssertEqual(SettingsControls.dashboardAddress("https://example.test:4000"),
                       "example.test:4000")
        XCTAssertEqual(SettingsControls.dashboardAddress(""), "")
    }

    func testThePortShowsTheSnapshotOnTheRealHostAndTheDesignedOneInThePreview() {
        XCTAssertEqual(SettingsControls.port(isSample: true, port: 4040), Strings.Advanced.portNumber)
        XCTAssertEqual(SettingsControls.port(isSample: false, port: 4040), "4040")
    }

    func testStorageTotalsAreHiddenOnTheRealHost() {
        XCTAssertTrue(SettingsControls.showsStorageTotals(isSample: true))
        XCTAssertFalse(SettingsControls.showsStorageTotals(isSample: false))
    }
}
