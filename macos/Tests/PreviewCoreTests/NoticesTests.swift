import XCTest
@testable import PreviewCore

@MainActor
private final class PagedHost: HostClient {
    func load() async throws -> HostSnapshot { HostSnapshot() }
    func perform(_ action: HostAction) async -> HostOutcome { .done(HostSnapshot()) }
    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .failure(HostRefusal(code: "unsupported", text: "")) }
    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription { HostSubscription {} }
    func folderHistory(_ folderID: String, before: String?) async -> HistoryPage? {
        HistoryPage(entries: [HistoryEntry(id: folderID, kind: .settled, title: "", detail: "", time: "")], next: nil)
    }
}

/// 08-sync.md 8.45: the C6 notice frame, what each card says and where its button goes.
@MainActor
final class NoticesTests: XCTestCase {
    private func line(_ json: String) -> Data { Data(json.utf8) }

    func testTheStreamPassesKnownNoticesAndLeavesTheRest() throws {
        var state = HostStreamState()
        XCTAssertEqual(try state.receive(line: line(#"{"type":"notice","id":"keyStopped:2026-11-15T10:00:00Z","kind":"keyStopped","folderID":null,"name":"","detail":""}"#)),
                       [.notice(HostNotice(id: "keyStopped:2026-11-15T10:00:00Z", kind: .keyStopped))])
        XCTAssertEqual(try state.receive(line: line(#"{"type":"notice","id":"addedToTeam:7","kind":"addedToTeam","folderID":"0dd3adfe2b553e75","name":"Marketing","detail":""}"#)),
                       [.notice(HostNotice(id: "addedToTeam:7", kind: .addedToTeam, folderID: "0dd3adfe2b553e75", name: "Marketing"))])
        XCTAssertEqual(try state.receive(line: line(#"{"type":"notice","id":"n","kind":"later"}"#)), [])
        XCTAssertEqual(try state.receive(line: line(#"{"type":"notice","kind":"conflict"}"#)), [])
        XCTAssertEqual(state.rev, -1, "a notice is not a snapshot")
    }

    func testAConflictCardOpensNeedsYouInTheBrowser() {
        XCTAssertEqual(HostNotice(id: "c", kind: .conflict).opens, .dashboard(route: "/needs-you"))
        XCTAssertEqual(HostNotice(id: "k", kind: .keyStopped).opens, .settings(.sync))
        XCTAssertEqual(HostNotice(id: "t", kind: .addedToTeam).opens, .settings(.folders))
    }

    func testEachCardNamesWhatItIsAbout() {
        let conflict = HostNotice(id: "c", kind: .conflict, folderID: "f", name: "apps/index.html")
        XCTAssertTrue(Strings.Notify.title(conflict, folder: "Design Team").hasPrefix("Design Team: apps/index.html"))
        XCTAssertTrue(Strings.Notify.title(conflict, folder: nil).hasPrefix("apps/index.html"))
        XCTAssertTrue(Strings.Notify.title(HostNotice(id: "t", kind: .addedToTeam, name: "Marketing"), folder: nil).contains("Marketing"))
        for kind in [HostNotice.Kind.conflict, .keyStopped, .addedToTeam] {
            let notice = HostNotice(id: "n", kind: kind)
            XCTAssertFalse(Strings.Notify.body(notice).isEmpty)
            XCTAssertFalse(Strings.Notify.open(notice).isEmpty)
        }
    }

    func testTheRelayForwardsFolderHistory() async {
        let relay = RelayHostClient()
        let before = await relay.folderHistory("0dd3adfe2b553e75", before: nil)
        XCTAssertNil(before, "no host attached yet")
        await relay.attach(PagedHost()) { _ in }
        let page = await relay.folderHistory("0dd3adfe2b553e75", before: nil)
        XCTAssertEqual(page?.entries.map(\.id), ["0dd3adfe2b553e75"])
    }
}
