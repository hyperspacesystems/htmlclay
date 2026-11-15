import XCTest
@testable import PreviewCore

@MainActor
final class PanelControlsTests: XCTestCase {
    private func sampleModel(_ persona: Persona = .fiveDocuments) -> AppModel {
        let host = SampleHost(persona: persona)
        return AppModel(host: host, initial: host.current)
    }

    func testSampleDataShowsEveryControl() {
        let model = sampleModel()
        XCTAssertTrue(model.isSample)
        XCTAssertTrue(PanelControls.showsDocumentAction(HostActionType.documentOpen, in: model.snapshot))
        XCTAssertTrue(PanelControls.showsDocumentAction(HostActionType.documentReveal, in: model.snapshot))
        XCTAssertTrue(PanelControls.showsDocumentAction(HostActionType.documentForget, in: model.snapshot))
        XCTAssertTrue(PanelControls.showsExport(isSample: model.isSample))
        XCTAssertTrue(PanelControls.asksAnotherDestination(for: "destination-exists"))
        XCTAssertFalse(PanelControls.asksAnotherDestination(for: "failed"))
        XCTAssertTrue(PanelControls.showsUpdate(can: model.can(HostActionType.updateStart)))
        XCTAssertTrue(PanelControls.showsSyncPause(can: model.can(HostActionType.syncSetPaused)))
        XCTAssertTrue(PanelControls.showsAnswer(can: model.can(HostActionType.requestAnswer)))
        XCTAssertTrue(PanelControls.showsAppLog(isSample: model.isSample))
    }

    func testTheRealHostShowsOnlyWhatItCanDo() {
        var snapshot = SampleData.snapshot(for: .fiveDocuments)
        snapshot.host.can = [
            HostActionType.documentOpen: true,
            HostActionType.updateStart: false,
            HostActionType.syncSetPaused: false,
            HostActionType.requestAnswer: false,
        ]
        let model = AppModel(host: SampleHost(persona: .fiveDocuments), initial: snapshot)
        XCTAssertFalse(model.isSample)

        XCTAssertTrue(PanelControls.showsDocumentAction(HostActionType.documentOpen, in: model.snapshot))
        XCTAssertFalse(PanelControls.showsUpdate(can: model.can(HostActionType.updateStart)))
        XCTAssertFalse(PanelControls.showsSyncPause(can: model.can(HostActionType.syncSetPaused)))
        XCTAssertFalse(PanelControls.showsAnswer(can: model.can(HostActionType.requestAnswer)))
        XCTAssertFalse(PanelControls.showsExport(isSample: model.isSample))
    }

    func testARealHostWithNoSnapshotYetIsNotSample() {
        let relay = RelayHostClient()
        let model = AppModel(host: relay, initial: relay.last)

        XCTAssertFalse(model.isSample)
        XCTAssertFalse(PanelControls.showsExport(isSample: model.isSample))
        XCTAssertFalse(PanelControls.showsAppLog(isSample: model.isSample))
        XCTAssertTrue(PanelControls.showsDocumentAction(HostActionType.documentOpen, in: model.snapshot))
    }

    func testADecodedSnapshotIsNotSampleData() throws {
        var sample = SampleData.snapshot(for: .fiveDocuments)
        sample.isSample = true
        let data = try JSONEncoder().encode(sample)

        XCTAssertFalse(try JSONDecoder().decode(HostSnapshot.self, from: data).isSample)
    }

    func testKeptCopiesFollowsTheHostsCanList() {
        let sample = sampleModel()
        XCTAssertTrue(PanelControls.showsKeptCopies(isSample: sample.isSample, can: sample.can(HostActionType.appOpenKeptPage)))

        var snapshot = SampleData.snapshot(for: .fiveDocuments)
        snapshot.host.can = [HostActionType.appOpenKeptPage: true]
        var model = AppModel(host: SampleHost(persona: .fiveDocuments), initial: snapshot)
        XCTAssertTrue(PanelControls.showsKeptCopies(isSample: model.isSample, can: model.can(HostActionType.appOpenKeptPage)))

        snapshot.host.can = [HostActionType.appOpenKeptPage: false]
        model = AppModel(host: SampleHost(persona: .fiveDocuments), initial: snapshot)
        XCTAssertFalse(PanelControls.showsKeptCopies(isSample: model.isSample, can: model.can(HostActionType.appOpenKeptPage)))
    }

    func testTheRealHostHidesADocumentActionItsCanListRefuses() {
        var snapshot = SampleData.snapshot(for: .fiveDocuments)
        snapshot.host.can = [HostActionType.documentOpen: true, HostActionType.documentForget: false]
        let model = AppModel(host: SampleHost(persona: .fiveDocuments), initial: snapshot)

        XCTAssertTrue(PanelControls.showsDocumentAction(HostActionType.documentOpen, in: model.snapshot))
        XCTAssertFalse(PanelControls.showsDocumentAction(HostActionType.documentForget, in: model.snapshot))
    }
}

@MainActor
final class PanelWaitingKindsTests: XCTestCase {
    /// 08-sync.md 8.46: conflicts group by the folder they are in, whatever the
    /// item's detail holds, and a held removal earns the dot in the Folders pane.
    func testConflictsGroupByTheirFolderAndAHeldRemovalEarnsTheDot() {
        var snapshot = SampleData.snapshot(for: .syncedWithTeams)
        let folder = snapshot.folders[0]
        snapshot.waiting = [
            WaitingItem(id: "w-1", kind: .conflict, title: "a.html changed in both places", detail: "both-edited", subjectID: folder.id),
            WaitingItem(id: "w-2", kind: .conflict, title: "b.html changed in both places", detail: "remote-deleted", subjectID: folder.id),
        ]
        let groups = PanelControls.conflictGroups(in: snapshot)
        XCTAssertEqual(groups.map(\.folder), [folder.name])
        XCTAssertEqual(groups.map(\.count), [2])
        XCTAssertTrue(WaitingItem.Kind.removalHeld.earnsDot)
        XCTAssertEqual(WaitingItem.Kind.removalHeld.pane, .folders)
    }
}
