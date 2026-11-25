import XCTest
@testable import PreviewCore

/// A host that answers host.setPort with each queued outcome in turn, and records what it got.
@MainActor
private final class PortHost: HostClient {
    var answers: [HostOutcome] = []
    private(set) var performed: [HostAction] = []

    func load() async throws -> HostSnapshot { HostSnapshot() }
    func perform(_ action: HostAction) async -> HostOutcome {
        performed.append(action)
        return answers.isEmpty ? .done(HostSnapshot()) : answers.removeFirst()
    }
    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .failure(HostRefusal(code: "unsupported", text: "")) }
    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription { HostSubscription {} }
}

@MainActor
final class ChangePortModelTests: XCTestCase {
    func testRestartStopsWorkTurnsTheButtonIntoRestartAnywayAndSendsItConfirmed() async {
        let host = PortHost()
        host.answers = [.refused(HostRefusal(code: HostRefusal.restartStopsWork, text: "stops work", running: ["work"]))]
        let sheet = ChangePortModel(model: AppModel(host: host, initial: HostSnapshot()))
        sheet.setText("2020")

        let first = await sheet.save()
        XCTAssertFalse(first)
        XCTAssertEqual(sheet.stopsWork, ["work"])
        XCTAssertEqual(sheet.actionTitle, Strings.Updates.restartAnyway)
        sheet.setText("2020x")
        XCTAssertEqual(sheet.actionTitle, Strings.Updates.restartAnyway, "a key that adds no digit dropped the Restart anyway step")

        let second = await sheet.save()
        XCTAssertTrue(second)
        XCTAssertEqual(host.performed, [.setPort(2020), .setPort(2020, confirmed: true)])
    }

    func testTypingAnotherPortAsksAgainBeforeRestartingAnyway() async {
        let host = PortHost()
        host.answers = [.refused(HostRefusal(code: HostRefusal.restartStopsWork, text: "stops work", running: ["sync"]))]
        let sheet = ChangePortModel(model: AppModel(host: host, initial: HostSnapshot()))
        sheet.setText("2020")
        _ = await sheet.save()
        sheet.setText("2021")
        XCTAssertEqual(sheet.actionTitle, Strings.Advanced.changePortAction)
        _ = await sheet.save()
        XCTAssertEqual(host.performed.last, .setPort(2021))
    }
}
