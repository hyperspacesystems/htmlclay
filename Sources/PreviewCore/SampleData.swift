import Foundation

public enum SampleData {
    public static func snapshot(for persona: Persona) -> HostSnapshot {
        switch persona {
        case .newPerson: return HostSnapshot()
        case .fiveDocuments: return fiveDocuments
        case .trustedFolder: return trustedFolder
        case .servedFolder: return servedFolder
        case .syncedWithTeams: return syncedWithTeams
        case .localUser: return localUser
        }
    }

    static let fiveDocuments = HostSnapshot(documents: [
        RecentDocument(id: "budget", name: "budget.htmlclay", place: "~/Downloads", openNow: true, opened: "Open now"),
        RecentDocument(id: "recipe-box", name: "recipe-box.htmlclay", place: "~/Documents", fileMissing: true, versionsKept: 9, opened: "Yesterday"),
        RecentDocument(id: "trip-plan", name: "trip-plan.htmlclay", place: "~/Documents", opened: "Yesterday"),
        RecentDocument(id: "reading-list", name: "reading-list.htmlclay", place: "~/Desktop", opened: "Monday"),
        RecentDocument(id: "invoice", name: "invoice.htmlclay", place: "~/Documents/Work", opened: "Nov 3"),
    ])

    static let notes = Folder(id: "notes", name: "Notes", path: "~/Documents/Notes", documentCount: 4, status: "~/Documents/Notes")
    static let journal = Folder(id: "journal", name: "Journal", path: "~/Documents/Notes/Journal", parentID: "notes", documentCount: 6, status: "Inside Notes")
    static let clips = Folder(id: "clips", name: "Clips", path: "~/Documents/Notes/Clips", parentID: "notes", documentCount: 3, status: "Inside Notes")
    static let prototypes = Folder(id: "prototypes", name: "prototypes", path: "~/Sites/prototypes", served: true, address: "prototypes.hyperclay.localhost:1212", documentCount: 12, status: "only on this Mac", versionsSize: "6 MB")
    static let todoSearch = Program(id: "todo-search", name: "todo-search", path: "~/bin/todo-search")

    static let trustedFolder = HostSnapshot(
        documents: [
            RecentDocument(id: "todo", name: "todo.htmlclay", place: "~/Documents/Notes", openNow: true, opened: "Open now"),
            RecentDocument(id: "budget", name: "budget.htmlclay", place: "~/Downloads", opened: "Yesterday"),
        ],
        folders: [notes, journal, clips],
        programs: [todoSearch],
        requests: [ProgramRequest(id: "req-todo", kind: .found, document: "todo.htmlclay", program: "todo-search", folder: "Notes")],
        waiting: [WaitingItem(id: "w-req-todo", kind: .programRequest, title: "todo.htmlclay wants to run todo-search", detail: "Notes", subjectID: "req-todo")])

    static let servedFolder = HostSnapshot(
        documents: [
            RecentDocument(id: "index", name: "index.html", place: "~/GitHub/prototypes", openNow: true, opened: "Open now"),
            RecentDocument(id: "todo", name: "todo.htmlclay", place: "~/Documents/Notes", opened: "Yesterday"),
        ],
        folders: [notes, prototypes],
        programs: [
            Program(id: "search-sh", name: "search.sh", path: "~/bin/search.sh", allowedDocuments: ["notes.htmlclay"]),
            Program(id: "bluesky-studio", name: "bluesky-studio", path: "~/Applications/bluesky-studio", allowedDocuments: ["posts.htmlclay", "drafts.htmlclay"]),
            Program(id: "clay-search", name: "clay-search", path: "~/bin/clay-search", fileMissing: true, allowedDocuments: ["index.html"]),
        ],
        waiting: [WaitingItem(id: "w-clay-search", kind: .programFileMissing, title: "clay-search can't be found", detail: "~/bin/clay-search", subjectID: "clay-search")])

    static let syncedWithTeams = HostSnapshot(
        documents: [
            RecentDocument(id: "index", name: "index.html", place: "panphora", openNow: true, opened: "Open now"),
            RecentDocument(id: "pricing", name: "pricing.html", place: "Design Team", opened: "Today"),
            RecentDocument(id: "todo", name: "todo.htmlclay", place: "~/Documents/Notes", opened: "Yesterday"),
        ],
        folders: [
            notes,
            prototypes,
            Folder(id: "panphora", name: "panphora", path: "~/hyperclay/panphora", origin: .account(username: "panphora"), served: true, synced: true, address: "panphora.hyperclay.localhost:1212", documentCount: 214, status: "up to date", versionsSize: "48 MB", lastSync: "Up to date · 2 minutes ago", historySummary: "12 uploaded, 3 downloaded today"),
            Folder(id: "design-team", name: "Design Team", path: "~/hyperclay/Design Team", origin: .team(name: "Design Team"), served: true, synced: true, address: "design-team.hyperclay.localhost:1212", documentCount: 18, status: "you are an editor", versionsSize: "21 MB", lastSync: "2 documents changed in both places", historySummary: "4 uploaded, 9 downloaded today"),
            Folder(id: "marketing", name: "Marketing", path: "", origin: .team(name: "Marketing"), status: "Not on this Mac", onThisMac: false),
        ],
        programs: [todoSearch, Program(id: "search-sh", name: "search.sh", path: "~/bin/search.sh", allowedDocuments: ["notes.htmlclay"])],
        requests: [ProgramRequest(id: "req-todo", kind: .found, document: "todo.htmlclay", program: "todo-search", folder: "Notes")],
        waiting: [
            WaitingItem(id: "w-conflict-index", kind: .conflict, title: "apps/index.html changed in both places", detail: "Design Team", subjectID: "design-team"),
            WaitingItem(id: "w-conflict-pricing", kind: .conflict, title: "pricing.html changed in both places", detail: "Design Team", subjectID: "design-team"),
            WaitingItem(id: "w-req-todo", kind: .programRequest, title: "todo.htmlclay wants to run todo-search", detail: "Notes", subjectID: "req-todo"),
        ],
        sync: SyncState(connected: true, username: "panphora", keySuffix: "7f3a", syncingFiles: 5, viewOnlyTeams: 1),
        update: UpdateState(current: "2.0.3", ready: "2.1.0"),
        preferences: PreferenceState(yourName: "David"))

    static let localUser = HostSnapshot(localMoveOffer: true)
}
