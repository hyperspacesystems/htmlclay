import Foundation
import Observation

/// The host's history JSON (`GET /shell/v1/folders/<id>/history`), turned into
/// rows the page shows.
struct WireHistoryPage: Decodable {
    struct Entry: Decodable {
        var id: String
        var at: String
        var kind: String
        var path: String
    }
    var entries: [Entry]
    var next: String?

    func page(now: Date = Date(), calendar: Calendar = .current) -> HistoryPage {
        HistoryPage(entries: entries.compactMap { entry in
            guard let kind = HistoryEntry.Kind(rawValue: entry.kind) else { return nil }
            let at = FolderHistoryText.date(entry.at)
            return HistoryEntry(id: entry.id, kind: kind, title: Strings.History.entry(kind, entry.path), detail: "",
                                time: at.map { FolderHistoryText.time($0) } ?? "",
                                day: at.map { FolderHistoryText.day($0, now: now, calendar: calendar) } ?? "")
        }, next: next)
    }
}

enum FolderHistoryText {
    static func date(_ text: String) -> Date? {
        let fractional = ISO8601DateFormatter()
        fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return fractional.date(from: text) ?? ISO8601DateFormatter().date(from: text)
    }
    static func time(_ date: Date) -> String {
        date.formatted(date: .omitted, time: .shortened)
    }
    static func day(_ date: Date, now: Date, calendar: Calendar) -> String {
        calendar.isDate(date, inSameDayAs: now) ? Strings.History.today : date.formatted(date: .abbreviated, time: .omitted)
    }
}

/// What the folder history page holds: the first page when it opens, then older
/// pages as the person scrolls to the end. A page already asked for is not asked
/// for twice, and a row already shown is not shown twice.
@MainActor
@Observable
public final class FolderHistoryModel {
    public let folderID: String
    public private(set) var entries: [HistoryEntry] = []
    public private(set) var loaded = false
    public private(set) var loading = false
    private var next: String?
    private let host: HostClient

    public init(folderID: String, host: HostClient) {
        self.folderID = folderID
        self.host = host
    }

    public var hasMore: Bool { next != nil }

    /// The entries in runs of one day, newest first.
    public static func days(_ entries: [HistoryEntry]) -> [(day: String, entries: [HistoryEntry])] {
        var runs: [(day: String, entries: [HistoryEntry])] = []
        for entry in entries {
            if let last = runs.indices.last, runs[last].day == entry.day {
                runs[last].entries.append(entry)
            } else {
                runs.append((day: entry.day, entries: [entry]))
            }
        }
        return runs
    }

    public func loadFirst() async {
        guard !loaded, !loading else { return }
        await load(before: nil)
    }

    public func loadMore() async {
        guard loaded, !loading, let next else { return }
        await load(before: next)
    }

    private func load(before: String?) async {
        loading = true
        defer { loading = false }
        guard let page = await host.folderHistory(folderID, before: before) else {
            loaded = true
            return
        }
        let seen = Set(entries.map(\.id))
        entries += page.entries.filter { !seen.contains($0.id) }
        next = page.next
        loaded = true
    }
}
