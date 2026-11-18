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
        "program.add", "sync.connect", "folder.setUpTeam", "move.start", "move.quitLocal",
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
        let routes: [SettingsRoute] = [.pane(.general), .pane(.htmlclayFiles), .pane(.folders),
                                       .pane(.programs), .pane(.updates), .pane(.advanced),
                                       .pane(.sync), .folder("notes"), .folderHistory("notes"),
                                       .program("search-sh")]
        for route in routes {
            XCTAssertEqual(SettingsControls.visibleRoute(route, in: model.snapshot), route)
        }
        XCTAssertTrue(SettingsControls.showsOpenAtLogin(isSample: model.isSample))
        XCTAssertTrue(SettingsControls.showsAccountChange(isSample: model.isSample))
        XCTAssertTrue(SettingsControls.showsUpdateControls(can: model.can(HostActionType.updateStart)))
        XCTAssertTrue(SettingsControls.showsServiceRestart(can: model.can(HostActionType.serviceRestart)))
    }

    func testTheRealHostShowsOnlyWhatItCanDo() {
        let model = realHostModel()
        XCTAssertFalse(model.isSample)

        XCTAssertFalse(SettingsControls.showsPane(.programs, in: model.snapshot))
        XCTAssertFalse(SettingsControls.showsPane(.sync, in: model.snapshot))
        for pane in [SettingsPane.general, .htmlclayFiles, .folders, .updates, .advanced] {
            XCTAssertTrue(SettingsControls.showsPane(pane, in: model.snapshot), pane.rawValue)
        }
        XCTAssertEqual(SettingsControls.visibleRoute(.pane(.programs), in: model.snapshot), .pane(.general))
        XCTAssertEqual(SettingsControls.visibleRoute(.program("search-sh"), in: model.snapshot), .pane(.general))
        XCTAssertEqual(SettingsControls.visibleRoute(.pane(.sync), in: model.snapshot), .pane(.general))
        XCTAssertEqual(SettingsControls.visibleRoute(.folder("notes"), in: model.snapshot), .folder("notes"))

        XCTAssertTrue(SettingsControls.showsOpenAtLogin(isSample: model.isSample))
        XCTAssertFalse(SettingsControls.showsAccountChange(isSample: model.isSample))
        XCTAssertFalse(SettingsControls.showsUpdateControls(can: model.can(HostActionType.updateStart)))
        XCTAssertFalse(SettingsControls.showsServiceRestart(can: model.can(HostActionType.serviceRestart)))
    }

    func testARealHostWithNoSnapshotYetIsNotSample() {
        let relay = RelayHostClient()
        let model = AppModel(host: relay, initial: relay.last)

        XCTAssertFalse(model.isSample)
        XCTAssertTrue(SettingsControls.showsOpenAtLogin(isSample: model.isSample))
        XCTAssertFalse(SettingsControls.showsAccountChange(isSample: model.isSample))
    }

    /// copy.md section 2: the seven tabs, in this order, and no Your name tab.
    func testTheSidebarListsTheSevenTabsInOrder() {
        XCTAssertEqual(SettingsPane.allCases.map(\.rawValue),
                       ["general", "htmlclayFiles", "folders", "programs", "updates", "advanced", "sync"])
        XCTAssertEqual(SettingsPane.allCases.map(Strings.Settings.title),
                       ["General", ".htmlclay files", "Folders", "Programs", "Updates", "Advanced", "Sync"])
        XCTAssertNil(SettingsPane(rawValue: "yourName"))
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

        XCTAssertTrue(SettingsControls.offersServiceRestart("failed", hostCan: true, isSample: false))
        XCTAssertFalse(SettingsControls.offersServiceRestart("running", hostCan: true, isSample: false))
        XCTAssertFalse(SettingsControls.offersServiceRestart("failed", hostCan: false, isSample: false))
        XCTAssertFalse(SettingsControls.offersServiceRestart("failed", hostCan: true, isSample: true))
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

    /// copy.md section 6, from the syncedWithTeams fixture: Claude was found and
    /// is selected, Codex was not found so it is absent, and the custom command
    /// comes last.
    func testTheToolPickerListsFoundToolsThenTheCustomCommand() throws {
        let fixtures = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .appendingPathComponent("Fixtures/v1")
        let data = try Data(contentsOf: fixtures.appendingPathComponent("syncedWithTeams.snapshot.json"))
        let wire = try HostWireCoding.decoder().decode(WireSnapshot.self, from: data)
        let prefs = HostSnapshotAdapter.snapshot(from: wire).preferences

        let rows = SettingsControls.aiToolRows(prefs)
        XCTAssertEqual(rows.map(\.id), ["claude", "custom"])
        XCTAssertEqual(rows.map(\.title), [Strings.General.aiToolClaude, Strings.General.aiCustomTitle])
        XCTAssertEqual(rows.map(\.selected), [true, false])
        XCTAssertTrue(rows[0].subtitle == Strings.General.aiFoundHere)
        XCTAssertEqual(rows[0].model, "claude-opus-5-5")
        XCTAssertEqual(rows[0].placeholder, "claude-opus-5-5")
        XCTAssertEqual(rows[1].subtitle, "my-agent {prompt}")
        XCTAssertTrue(rows[1].custom)
        XCTAssertFalse(SettingsControls.showsNoToolsNote(prefs))

        var noneFound = prefs
        noneFound.aiEditTools = prefs.aiEditTools.filter { $0.custom || !$0.found }
        XCTAssertTrue(SettingsControls.showsNoToolsNote(noneFound))
        XCTAssertEqual(SettingsControls.aiToolRows(noneFound).map(\.id), ["custom"])

        var codexOnly = prefs
        codexOnly.aiEditTool = "codex"
        codexOnly.aiEditTools = [AIEditTool(id: "codex", found: true)]
        let codex = SettingsControls.aiToolRows(codexOnly)
        XCTAssertEqual(codex.map(\.id), ["codex", "custom"])
        XCTAssertTrue(codex[1].custom)
        XCTAssertFalse(codex[1].selectable, "the custom row cannot be picked before a command is set")
        XCTAssertEqual(codex[1].subtitle, Strings.General.aiCustomSub)
        XCTAssertEqual(codex[0].title, Strings.General.aiToolCodex)
        XCTAssertTrue(codex[0].selected)
        XCTAssertEqual(codex[0].placeholder, Strings.General.aiCodexDefault)
    }

    func testTheSyncLogRowShowsOnceSyncHasBeenOn() {
        XCTAssertFalse(SettingsControls.showsSyncLog(SyncState()))
        XCTAssertTrue(SettingsControls.showsSyncLog(SyncState(connected: true, username: "panphora", everOn: true)))
        XCTAssertTrue(SettingsControls.showsSyncLog(SyncState(everOn: true)))
    }

    func testThePortCheckKeepsTheRange() {
        XCTAssertNil(SettingsControls.portProblem("1212"))
        XCTAssertNil(SettingsControls.portProblem("1024"))
        XCTAssertNil(SettingsControls.portProblem("65535"))
        XCTAssertNil(SettingsControls.portProblem(" 8080 "))

        for text in ["", "1023", "65536", "0", "abc", "12 12", "-1", "12.5"] {
            XCTAssertEqual(SettingsControls.portProblem(text), Strings.Advanced.portRange, text)
        }
    }

    func testStorageTotalsAreHiddenOnTheRealHost() {
        XCTAssertTrue(SettingsControls.showsStorageTotals(isSample: true))
        XCTAssertFalse(SettingsControls.showsStorageTotals(isSample: false))
    }
}
