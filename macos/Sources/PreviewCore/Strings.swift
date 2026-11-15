import Foundation

public enum Strings {
    public enum App {
        public static let name = "HTML Clay Preview"
    }
    public enum Panel {
        public static let openDashboard = "Open dashboard"
        public static let recentDocuments = "Recent documents"
        public static let noDocumentsTitle = "No documents yet"
        public static let noDocumentsSub = "Double-click any .htmlclay file in Finder. It shows up here."
        public static let openAFile = "Open a file…"
        public static let startFromTemplate = "Start from a template"
        public static let review = "Review"
        public static let answer = "Answer"
        public static let pauseSyncing = "Pause syncing"
        public static let resumeSyncing = "Resume syncing"
        public static let syncPaused = "Syncing paused"
        public static let wantsSuffix = " wants"
        private static let homePrefix = "~/"
        public static func conflicts(folder: String, count: Int) -> String {
            count == 1 ? "\(folder): 1 document changed in both places" : "\(folder): \(count) documents changed in both places"
        }
        public static func wantsProgram(document: String) -> String { "\(document) wants to run a program" }
        public static func fileMissing(versions: Int) -> String { "File moved or deleted · \(versions) versions kept" }
        public static func documentSubtitle(place: String, opened: String) -> String { "\(droppingHome(place)) · \(opened)" }
        public static func syncing(_ count: Int, username: String) -> String { "Syncing \(count) files… · \(username)" }
        public static func upToDate(_ username: String) -> String { "Up to date · \(username)" }
        public static func syncLine(_ sync: SyncState) -> String {
            if sync.paused { return syncPaused }
            if sync.syncingFiles > 0 { return syncing(sync.syncingFiles, username: sync.username) }
            return upToDate(sync.username)
        }
        private static func droppingHome(_ place: String) -> String {
            place.hasPrefix(homePrefix) ? String(place.dropFirst(homePrefix.count)) : place
        }
    }
    public enum Recover {
        public static let title = "Recover missing file"
        public static let save = "Recover"
    }
    public enum Menu {
        public static let settings = "Settings…"
        public static let checkForUpdates = "Check for updates…"
        public static let quit = "Quit HTML Clay"
        public static let rowOpen = "Open"
        public static let versionHistory = "Version history…"
        public static let showInFinder = "Show in Finder"
        public static let exportZip = "Export as zip…"
        public static let recoverMissing = "Recover missing file…"
        public static let removeFromRecent = "Remove from Recent"
        public static let keptCopies = "Kept Copies…"
        public static func needsYou(_ count: Int) -> String { "Needs you (\(count))" }
    }
    public enum Welcome {
        public static let title = "HTML Clay is ready"
        public static let lead = "Now you can open .htmlclay files on this Mac. Double-click one: it opens in your browser, and your changes save back into the file."
        public static let filesTitle = "Files that save themselves"
        public static let filesSub = "Edit in the browser. The file on your Mac updates as you go."
        public static let versionsTitle = "Versions you can go back to"
        public static let versionsSub = "HTML Clay keeps earlier versions of each document."
        public static let localTitle = "Stays on your Mac"
        public static let localSub = "No account needed. It works offline."
        public static let moreTitle = "When you want more"
        public static let moreSub = "Serve a folder as a local site, or sync with hyperclay.com. Both are in Settings."
        public static let allSet = "I'm all set"
        public static let openSettings = "Open Settings"
        public static let menuBarHint = "HTML Clay lives in your menu bar:"
    }
    public enum Settings {
        public static func title(_ pane: SettingsPane) -> String {
            switch pane {
            case .general: return "General"
            case .folders: return "Folders"
            case .programs: return "Programs"
            case .sync: return "Sync"
            case .yourName: return "Your name"
            case .updates: return "Updates"
            case .advanced: return "Advanced"
            }
        }
    }
    public enum Folders {
        public static let title = "Folders"
        public static let elbow = "└"
        public static let trustMenu = "Trust a folder…"
        public static let serveMenu = "Serve a folder…"
        public static let connectMenu = "Connect hyperclay.com…"
        public static let openHyperclayNote = "Build 1: this would open your folders on hyperclay.com."
        public static let emptyTitle = "Trust a folder when its documents belong together"
        public static let emptySub = "Documents in a trusted folder open without asking and can link to each other."
        public static let serveLink = "Building a site or an app? Serve a folder with Hyperclay Local…"
        public static let moveTitle = "Move from Hyperclay Local"
        public static let moveSub = "Your Hyperclay Local folders were found. Bring them into HTML Clay."
        public static let move = "Move…"
        public static let onThisMac = "On this Mac"
        public static let onlyOnThisMac = "only on this Mac"
        public static let setUp = "Set up…"
        public static let openHyperclay = "Open hyperclay.com ↗"
        public static func copied(_ address: String) -> String { "Copied \(address) to the clipboard." }
        public static func accountGroup(username: String) -> String { "hyperclay.com · @\(username)" }
        public static func inside(parent: String, path: String) -> String { "Inside \(parent) · \(path)" }
        public static func viewOnlyTeams(_ count: Int) -> String {
            count == 1 ? "1 team you can only view" : "\(count) teams you can only view"
        }
    }
    public enum Sync {
        public static let title = "Sync with hyperclay.com"
        public static let invite = "Keep a folder in step with your hyperclay.com account or your team. Nothing syncs until you choose a folder."
        public static let connect = "Connect hyperclay.com…"
        public static let replaceKey = "Replace key…"
        public static let disconnect = "Disconnect…"
        public static let syncing = "Syncing"
        public static let folders = "Folders"
        public static let syncingWord = "syncing"
        public static let setUp = "Set up…"
        public static let keyRefusedTitle = "Your sync key no longer works"
        public static let keyRefusedSub = "All folders stopped syncing. Your files are safe on this Mac."
        public static let reconnect = "Reconnect…"
        public static let wouldReplaceKey = "Build 1: this would ask for a new sync key."
        // TODO-COPY: Sync pane, when the key is kept in a file because there is no system keychain, max 90 chars
        public static let keyInFile = "Your sync key is kept in a file only you can read, because this computer has no keychain."
        public static let wouldOpenSite = "Build 1: this would open hyperclay.com in your browser."
        public static func account(_ username: String) -> String { "@\(username)" }
        public static func connected(keySuffix: String) -> String { "Connected · sync key ending \(keySuffix)" }
        public static func syncingFiles(_ n: Int, lastFinished: String) -> String {
            let base = "Syncing \(n) files…"
            return lastFinished.isEmpty ? base : base + " · last finished \(lastFinished)"
        }
        public static func upToDate(lastFinished: String) -> String {
            lastFinished.isEmpty ? "Up to date" : "Up to date · last finished \(lastFinished)"
        }
        public static func folderLine(path: String, state: String) -> String { "\(path) · \(state)" }
        public static func review(_ n: Int) -> String { "Review \(n) in dashboard" }
    }
    public enum FolderPage {
        public static let trusted = "Trusted"
        public static let trustedSubNotServed = ".htmlclay files here save, link to each other, and keep one address. They can also change each other."
        public static let trustedSubServed = "Documents here save, link to each other, and can change each other. Turning off asks first and removes this folder."
        public static let serve = "Serve on this Mac"
        public static let serveSubOff = "Gives a web address on this Mac, and lets .html files save too"
        public static let serveSubOn = "Local web address and .html saving enabled."
        public static let serveSubOnlyOnThisMac = " · Only on this Mac"
        public static let address = "Address"
        public static let framing = "Let other sites embed pages"
        public static let framingSub = "Other websites can show these pages inside their own pages."
        public static let framingNote = "Turn this on only for pages you mean to share with another site. You can turn it back off only in the Mac app."
        public static let copy = "Copy"
        public static let versions = "Versions and uploads"
        public static let show = "Show"
        public static let sync = "Sync with hyperclay.com"
        public static let syncSub = "Turning off disconnects sync while keeping files served on this Mac."
        public static let onlyOnThisMac = "Only on this Mac"
        public static let onlyOnThisMacSub = "Connect your hyperclay.com account to sync folders."
        public static let connect = "Connect…"
        public static let folderHistory = "Folder history"
        public static let cancel = "Cancel"
        public static let stopTrusting = "Stop trusting"
        public static let stopServing = "Stop serving"
        public static let disconnect = "Disconnect"
        public static func pathLine(path: String, documents: Int) -> String { "\(path) · \(documents) documents" }
        public static func childrenTitle(count: Int, parent: String) -> String { "\(count) trusted folders inside \(parent) keep their own address" }
        public static func childrenSub(parent: String) -> String { "Documents in them stay separate from \(parent) and keep their own browser data." }
        public static func mergeButton(parent: String) -> String { "Merge into \(parent)…" }
        public static func versionsSub(size: String) -> String { "Kept in this folder · \(size)" }
        public static func versionsNote(folder: String) -> String { "Build 1: this would list the versions and uploads kept in \(folder)." }
        public static func accountTitle(username: String) -> String { "@\(username)" }
        public static func stopTrustingTitle(_ name: String) -> String { "Stop trusting \(name)?" }
        public static func stopTrustingMessage(_ name: String) -> String { "Documents open from \(name) stop saving now, and \(name) leaves your Folders list. Files there save again only when you open them yourself." }
        public static func stopServingTitle(_ name: String) -> String { "Stop serving \(name)?" }
        public static func stopServingMessage(_ name: String) -> String { "Its address stops working and .html files there stop saving. The folder stays trusted and keeps its name for later." }
        public static func disconnectTitle(_ name: String) -> String { "Disconnect \(name)?" }
        public static func disconnectMessage(_ name: String, team: Bool) -> String {
            let base = "Your files stay on this Mac and keep being served. Changes no longer sync."
            return team ? base + " Programs keep treating it as a team folder." : base
        }
        public static func mergeTitle(child: String, parent: String) -> String { "Merge \(child) into \(parent)?" }
        public static func mergeMessage(child: String, parent: String, documents: Int) -> String {
            "\(child)'s \(documents) documents will move to \(parent)'s address, where documents in \(parent) can link to and change each other. Anything those documents saved in the browser stays behind at \(child)'s old address. Your files on disk stay right where they are."
        }
        public static func mergeAction(parent: String) -> String { "Merge into \(parent)" }
        public static func relinkTitle(_ name: String) -> String { "Find \(name)?" }
        public static let relinkLead = "Choose where this folder moved. It keeps its id, name, and address."
    }
    public enum Programs {
        public static let addProgram = "Add program…"
        public static let intro = "A document can ask HTML Clay to run a program on this Mac. You answer each request in HTML Clay."
        public static let waitingForYou = "Waiting for you"
        public static let programs = "Programs"
        public static let answer = "Answer…"
        public static let findIt = "Find it…"
        public static let allTrusted = "All documents in trusted folders"
        public static let noneYet = "No document may run it until you allow one."
        public static func wants(document: String, program: String) -> String { "\(document) wants “\(program)”" }
        public static func mayRun(_ document: String) -> String { "\(document) may run it" }
        public static func mayRunCount(_ count: Int) -> String { "\(count) documents may run it" }
        public static func fileMissing(_ count: Int) -> String {
            count == 1 ? "file missing · 1 document uses it" : "file missing · \(count) documents use it"
        }
        public static func wouldFind(_ name: String) -> String { "Build 1: this would ask you where \(name) went." }
        public static func aiEditAllowedFor(_ document: String) -> String { "Allowed for \(document)" }
        public static func aiEditAllowedCount(_ count: Int) -> String { "Allowed for \(count) documents" }
        public static func aiEditWaiting(document: String) -> String { "Allow AI editing for \(document)?" }
    }
    public enum ProgramPage {
        public static let runsAsYou = "This program runs as you. It can read and change any file you can."
        public static let whoMayRun = "Who may run it"
        public static let aiEditNote = "An AI agent on this computer rewrites selected text when you press ⌘J. Turn AI editing on or off in HTML Clay's settings."
        public static let allTrustedTitle = "All documents in my trusted folders"
        public static let allTrustedSub = "Not for team folders. Those documents always ask one at a time."
        public static let removeAccess = "Remove access"
        public static let accessRemoved = "Access removed"
        public static let allowAgain = "Allow again…"
        public static let chooseAnotherFile = "Choose another file…"
        public static let removeAllAccess = "Remove all access"
        public static let showInFinder = Menu.showInFinder
        public static func pathLine(path: String, ready: Bool, name: String) -> String {
            "\(path) · \(ready ? "ready" : "file missing") · documents ask for it as “\(name)”"
        }
    }
    public enum General {
        public static let title = "General"
        public static let openAtLogin = "Open HTML Clay at login"
        public static let aiTitle = "AI editing"
        public static let aiSub = "Rewrites text you select, using an AI tool on this Mac. The selection goes to that tool's provider."
        public static let appearanceTitle = "Appearance"
        public static let appearanceSub = "Controls HTML Clay's Mac windows and the browser dashboard."
        public static let appearanceSystem = "System"
        public static let appearanceLight = "Light"
        public static let appearanceDark = "Dark"
        public static let dashboard = "Dashboard"
        public static let dashboardAddress = "htmlclay.localhost:1212"
        public static let open = "Open"
        public static let welcome = "Welcome window"
        public static let showAgain = "Show again"
        public static let documentsLine = "Documents open in your default browser."
    }
    public enum YourName {
        public static let useName = "Use my name in documents"
        public static let offSub = "Documents can show who made a change."
        public static let name = "Name"
        public static let accountSub = "Your hyperclay.com name"
        public static let change = "Change on hyperclay.com ↗"
        public static let note = "Documents can save your name. Anyone you send them to can read it."
        public static let wouldChange = "Build 1: this would open your hyperclay.com profile."
    }
    public enum Updates {
        public static func ready(_ version: String) -> String { "HTML Clay \(version) is ready" }
        public static func youHave(_ version: String, checked: String) -> String { "You have \(version) · checked \(checked)" }
        public static let restart = "Restart to update"
        public static func current(_ version: String) -> String { "HTML Clay \(version)" }
        public static func upToDate(checked: String) -> String { "Up to date · checked \(checked)" }
        public static let checkNow = "Check now"
        public static let automatic = "Update automatically"
        public static let restartNote = "HTML Clay finishes saves and sync before it restarts."
        public static let whatsNew = "What's new"
        public static let versionDot = "."
        public static func wouldRestart(_ version: String) -> String { "Build 1: this would restart HTML Clay into \(version)." }
        public static let wouldCheck = "Build 1: this would check for a new version."
        public static let wouldShowWhatsNew = "Build 1: this would show what changed in the new version."
    }
    public enum Advanced {
        public static let port = "Port"
        public static let portSub = "Every address on this Mac uses it. Changing it resets page storage, so HTML Clay only asks when 1212 is taken."
        public static let portNumber = "1212"
        public static let liveService = "Live service"
        public static let liveServiceSub = "Runs served and synced folders"
        public static let running = "Running"
        public static let starting = "Starting"
        public static let stopped = "Stopped"
        public static let off = "Not running"
        public static let storage = "Storage"
        public static let versions = "Versions of documents outside served folders"
        public static let versionsSize = "240 MB"
        public static let uploads = "Uploads"
        public static let uploadsSize = "1.2 GB"
        public static let showInFinder = Menu.showInFinder
        public static let troubleshooting = "Troubleshooting"
        public static let logs = "Logs"
        public static let showAppLog = "Show app log"
        public static let showSyncLog = "Show sync log"
        public static let stopOpening = "Stop opening .htmlclay files with HTML Clay"
        public static let remove = "Remove…"
        public static let wouldShowVersions = "Build 1: this would show the versions folder in Finder."
        public static let wouldShowUploads = "Build 1: this would show the uploads folder in Finder."
        public static let wouldShowAppLog = "Build 1: this would open the app log."
        public static let wouldShowSyncLog = "Build 1: this would open the sync log."
        public static let wouldRemoveFileType = "Build 1: this would ask before HTML Clay stops opening .htmlclay files."
    }
    public enum Request {
        public static func title(_ document: String) -> String { "Let \(document) run a program?" }
        public static let dontAllow = "Don't allow"
        public static let allow = "Allow"
        public static let windowTitle = "HTML Clay"
        public static func counter(_ position: Int, _ total: Int) -> String { "\(position) of \(total)" }
        public static let ended = "This request ended"
        public static func callsIt(_ program: String) -> String { "The document calls it “\(program)”." }
        public static let foundHere = "Found in a folder where you keep programs."
        public static let runsAsYou = "A program runs as you. It can read, change or delete any file you can, and use the internet. Allow only a program you trust."
        public static let moreOptions = "More options"
        public static func only(_ document: String) -> String { "Only \(document)" }
        public static func allowFile(_ name: String) -> String { "Allow \(name)" }
        public static func runNamed(document: String, program: String) -> String { "Let \(document) run “\(program)”?" }
        public static func useAIEditing(document: String) -> String { "Let \(document) use AI editing?" }
        public static func aiEditTeamLead(_ folder: String) -> String { "This document is in \(folder), a team folder where others can edit documents later." }
        public static let noFileLead = "HTML Clay didn't find this program on your Mac. The document should come with instructions. If you're not sure, choose Don't allow: the document still opens and saves."
        public static let chooseAFile = "Choose a file…"
        public static func fromTeam(_ folder: String) -> String { "From \(folder). Its editors can change this document later." }
        public static let teamAsksEachTime = "Team documents always ask one at a time."
        public static let allowForThisDocument = "Allow for this document"
        public static let useThisFile = "Use this file?"
        public static let riskyNote = "Programs that just arrived are the most common way to be tricked."
        public static let chooseAnother = "Choose another"
        public static let allowAnyway = "Allow anyway"
        public static func readFolder(_ document: String) -> String { "Let \(document) read a folder?" }
        public static func readNote(_ folder: String) -> String { "Allow once lasts until you close the document. Trust lets every document in \(folder) save and link." }
        public static let allowOnce = "Allow once"
        public static let trustThisFolder = "Trust this folder"
        public static func trustOwnFolder(_ document: String) -> String { "Trust the folder \(document) is in?" }
        public static let ownFolderNote = "Documents there will save, link to each other and keep one address."
        public static func trustNamed(_ folder: String) -> String { "Trust \(folder)" }
        public static func folderLine(path: String, detail: String) -> String {
            guard let sentence = detailText(detail) else { return path }
            return "\(path) \u{00b7} \(sentence)"
        }
        public static let mainFolders = ["~", "~/Desktop", "~/Documents", "~/Downloads"]
        /// The sentence for one of the host's detail codes, or the prose sample
        /// data sends instead. nil when there is nothing to say.
        public static func detailText(_ detail: String) -> String? {
            if detail.isEmpty { return nil }
            switch detail {
            case "in-downloads":
                return "It's in Downloads, where files from the internet land."
            case "in-temp":
                return "It's in the temporary folder, where installers leave files."
            case "just-arrived":
                return "It changed a moment ago."
            case "downloaded":
                return "It came from the internet."
            case "not-executable":
                return "It isn't marked as runnable."
            case "script-interpreter":
                return "It's a script another program runs for you."
            case "scope-unknown":
                return "HTML Clay can't tell which folder this program may touch."
            case "main-folder":
                return "This is one of your main folders."
            default:
                return detail
            }
        }
    }
    public enum Sheets {
        public static let cancel = "Cancel"
        public static let choose = "Choose…"
        public static func taken(_ name: String) -> String { "“\(name)” is already taken. Try another name." }
    }
    public enum ServeSheet {
        public static let title = "Serve a folder"
        public static let leadStart = "It gets an address on this Mac, "
        public static let leadMono = ".html"
        public static let leadEnd = " files save too, and the dashboard can create, rename and move files. This also trusts the folder."
        public static let folder = "Folder"
        public static let name = "Name"
        public static let addressPrefix = "Address: "
        public static let serve = "Serve"
        public static let localStillServing = "Hyperclay Local is still serving this folder. Move from Hyperclay Local first."
        public static func insideServed(_ parent: String) -> String { "This is inside \(parent), which is already served." }
        public static func alreadyServed(_ name: String) -> String { "\(name) is already served." }
    }
    public enum TrustSheet {
        public static let htmlclay = ".htmlclay"
        public static func mainTitle(_ name: String) -> String { "Trust all of \(name)?" }
        public static func mainStart(_ name: String) -> String { "\(name) is one of your main personal folders. Every " }
        public static let mainEnd = " file in it could change every other one. Most people want a single project folder instead."
        public static let chooseAnother = "Choose another folder…"
        public static func title(_ name: String) -> String { "Trust \(name)?" }
        public static let every = "Every "
        public static let fileIn = " file in "
        public static let willSave = " will save as you edit, link to the others, and share one address. Documents in "
        public static let canChange = " can also change each other."
        public static func inside(_ name: String, count: Int) -> String {
            count == 1 ? "\(name) already has 1 trusted folder inside. It keeps its own address." : "\(name) already has \(count) trusted folders inside. They keep their own address."
        }
        public static func insideTrusted(_ name: String, around: String) -> String { "\(name) is inside \(around). Its documents get a new address, and nothing they stored in your browser moves with them." }
        public static let trustNewAddress = "Trust with new address"
    }
    public enum Levels {
        public static let trusted = "Trusted"
        public static let served = "Served"
        public static let synced = "Synced"
        public static let warning = "Warning"
        public static let error = "Error"
    }
    public enum Preview {
        public static let dashboardMissing = "Build 1: the dashboard page is missing from this build."
        public static let menuTitle = "Preview"
        public static let pretend = "Pretend I am"
        public static let wouldOpenFile = "Build 1: this would ask you to choose a .htmlclay file."
        public static let showRequest = "Show a request"
        public static func requestKind(_ kind: ProgramRequest.Kind) -> String {
            switch kind {
            case .found: return "A program was found"
            case .noFile: return "No file found"
            case .teamDocument: return "A team document"
            case .riskyFile: return "A risky file"
            case .folderRead: return "Reading outside the folder"
            case .folderReadAgain: return "A page asks to trust its own folder"
            }
        }
        public static func wouldOpen(_ name: String) -> String { "Build 1: this would open \(name) in your browser." }
        public static func wouldShowInFinder(_ name: String) -> String { "Build 1: this would show \(name) in Finder." }
        public static func wouldExport(_ name: String) -> String { "Build 1: this would export \(name) as a zip file." }
        public static func wouldRecover(_ name: String) -> String { "Build 1: this would ask where to save a recovered copy of \(name)." }
        public static let wouldOpenKeptCopies = "Build 1: this would open Kept Copies in your browser."
        public static func persona(_ persona: Persona) -> String {
            switch persona {
            case .newPerson: return "A new person"
            case .fiveDocuments: return "Five documents, one file gone"
            case .trustedFolder: return "A trusted folder and a waiting program"
            case .servedFolder: return "A served folder, not synced"
            case .syncedWithTeams: return "Synced, with teams and two conflicts"
            case .localUser: return "A Hyperclay Local user, first launch"
            }
        }
        public enum Gallery {
            public static let title = "Components"
            public static let path = "~/Clay/Example"
            public static let note = "Build 1: every Kiln control on one page."
            public static let cards = "Cards"
            public static let plainRowTitle = "A plain row"
            public static let plainRowSubtitle = "The subtitle sits under the title, and pills sit between them."
            public static let warnCardTitle = "This folder is not synced"
            public static let warnCardSubtitle = "Changes stay on this Mac until sync finishes."
            public static let errCardTitle = "Five documents are waiting"
            public static let errCardSubtitle = "Open the dashboard to see what needs you."
            public static let buttons = "Buttons"
            public static let plainButton = "Plain"
            public static let largeButton = "Large"
            public static let primaryButton = "Primary"
            public static let dangerButton = "Delete folder"
            public static let openDashboard = "Open dashboard"
            public static let withSymbol = "Add folder"
            public static let pills = "Pills"
            public static let squircles = "Squircles"
            public static let statusDots = "Status dots"
            public static let segmented = "Segmented"
            public static let segmentAll = "All"
            public static let segmentNeedsYou = "Needs you"
            public static let segmentSynced = "Synced"
            public static let input = "Search documents"
            public static let inputValue = "todo.htmlclay"
            public static let nesting = "Nested frames"
            public static let outerFrame = "Notes"
            public static let midFrame = "Journal"
            public static let innerFrame = "Today"
            public static let links = "Links"
            public static let link = "Open the dashboard"
            public static let dangerLink = "Remove this folder"
            public static let rowTitle = "Journal folder"
            public static let rowSubtitle = "Inside Notes"
            public static let switchTitle = "Serve this folder"
            public static let switchSubtitle = "Reachable at notes.htmlclay.localhost:1212"
            public static let badgeTitle = "Waiting requests"
            public static let back = "Back button"
        }
    }
    public enum Connect {
        public static let title = "Connect hyperclay.com"
        public static let username = "Username"
        public static let syncKey = "Sync key"
        public static let keyPlaceholder = "hcsk_…"
        public static let paste = "Paste"
        public static let refused = "That sync key wasn't accepted. Get a new one on hyperclay.com."
        public static let getKey = "Get a sync key ↗"
        public static let nothingSent = "Nothing is sent to hyperclay.com until you press Connect."
        public static let connect = "Connect"
        public static let wouldOpenKeys = "Build 1: this would open the sync key page on hyperclay.com."
        /// Where a person makes a sync key: the account menu of hyperclay.com's dashboard.
        public static let keyPageURL = "https://hyperclay.com/dashboard"
        // TODO-COPY: Connect sheet, while the key is being checked, max 20 chars
        public static let connecting = "Connecting…"
    }
    public enum AccountFolder {
        public static func title(_ username: String) -> String { "Where should @\(username) live on this Mac?" }
        public static let newFolder = "A new folder "
        public static let existing = "A folder I already have…"
        public static func willDownload(documents: Int, size: String) -> String { "\(documents) documents, \(size) will download." }
        public static let willStart = "It will be trusted, served at "
        public static let willEnd = ", and kept in step with hyperclay.com."
        public static let back = "Back"
        public static let setUpAndSync = "Set up and sync"
        public static let wouldChooseExisting = "Build 1: this would ask you to choose a folder you already have."
        // TODO-COPY: account set-up sheet, what a first sync into a folder already there will do, max 90 chars
        public static func willSync(_ counts: SyncCounts) -> String { "\(counts.toUpload) files will upload, \(counts.toDownload) will download, and \(counts.differ) differ. Nothing is deleted." }
        // TODO-COPY: account set-up sheet, the button after the counts show, max 20 chars
        public static let syncThese = "Sync these files"
        // TODO-COPY: set-up sheets, while the folder is being set up, max 20 chars
        public static let settingUp = "Setting up…"
    }
    public enum TeamSetup {
        public static func title(_ name: String) -> String { "Set up \(name)" }
        public static let opensAt = "Opens at "
        public static let trustCheck = "Trust the documents in this folder. They can change each other."
        public static func downloading(files: Int, total: Int, size: String, totalSize: String) -> String { "Downloading \(files) of \(total) files · \(size) of \(totalSize) MB" }
        public static func downloading(_ download: FirstDownload) -> String {
            downloading(files: download.done, total: download.total, size: megabytes(download.bytesDone), totalSize: megabytes(download.bytesTotal))
        }
        static func megabytes(_ bytes: Int64) -> String { String(format: "%.1f", Double(bytes) / 1_000_000) }
        public static let setUp = "Set up"
        public static let notEmpty = "This folder isn't empty. Choose an empty one."
        public static func inside(_ name: String) -> String { "It's inside \(name)." }
    }
    public enum AddProgram {
        public static let title = "Add a program"
        public static let lead = "No document may run it until you allow one. The first document that asks still gets a request."
        public static let file = "File"
        public static let nameHint = "The name documents ask for. Lowercase letters, numbers and hyphens."
        public static let add = "Add"
        public static let relinkTitle = "Find the program file"
        public static let relinkLead = "HTML Clay will run the file you choose instead. Nothing else about the program changes."
        public static let useFile = "Use this file"
        public static let beforeYouAdd = "Before you add it"
        public static let beforeYouUse = "Before you use it"
    }
    public enum Move {
        public static let title = "Move your folders from Hyperclay Local"
        public static func mapping(port: Int, address: String) -> String { "localhost:\(port) → \(address)" }
        public static let beforeYouMove = "Before you move"
        public static let localRunning = "Hyperclay Local is still running."
        public static let quitLocal = "Quit Hyperclay Local"
        public static func unsaved(_ document: String) -> String { "\(document) has unsaved changes in Hyperclay Local." }
        public static let showIt = "Show it"
        public static let comesAlong = "Comes along"
        public static func comesAlongText(conflicts: Int) -> String { "Versions, folder history, and \(conflicts) open conflicts, still waiting for you. Old bookmarks redirect. Anything a page kept only in browser storage stays at the old address." }
        public static let notNow = "Not now"
        public static let moveFolders = "Move folders"
        public static let wouldQuitLocal = "Build 1: this would quit Hyperclay Local after it saves."
        public static func wouldShowUnsaved(_ document: String) -> String { "Build 1: this would show \(document) in Hyperclay Local." }
        public static let movedTitle = "Your folders moved"
        public static let switchesTitle = "Serve and sync switches"
        public static let switchesWhere = "Settings › Folders"
        public static let noticesTitle = "Notices"
        public static let noticesWhere = "Needs you, in the dashboard"
        public static let activityTitle = "Activity"
        public static let activityWhere = "Each folder's history"
        public static let workspaceTitle = "The workspace window"
        public static let workspaceWhere = "Your browser"
        public static func conflictsWaiting(_ count: Int) -> String { count == 1 ? "1 conflict is waiting in Needs you." : "\(count) conflicts are waiting in Needs you." }
    }
    public enum Quit {
        public static let finishingTitle = "Finishing up…"
        public static func uploading(_ files: Int) -> String { "Uploading \(files) files to hyperclay.com. Your files are already saved on this Mac." }
        public static let quitNow = "Quit now"
        public static func cantSaveTitle(_ document: String) -> String { "\(document) can't save" }
        public static let cantSaveMessage = "The file is no longer writable. Quitting now loses the last change."
        public static let showDocument = "Show document"
        public static let retry = "Retry"
        public static let quitAnyway = "Quit anyway"
        public static let unsavedTitle = "Some edits aren't saved yet"
        public static func unsavedBody(_ lines: [String]) -> String {
            "HTML Clay couldn't save these documents:\n\n\(lines.joined(separator: "\n"))\n\nQuitting now loses those edits."
        }
        public static let cancel = "Cancel"
        public static let noAnswer = "Your open documents could not be checked."
        public static func portTitle(_ port: String, process: String) -> String { "Port \(port) is in use by “\(process)”" }
        public static func portTakenTitle(_ port: Int) -> String { "Port \(port) is in use" }
        public static let portTakenMessage = "Another app is using the port HTML Clay needs. Quit that app, then open HTML Clay again."
        public static let portMessage = "HTML Clay can use another port, but every page's saved browser data starts over at the new address."
        public static func quitProcess(_ process: String) -> String { "Quit \(process) and retry" }
        public static func usePort(_ port: Int) -> String { "Use port \(port)" }
        public static let wouldQuit = "Build 1: HTML Clay would quit now. The preview stays open."
        public static func wouldRetrySave(_ document: String) -> String { "Build 1: this would try saving \(document) again." }
        public static func wouldQuitProcess(_ process: String) -> String { "Build 1: this would quit \(process) and start HTML Clay on port 1212." }
        public static func wouldUsePort(_ port: Int) -> String { "Build 1: this would move every address to port \(port). Saved browser data starts over." }
    }
    public enum Open {
        public static func cantOpenTitle(_ name: String) -> String { "Can't open \"\(name)\"" }
        public static let cantOpenFallback = "Something went wrong. Try opening it again."
        public static let ok = "OK"
    }
    public enum Notify {
        public static func title(_ note: SampleNotification) -> String {
            switch note {
            case .conflict: return "Design Team: index.html changed in both places"
            case .keyStopped: return "Your sync key stopped working"
            case .addedToTeam: return "You were added to Marketing"
            case .wentBack: return "HTML Clay went back to 2.0"
            case .cantOpen: return "Can't open trip-plan.htmlclay"
            }
        }
        public static func body(_ note: SampleNotification) -> String {
            switch note {
            case .conflict: return "Choose which to keep. Its upload waits."
            case .keyStopped: return "Syncing stopped. Your files are safe."
            case .addedToTeam: return "Set it up on this Mac in Settings › Folders."
            case .wentBack: return "2.1 didn't start correctly."
            case .cantOpen: return "HTML Clay only opens files in your home folder."
            }
        }
        public static let fallbackNote = "Shown here because notifications are off for HTML Clay Preview."
        // TODO-COPY: live notifications, one title and body per kind; folder is the synced folder's name, max 70 chars
        public static func title(_ notice: HostNotice, folder: String?) -> String {
            switch notice.kind {
            case .conflict: return folder.map { "\($0): \(notice.name) changed in both places" } ?? "\(notice.name) changed in both places"
            case .keyStopped: return "Your sync key stopped working"
            case .addedToTeam: return "You were added to \(notice.name)"
            }
        }
        public static func body(_ notice: HostNotice) -> String {
            switch notice.kind {
            case .conflict: return "Choose which to keep. Its upload waits."
            case .keyStopped: return "Syncing stopped. Your files are safe."
            case .addedToTeam: return "Set it up on this Mac in Settings › Folders."
            }
        }
        // TODO-COPY: live notification card button, max 16 chars
        public static func open(_ notice: HostNotice) -> String {
            switch notice.kind {
            case .conflict: return "Review"
            case .keyStopped: return "Replace key"
            case .addedToTeam: return "Set up"
            }
        }
        // TODO-COPY: live notification card, why it shows in the corner instead of Notification Center, max 80 chars
        public static let fallbackNoteLive = "Shown here because notifications are off for HTML Clay."
    }
}

