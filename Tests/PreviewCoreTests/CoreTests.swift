import XCTest
@testable import PreviewCore

@MainActor
final class CoreTests: XCTestCase {
    func testEveryPersonaCountsAddUp() {
        for persona in Persona.allCases {
            let snapshot = SampleData.snapshot(for: persona)
            let sum = SettingsPane.allCases.map { snapshot.count(in: $0) }.reduce(0, +)
            XCTAssertEqual(sum, snapshot.totalCount, persona.rawValue)
        }
    }

    func testSnapshotsRoundTripAsJSON() throws {
        for persona in Persona.allCases {
            let snapshot = SampleData.snapshot(for: persona)
            let data = try JSONEncoder().encode(snapshot)
            XCTAssertEqual(try JSONDecoder().decode(HostSnapshot.self, from: data), snapshot)
        }
    }

    func testStopServingAlsoStopsSync() {
        let before = SampleData.snapshot(for: .syncedWithTeams)
        let after = SampleReducer.apply(.setServed(folderID: "panphora", on: false), to: before)
        XCTAssertEqual(after.folders.first?.served, false)
        XCTAssertEqual(after.folders.first?.synced, false)
    }

    func testDotClearsWhenSeenAndReturnsForANewItem() async throws {
        let host = SampleHost(persona: .trustedFolder)
        let model = AppModel(host: host, initial: host.current)
        XCTAssertTrue(model.showsDot)
        model.markWaitingSeen()
        XCTAssertFalse(model.showsDot)
        host.pretend(.syncedWithTeams)
        XCTAssertTrue(model.showsDot)
    }
}
