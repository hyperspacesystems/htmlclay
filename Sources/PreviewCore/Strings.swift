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
    public enum Menu {
        public static let settings = "Settings…"
        public static let checkForUpdates = "Check for updates…"
        public static let quit = "Quit HTML Clay"
        public static let rowOpen = "Open"
        public static let versionHistory = "Version history…"
        public static let showInFinder = "Show in Finder"
        public static let exportZip = "Export as zip…"
        public static let removeFromRecent = "Remove from Recent"
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
        public static let trustNote = "Build 1: this would ask you to choose a folder to trust."
        public static let serveNote = "Build 1: this would ask you to choose a folder to serve as a local site."
        public static let moveNote = "Build 1: this would bring your Hyperclay Local folders into HTML Clay."
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
        public static func accountGroup(username: String) -> String { "hyperclay.com · @\(username)" }
        public static func inside(parent: String, path: String) -> String { "Inside \(parent) · \(path)" }
        public static func viewOnlyTeams(_ count: Int) -> String {
            count == 1 ? "1 team you can only view" : "\(count) teams you can only view"
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
        public static let dashboardAddress = "dashboard.localhost:1212"
        public static let open = "Open"
        public static let welcome = "Welcome window"
        public static let showAgain = "Show again"
        public static let documentsLine = "Documents open in your default browser."
    }
    public enum Request {
        public static func title(_ document: String) -> String { "Let \(document) run a program?" }
        public static let dontAllow = "Don't allow"
        public static let allow = "Allow"
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
        public static let notYet = "Build 1: this pane comes in a later step tonight."
        public static func wouldOpen(_ name: String) -> String { "Build 1: this would open \(name) in your browser." }
        public static func wouldShowInFinder(_ name: String) -> String { "Build 1: this would show \(name) in Finder." }
        public static func wouldExport(_ name: String) -> String { "Build 1: this would export \(name) as a zip file." }
        public static func wouldRemove(_ name: String) -> String { "Build 1: this would remove \(name) from Recent." }
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
            public static let switchSubtitle = "Reachable at notes.hyperclay.localhost:1212"
            public static let badgeTitle = "Waiting requests"
            public static let back = "Back button"
        }
    }
}