extension Strings.Preview {
    public static let quitWhileUploading = "Quit while uploading"
    public static let saveCantFinish = "Quit when a save can't finish"
    public static let portTaken = "Port 1212 taken at launch"
    public static let sendNotification = "Send a notification"
}

extension Strings.Panel {
    public static let keyRefused = "Your sync key no longer works. Syncing stopped."
    public static func offline(_ changes: Int) -> String { "Can't reach hyperclay.com. \(changes) changes will sync when you're back online." }
    public static func updateReady(_ version: String) -> String { "HTML Clay \(version) is ready" }
    public static let restart = "Restart"
    public static let finishing = "Finishing saves and sync…"
    public static let restartsSoon = "HTML Clay restarts in a moment"
    public static let hostStopped = "HTML Clay stopped working."
    public static let hostStoppedSub = "Your files are safe. Open documents can't save until it restarts."
    public static let notReady = "HTML Clay is still starting. Try again in a moment."
    public static let showLogs = "Show logs"
}
extension Strings.FolderPage {
    public static let pausedAll = "Syncing is paused for all folders"
    public static func offline(_ changes: Int) -> String { "Offline, \(changes) changes waiting" }
    public static func viewerSub(_ team: String) -> String { "You're now a viewer on \(team). Syncing stopped. Your files are still here." }
    public static let askOwner = "Ask a team owner to make you an editor."
    public static let open = "Open ↗"
    public static func missingTitle(_ path: String) -> String { "\(path) isn't there any more" }
    public static let missingSub = "Syncing stopped. Nothing was deleted on hyperclay.com."
    public static let setUpAgain = "Set up again…"
    public static func wasTeam(_ team: String) -> String { "Was \(team)" }
    public static let wasTeamSub = "Served on this Mac only. Programs still ask one document at a time, as for team folders."
    public static let reconnect = "Reconnect…"
    public static func wouldSetUpAgain(_ name: String) -> String { "Build 1: this would set up \(name) again in a new folder." }
}
extension Strings { public enum History {
    public static func title(_ folder: String) -> String { "Folder history · \(folder)" }
    public static let showSyncLog = "Show sync log"
    public static let today = "Today"
    // TODO-COPY: folder history rows, one per kind, the file's path after it, max 60 chars
    public static func entry(_ kind: HistoryEntry.Kind, _ path: String) -> String {
        switch kind {
        case .uploaded: return "Uploaded \(path)"
        case .downloaded: return "Downloaded \(path)"
        case .renamed: return "Renamed \(path)"
        case .removed: return "Removed \(path)"
        case .conflict: return "\(path) changed in both places"
        case .settled: return "Settled a conflict in \(path)"
        }
    }
    // TODO-COPY: folder history, when sync has done nothing in this folder yet, max 60 chars
    public static let empty = "Nothing has synced in this folder yet."
} }
extension Strings.Preview {
    public static let alsoPretend = "Also pretend"
    public static func overlay(_ overlay: PreviewOverlay) -> String {
        switch overlay {
        case .keyRefused: return "Sync key stopped working"
        case .offline: return "Offline"
        case .updateReady: return "Update ready"
        case .hostStopped: return "The host stopped"
        case .teamViewer: return "Design Team: you became a viewer"
        case .teamFolderMissing: return "Design Team: folder missing on disk"
        }
    }
}
