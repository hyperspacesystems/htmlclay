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

    func testTheRealHostHidesADocumentActionItsCanListRefuses() {
        var snapshot = SampleData.snapshot(for: .fiveDocuments)
        snapshot.host.can = [HostActionType.documentOpen: true, HostActionType.documentForget: false]
        let model = AppModel(host: SampleHost(persona: .fiveDocuments), initial: snapshot)

        XCTAssertTrue(PanelControls.showsDocumentAction(HostActionType.documentOpen, in: model.snapshot))
        XCTAssertFalse(PanelControls.showsDocumentAction(HostActionType.documentForget, in: model.snapshot))
    }
}
