import Foundation

public enum SampleData {
    public static let trustCandidates = [
        TrustCandidate(name: "Documents", path: "~/Documents", documents: 23),
        TrustCandidate(name: "Notes", path: "~/Documents/Notes", documents: 4),
        TrustCandidate(name: "Recipes", path: "~/Documents/Recipes", documents: 7),
    ]
    public static let servePaths = ["~/Sites/prototypes", "~/hyperclay/panphora/apps", "~/Sites/blog", "~/HyperclayLocal/panphora"]
    public static let hyperclayLocalPaths = ["~/HyperclayLocal/panphora", "~/HyperclayLocal/Design Team"]
    public static let syncKey = "hcsk_4c1e9b2d07e67f3a"
    public static let programFiles = ["~/bin/export-csv.py", "~/bin/clay-search", "~/Downloads/search.sh"]
    public static let accountDownload = (documents: 214, size: "38 MB")
    public static let teamDownloadSteps = [(files: 0, size: "0"), (files: 8, size: "1.9"), (files: 14, size: "3.3"), (files: 18, size: "4.2")]
    public static let teamDownloadTotal = (files: 18, size: "4.2")
    public static let overlayTeamID = "design-team"
    public static let offlineChanges = 8
    public static let history = [HistoryEntry(id: "h1", kind: .uploaded, title: "Uploaded apps/index.html", detail: "", time: "13:58"), HistoryEntry(id: "h2", kind: .downloaded, title: "Downloaded blog/launch.html", detail: "changed by maria", time: "13:44"), HistoryEntry(id: "h3", kind: .renamed, title: "Renamed notes.html to meeting-notes.html", detail: "", time: "11:20"), HistoryEntry(id: "h4", kind: .settled, title: "Settled a conflict in about.html", detail: "kept both", time: "10:05")]

    public static let localMoveFolders = [
        LocalMoveFolder(id: "panphora", name: "panphora", port: 4321, address: "panphora.hyperclay.localhost:1212"),
        LocalMoveFolder(id: "design-team", name: "Design Team", port: 4322, address: "design-team.hyperclay.localhost:1212"),
    ]
    public static let localUnsavedDocument = "budget.html"
    public static let conflictsComingAlong = syncedWithTeams.waiting.filter { $0.kind == .conflict }.count
    public static let cantSaveDocument = "budget.htmlclay"
    public static let portProcess = "node"
    public static let freePort = 4545

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
        requests: [ProgramRequest(id: "req-todo", kind: .found, document: "todo.htmlclay", program: "todo-search", folder: "Notes", path: "~/bin/todo-search")],
        waiting: [WaitingItem(id: "w-req-todo", kind: .programRequest, title: "todo.htmlclay wants to run todo-search", detail: "Notes", subjectID: "req-todo")])

    static let servedFolder = HostSnapshot(
        documents: [
            RecentDocument(id: "index", name: "index.html", place: "~/GitHub/prototypes", openNow: true, opened: "Open now"),
            RecentDocument(id: "todo", name: "todo.htmlclay", place: "~/Documents/Notes", opened: "Yesterday"),
        ],
        folders: [notes, prototypes],
        programs: [
            Program(id: "search-sh", name: "search.sh", path: "~/bin/search.sh", allowedDocuments: ["notes.htmlclay"]),
            Program(id: "bluesky-studio", name: "bluesky-studio", path: "~/Applications/bluesky-studio", allowedDocuments: ["posts.htmlclay", "drafts.htmlclay"], allTrustedFolders: true),
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
        requests: [ProgramRequest(id: "req-todo", kind: .found, document: "todo.htmlclay", program: "todo-search", folder: "Notes", path: "~/bin/todo-search")],
        waiting: [
            WaitingItem(id: "w-conflict-index", kind: .conflict, title: "apps/index.html changed in both places", detail: "Design Team", subjectID: "design-team"),
            WaitingItem(id: "w-conflict-pricing", kind: .conflict, title: "pricing.html changed in both places", detail: "Design Team", subjectID: "design-team"),
            WaitingItem(id: "w-req-todo", kind: .programRequest, title: "todo.htmlclay wants to run todo-search", detail: "Notes", subjectID: "req-todo"),
        ],
        sync: SyncState(connected: true, username: "panphora", keySuffix: "7f3a", syncingFiles: 5, viewOnlyTeams: 1, lastFinished: "2 minutes ago", fullName: "David Miranda"),
        update: UpdateState(current: "2.0.3"),
        preferences: PreferenceState(yourName: "David", useName: true))

    static let localUser = HostSnapshot(localMoveOffer: true)

    public static func request(_ kind: ProgramRequest.Kind) -> (request: ProgramRequest, item: WaitingItem) {
        let request: ProgramRequest
        let title: String
        switch kind {
        case .found:
            request = ProgramRequest(id: "req-sample-found", kind: kind, document: "todo.htmlclay", program: "todo-search", folder: "Notes", path: "~/Documents/todo/tools/search.sh")
            title = "todo.htmlclay wants to run todo-search"
        case .noFile:
            request = ProgramRequest(id: "req-sample-noFile", kind: kind, document: "todo.htmlclay", program: "todo-search", folder: "Notes")
            title = "todo.htmlclay wants to run todo-search"
        case .teamDocument:
            request = ProgramRequest(id: "req-sample-teamDocument", kind: kind, document: "roadmap.html", program: "export-csv", folder: "Design Team", path: "~/bin/export-csv.py")
            title = "roadmap.html wants to run export-csv"
        case .riskyFile:
            request = ProgramRequest(id: "req-sample-riskyFile", kind: kind, document: "todo.htmlclay", program: "todo-search", folder: "Notes", path: "~/Downloads/search.sh", detail: "It's in Downloads and changed 2 minutes ago.")
            title = "todo.htmlclay wants to run todo-search"
        case .folderRead:
            request = ProgramRequest(id: "req-sample-folderRead", kind: kind, document: "budget.htmlclay", program: "", folder: "Receipts", path: "~/Documents/Receipts")
            title = "budget.htmlclay wants to read Receipts"
        case .folderReadAgain:
            request = ProgramRequest(id: "req-sample-folderReadAgain", kind: kind, document: "trip-plan.htmlclay", program: "", folder: "Trips", path: "~/Documents/Trips", detail: "3 documents")
            title = "trip-plan.htmlclay wants you to trust Trips"
        }
        let item = WaitingItem(id: "w-" + request.id, kind: .programRequest, title: title, detail: request.folder, subjectID: request.id)
        return (request, item)
    }
}
