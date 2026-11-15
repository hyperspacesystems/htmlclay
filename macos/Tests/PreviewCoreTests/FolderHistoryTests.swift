import XCTest
@testable import PreviewCore

/// A host that answers history pages from a map of cursor to page, and records the cursors asked for.
@MainActor
private final class HistoryHost: HostClient {
    var pages: [String: HistoryPage] = [:]
    private(set) var asked: [String] = []

    func load() async throws -> HostSnapshot { HostSnapshot() }
    func perform(_ action: HostAction) async -> HostOutcome { .done(HostSnapshot()) }
    func check(_ action: HostAction) async -> Result<HostCheckAnswer, HostRefusal> { .failure(HostRefusal(code: "unsupported", text: "")) }
    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) -> HostSubscription { HostSubscription {} }
    func folderHistory(_ folderID: String, before: String?) async -> HistoryPage? {
        asked.append(before ?? "")
        return pages[before ?? ""]
    }
}

private func entry(_ id: String, day: String = "Today") -> HistoryEntry {
    HistoryEntry(id: id, kind: .uploaded, title: "Uploaded \(id).html", detail: "", time: "13:58", day: day)
}

@MainActor
final class FolderHistoryTests: XCTestCase {
    func testTheHostsJSONBecomesRowsAndUnknownKindsAreLeftOut() throws {
        let json = """
        {"entries": [
          {"id": "a", "at": "2026-11-15T13:58:01.5Z", "kind": "uploaded", "path": "apps/index.html", "detail": "upload"},
          {"id": "b", "at": "2026-11-14T09:00:00Z", "kind": "renamed", "path": "notes.html", "detail": "relocate"},
          {"id": "c", "at": "2026-11-14T08:00:00Z", "kind": "someday", "path": "x.html", "detail": ""}
        ], "next": "b"}
        """
        let wire = try JSONDecoder().decode(WireHistoryPage.self, from: Data(json.utf8))
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        let page = wire.page(now: FolderHistoryText.date("2026-11-15T20:00:00Z")!, calendar: calendar)
        XCTAssertEqual(page.entries.map(\.id), ["a", "b"])
        XCTAssertEqual(page.entries.map(\.kind), [.uploaded, .renamed])
        XCTAssertEqual(page.entries[0].title, Strings.History.entry(.uploaded, "apps/index.html"))
        XCTAssertEqual(page.entries[0].day, Strings.History.today)
        XCTAssertNotEqual(page.entries[1].day, Strings.History.today)
        XCTAssertFalse(page.entries[0].time.isEmpty)
        XCTAssertEqual(page.next, "b")
    }

    func testOlderPagesFollowTheCursorAndARowIsNeverShownTwice() async {
        let host = HistoryHost()
        host.pages = [
            "": HistoryPage(entries: [entry("3"), entry("2")], next: "2"),
            "2": HistoryPage(entries: [entry("2"), entry("1", day: "Nov 14, 2026")], next: nil),
        ]
        let history = FolderHistoryModel(folderID: "0dd3adfe2b553e75", host: host)
        await history.loadFirst()
        XCTAssertEqual(history.entries.map(\.id), ["3", "2"])
        XCTAssertTrue(history.hasMore)
        await history.loadMore()
        XCTAssertEqual(history.entries.map(\.id), ["3", "2", "1"])
        XCTAssertFalse(history.hasMore)
        await history.loadMore()
        await history.loadFirst()
        XCTAssertEqual(host.asked, ["", "2"], "nothing is asked for twice")
        XCTAssertEqual(FolderHistoryModel.days(history.entries).map(\.day), ["Today", "Nov 14, 2026"])
        XCTAssertEqual(FolderHistoryModel.days(history.entries).map(\.entries.count), [2, 1])
    }

    func testAHostWithNoHistoryShowsAnEmptyPage() async {
        let history = FolderHistoryModel(folderID: "0dd3adfe2b553e75", host: HistoryHost())
        await history.loadFirst()
        XCTAssertTrue(history.loaded)
        XCTAssertTrue(history.entries.isEmpty)
        XCTAssertFalse(history.hasMore)
    }
}
