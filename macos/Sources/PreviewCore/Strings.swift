import Foundation

public enum Strings {
    public enum App {
        public static let name = "HTML Clay Preview"
    }
    public enum Panel {
        public static let openDashboard = "Open dashboard"
        public static let recentDocuments = "Recent HTML files"
        public static let noDocumentsTitle = "No HTML files yet"
        public static let noDocumentsSub = "Double-click any .htmlclay file in Finder."
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
            count == 1 ? "\(folder): 1 HTML file changed in both places" : "\(folder): \(count) HTML files changed in both places"
        }
        public static func wantsProgram(document: String) -> String { "\(document) wants to run a program" }
        public static func fileMissing(versions: Int) -> String { "File moved or deleted · \(versions) versions kept" }
        public static func documentSubtitle(place: String, opened: String) -> String { "\(droppingHome(place)) · \(opened)" }
        public static func syncing(_ count: Int, username: String) -> String { "Syncing \(count) \(count == 1 ? "file" : "files")… · @\(username)" }
        public static func upToDate(_ username: String) -> String { "Up to date · @\(username)" }
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
        public static let recoveredChanges = "Recovered changes…"
        public static let noRecent = "No recent HTML files"
        public static let gear = "HTML Clay menu"
        public static func needsYou(_ count: Int) -> String { "Needs you (\(count))" }
        public static func version(_ version: String) -> String { "HTML Clay \(version)" }
        public static func restartToUpdate(_ version: String) -> String { "Restart to update to \(version)" }
    }
    public enum Welcome {
        public static let appName = "HTML Clay"
        public static let title = "Build software in a single HTML file"
        public static let lead = "Open or create any HTML file with whatever interface you want. It saves changes straight back into itself, talks to local tools on your Mac, and syncs live with Hyperclay."
        public static let filesTitle = "Any interface you can imagine"
        public static let filesSub = "Edit in the browser. The file on your Mac updates as you go."
        public static let versionsTitle = "Single portable HTML file"
        public static let versionsSub = "HTML Clay keeps earlier versions of each HTML file."
        public static let localTitle = "Reshape files with your local AI agent"
        public static let localSub = "No account needed. It works offline."
        public static let moreTitle = "Runs local programs with permission"
        public static let moreSub = "Serve a folder as a local site, or sync with Hyperclay. Both are in Settings."
        public static let openDashboard = "Open Dashboard"
        public static let allSet = "I'm all set"
        public static let openSettings = "Open Settings"
        public static let menuBarHint = "HTML Clay stays in your menu bar:"
    }
    public enum Settings {
        public static func title(_ pane: SettingsPane) -> String {
            switch pane {
            case .general: return "General"
            case .aiEditing: return "AI Editing"
            case .folders: return "Folders"
            case .programs: return "Programs"
            case .sync: return "Sync"
            case .updates: return "Updates"
            case .advanced: return "Advanced"
            }
        }
    }
    public enum Folders {
        public static let title = "Folders"
        public static let intro = "Choose folders on this Mac to run as local sites or trust for standalone files."
        public static let serveCardTitle = "Run a project folder as a local site"
        public static let serveCardSub = "Gives a folder its own localhost address so HTML files can save and share assets."
        public static let trustCardTitle = "Let standalone .htmlclay files save and run programs"
        public static let trustCardSub = "For a folder of loose .htmlclay tools or notes on this Mac, so files save changes to disk and can run allowed programs."
        public static let addFolder = "Add Folder…"
        public static let open = "Open"
        public static let openInDashboard = "Open in Dashboard"
        public static let openInFinder = "Open in Finder"
        public static let finder = "Finder"
        public static let more = "More"
        public static let less = "Less"
        public static let openLink = "Open Link"
        public static let copyAddress = "Copy Address"
        public static let httpPrefix = "http://"
        public static func addressURL(url: String, address: String) -> URL? {
            if !url.isEmpty, let parsed = URL(string: url) { return parsed }
            if !address.isEmpty { return URL(string: httpPrefix + address) }
            return nil
        }
        public static func copyPathLabel(_ name: String) -> String { "Copy path for \(name)" }
        public static func copyPathHelp(_ path: String) -> String { "Copy full path (\(path))" }
        public static func copiedPath(_ path: String) -> String { "Copied \(path) to the clipboard." }
        public static func insideParent(_ parent: String) -> String { "Inside \(parent)" }
        public static let advanced = "Advanced"
        public static let serveLocally = "Serve on this Mac"
        public static let copy = "Copy"
        public static let stopSyncing = "Stop Syncing"
        public static let syncNotConnected = "Not connected"
        public static let connectHyperclay = "Connect Hyperclay…"
        public static let discardAndStopTrusting = "Discard & Stop Trusting"
        public static let framingOnPill = "Embedding on"
        public static let elbow = "└"
        public static let trustMenu = "Trust a folder…"
        public static let serveMenu = "Serve a folder…"
        public static let connectMenu = "Connect Hyperclay…"
        public static let openHyperclayNote = "Build 1: this would open your folders on Hyperclay."
        public static let emptyTitle = "Add a folder to open and save HTML files"
        public static let emptySub = "HTML files in an added folder save changes to disk and can link to each other."
        public static let serveLink = "Building a site or an app? Serve a folder…"
        public static let readAccessTitle = "Read access"
        public static let readAccessHelp = "Folders you allowed a page to read. Removing one makes the page ask again."
        public static let readAccessRemove = "Remove access"
        public static func readAccessRemoveLabel(_ folder: String) -> String { "Remove access to \(folder)" }
        public static func readAccessPages(_ pages: [String]) -> String {
            guard let first = pages.first.map(ProgramControls.documentLabel) else { return "No HTML files open right now" }
            return pages.count == 1 ? "Readable by \(first)" : "Readable by \(first) and \(pages.count - 1) more"
        }
        public static let moveTitle = "Move from Hyperclay Local"
        public static let moveSub = "Your Hyperclay Local folders were found. Bring them into HTML Clay."
        public static let move = "Move…"
        public static let onThisMac = "On this Mac"
        public static let onlyOnThisMac = "Only on this Mac"
        public static let setUp = "Set up…"
        public static let openHyperclay = "Open Hyperclay ↗"
        public static func rootRoute(_ id: String) -> String { "/roots/\(id)/files" }
        public static func trustedRoute(_ id: String) -> String { "/trusted/\(id)/files" }
        public static func pathAndAddress(path: String, address: String) -> String { "\(path) · \(address)" }
        public static func copied(_ address: String) -> String { "Copied \(address) to the clipboard." }
        public static func accountGroup(username: String) -> String { "Hyperclay · @\(username)" }
        public static func inside(parent: String, path: String) -> String { "Inside \(parent) · \(path)" }
        public static func viewOnlyTeams(_ count: Int) -> String {
            count == 1 ? "1 team you can only view" : "\(count) teams you can only view"
        }
    }
    public enum Sync {
        public static let title = "Sync with Hyperclay"
        // copy: the sync pane before connecting, beside Connect: answers "does connecting sync at once" (Carbon, picked by David 2026-11-25)
        public static let inviteLead = "Connecting syncs nothing."
        public static let invite = "You need to pick a folder before syncing starts."
        public static let connect = "Connect Hyperclay…"
        public static let benefit1Title = "Cloud backup & sync"
        public static let benefit1Sub = "Keep synced folders backed up online so you can edit anywhere."
        public static let benefit2Title = "Work with your team"
        public static let benefit2Sub = "Edit the same HTML file together live and share links anytime."
        public static let benefit3Title = "Publish to the web"
        public static let benefit3Sub = "Give any HTML file or folder its own web address to share."
        // copy: the sync pane's three steps before connecting, matching ConnectSheet, AccountFolderSheet and TeamSetupSheet
        public static let stepsTitle = "What happens when you connect"
        public static let step1Title = "Enter your sync key"
        public static let step1Body = "Paste your username and hcsk_ key from your dashboard."
        public static let step2Title = "Choose one Mac folder"
        public static let step2Body = "Pick a new or existing folder for your whole account."
        public static let step3Title = "Confirm to start sync"
        public static let step3Body = "Click Set up and sync to start. Set up teams later in empty folders."
        public static let replaceKey = "Replace key…"
        public static let disconnect = "Disconnect…"
        public static let viewOnly = "Can't sync here"
        public static let disconnectTitle = "Disconnect from Hyperclay?"
        public static let disconnectMessage = "Syncing stops and HTML Clay removes your sync key from this Mac. Your files stay here. Connect the same account again to sync the same folders."
        public static let disconnectConfirm = "Disconnect"
        public static let syncing = "Syncing to Hyperclay"
        public static let folders = "Folders"
        public static let syncingWord = "syncing"
        public static let setUp = "Set up…"
        public static let keyRefusedTitle = "Your sync key no longer works"
        public static let keyRefusedSub = "All folders stopped syncing. Your files are safe on this Mac."
        public static let reconnect = "Reconnect…"
        public static let wouldReplaceKey = "Build 1: this would ask for a new sync key."
        public static let keyInFile = "Your sync key stays in a file only you can read because this computer has no keychain."
        public static let wouldOpenSite = "Build 1: this would open Hyperclay in your browser."
        public static func account(_ username: String) -> String { "@\(username)" }
        public static func connected(keySuffix: String) -> String { "Connected · sync key ending \(keySuffix)" }
        public static func syncingFiles(_ n: Int, lastFinished: String) -> String {
            let base = "Syncing \(n) \(n == 1 ? "file" : "files")…"
            return lastFinished.isEmpty ? base : base + " · last finished \(lastFinished)"
        }
        public static func upToDate(lastFinished: String) -> String {
            lastFinished.isEmpty ? "Up to date" : "Up to date · last finished \(lastFinished)"
        }
        public static func folderLine(path: String, state: String) -> String { "\(path) · \(state)" }
        public static let stateUpToDate = "up to date"
        public static let statePaused = "paused"
        public static let stateOff = "not syncing"
        public static let stateConflicts = "needs you"
        public static func downloading(done: Int, total: Int) -> String { "downloading \(done) of \(total)" }
        public static func review(_ n: Int) -> String { "Review \(n) in dashboard" }
    }
    public enum FolderPage {
        public static let trusted = "Trusted"
        public static let trustedSub = "Lets HTML files in this folder save changes to disk and link to each other. Turning this off also stops serving and syncing."
        public static let trustedSubNotServed = trustedSub
        public static let trustedSubServed = trustedSub
        public static let serve = "Serve on this Mac"
        public static let serveSubOff = "Gives a web address on this Mac, and lets .html files save too"
        public static let serveSubOn = "Local web address and .html saving enabled."
        public static let serveSubOnlyOnThisMac = " · Only on this Mac"
        public static let address = "Address"
        public static let framing = "Allow other sites to embed pages"
        public static let framingSub = "Other websites can show these pages inside their own pages."
        public static let framingNote = "Turn this on only for pages you mean to share with another site. You can turn it back off only in the Mac app."
        public static let copy = "Copy address"
        public static let versions = "Versions and uploads"
        public static let show = "Show"
        public static let sync = "Sync with Hyperclay"
        public static let syncSub = "Turning off disconnects sync while keeping files served on this Mac."
        public static let onlyOnThisMac = "Only on this Mac"
        public static let onlyOnThisMacSub = "Connect your Hyperclay account to sync folders."
        public static let connect = "Connect…"
        public static let folderHistory = "Folder history"
        public static let cancel = "Cancel"
        public static let stopTrusting = "Stop Trusting"
        public static let stopServing = "Stop serving"
        public static let disconnect = "Disconnect"
        public static func pathLine(path: String, documents: Int) -> String { "\(path) · \(documents) HTML files" }
        public static func childrenTitle(count: Int, parent: String) -> String { "\(count) trusted folders inside \(parent) keep their own address" }
        public static func childrenSub(parent: String) -> String { "HTML files in them stay separate from \(parent) and keep their own browser data." }
        public static func mergeButton(parent: String) -> String { "Merge into \(parent)…" }
        public static func versionsSub(size: String) -> String { "Kept in this folder · \(size)" }
        public static func versionsNote(folder: String) -> String { "Build 1: this would list the versions and uploads kept in \(folder)." }
        public static func accountTitle(username: String) -> String { "@\(username)" }
        public static func stopTrustingTitle(_ name: String) -> String { "Stop trusting \(name)?" }
        public static func stopTrustingMessage(_ name: String) -> String { "HTML files open from \(name) stop saving now, and \(name) leaves your Folders list. Files there save again only when you open them yourself." }
        public static let stopTrustingSyncMessage = "Syncing with Hyperclay and local serving will stop. Your HTML files stay safely on this Mac."
        public static let unsavedStopTrustingTitle = "Unsaved edits in open files"
        public static func unsavedStopTrustingMessage(_ documents: [String]) -> String {
            let names = documents.isEmpty ? "An open file" : documents.joined(separator: ", ")
            return "\(names) has unsaved edits in your browser. Stop trusting anyway and discard unsaved changes, or Cancel to save first."
        }
        public static func stopServingTitle(_ name: String) -> String { "Stop serving \(name)?" }
        public static func stopServingMessage(_ name: String) -> String { "Its address stops working and .html files there stop saving. The folder stays trusted and keeps its name for later." }
        public static func disconnectTitle(_ name: String) -> String { "Disconnect \(name)?" }
        public static func disconnectMessage(_ name: String, team: Bool) -> String {
            let base = "Your files stay on this Mac and keep being served. Changes no longer sync."
            return team ? base + " Programs keep treating it as a team folder." : base
        }
        public static func mergeTitle(child: String, parent: String) -> String { "Merge \(child) into \(parent)?" }
        public static func mergeMessage(child: String, parent: String, documents: Int) -> String {
            "\(child)'s \(documents) HTML files will move to \(parent)'s address, where HTML files in \(parent) can link to and change each other. Anything those HTML files saved in the browser stays behind at \(child)'s old address. Your files on disk stay right where they are."
        }
        public static func mergeAction(parent: String) -> String { "Merge into \(parent)" }
        public static func relinkTitle(_ name: String) -> String { "Find \(name)?" }
        public static let relinkLead = "Choose where this folder moved. It keeps its id, name, and address."
    }
    public enum Programs {
        public static let addProgram = "Add program…"
        public static let intro = "Allow trusted HTML files to run programs and scripts on this Mac."
        public static let waitingForYou = "Waiting for you"
        public static let builtin = "Built-in programs"
        public static let programs = "Your programs"
        public static let openExample = "Open example"
        public static let noPermissionYet = "No HTML file has permission yet. Open the example above to try it."
        public static func enableBuiltin(_ title: String) -> String { "Enable \(title)" }
        public static let fileSearch = "File search"
        public static let fileConverter = "Image converter"
        public static let folderWatcher = "Folder watcher"
        public static let builtinDot = " · "
        public static let noneYetOn = "No HTML file may run it yet"
        public static let searchDoes = "Searches file names and text inside files"
        public static let searchOffSub = "Searches file names and text inside files in a folder you allow."
        public static let searchEnableSub = "Lets allowed HTML files search file names and text inside files in a folder you choose. HTML files ask for it as “file-search”."
        public static let searchExample = "filesearch.htmlclay"
        public static let searchExampleSub = "Search a folder by word or file type and reveal matches in Finder."
        public static let convertDoes = "Converts and resizes images"
        public static let convertOffSub = "Converts and resizes PNG, JPEG, GIF, BMP, TIFF and WebP images, and converts HEIC photos to those formats."
        public static let convertEnableSub = "Lets allowed HTML files convert and resize PNG, JPEG, GIF, BMP, TIFF and WebP images, and save HEIC photos in those formats. HTML files ask for it as “file-convert”."
        public static let convertExample = "imageconverter.htmlclay"
        public static let convertExampleSub = "Convert images between formats and sizes right inside a page."
        public static let watchDoes = "Tells an HTML file when files in a folder change"
        public static let watchOffSub = "Tells an HTML file when files in an allowed folder are added, changed or removed."
        public static let watchEnableSub = "Lets allowed HTML files watch an allowed folder and hear when files are added, changed or removed. HTML files ask for it as “folder-watch”."
        public static let watchExample = "folderactivity.htmlclay"
        public static let watchExampleSub = "Watch a folder live as files arrive, change or get deleted."
        public static func example(_ name: String) -> String { "Example: \(name)" }
        public static let answer = "Answer…"
        public static let findIt = "Find it…"
        public static let allTrusted = "All HTML files in trusted folders"
        public static let blockedDocuments = "Blocked HTML files"
        public static let askAgain = "Ask again"
        public static let noneYet = "No HTML file may run it until you allow one."
        public static func wants(document: String, program: String) -> String { "\(document) wants “\(program)”" }
        public static func mayRun(_ document: String) -> String { "\(document) may run it" }
        public static func mayRunCount(_ count: Int) -> String { "\(count) HTML files may run it" }
        public static func fileMissing(_ count: Int) -> String {
            count == 1 ? "file missing · 1 HTML file uses it" : "file missing · \(count) HTML files use it"
        }
        public static func wouldFind(_ name: String) -> String { "Build 1: this would ask you where \(name) went." }
        public static func aiEditAllowedFor(_ document: String) -> String { "Allowed for \(document)" }
        public static func aiEditAllowedCount(_ count: Int) -> String { "Allowed for \(count) HTML files" }
        public static func aiEditWaiting(document: String) -> String { "Allow AI editing for \(document)?" }
        public static let aiAgent = "AI agent"
    }
    public enum ProgramPage {
        public static let runsAsYou = "This program runs as you. It can read and change any file you can."
        public static let whoMayRun = "Who may run it"
        public static let aiEditNote = "An AI tool on this Mac rewrites selected text when you press ⌘J. Turn AI editing on or off in Settings › AI Editing."
        public static let allTrustedTitle = "All HTML files in my trusted folders"
        public static let allTrustedSub = "HTML files in your own trusted folders run it without asking, after the one warning for an HTML file nobody vouches for. Team folders still ask one HTML file at a time."
        public static let removeAccess = "Remove access"
        public static let accessRemoved = "Access removed"
        public static let allowAgain = "Allow again…"
        public static let chooseAnotherFile = "Choose another file…"
        public static let removeAllAccess = "Remove all access"
        public static func removeAllTitle(_ title: String) -> String { "Remove all access to \(title)?" }
        public static let removeAllBody = "No HTML file can use it until you allow it again."
        public static let cancel = "Cancel"
        public static let showInFinder = Menu.showInFinder
        public static func pathLine(path: String, ready: Bool, name: String) -> String {
            "\(path) · \(ready ? "ready" : "file missing") · HTML files ask for it as “\(name)”"
        }
    }
    public enum General {
        public static let title = "General"
        public static let openAtLogin = "Open HTML Clay at login"
        public static let aiTitle = "AI editing"
        public static let aiEnableTitle = "Enable AI editing"
        public static let aiSub = "Press ⌘J in an HTML file to rewrite selected elements using an AI CLI on this Mac. Only the selected HTML and your prompt are sent."
        public static let aiColTool = "TOOL"
        public static let aiColModel = "MODEL"
        public static let aiColEffort = "EFFORT"
        public static let aiFoundHere = "Found on this Mac"
        public static let aiNotFound = "Not found on PATH"
        public static let aiAgyFound = "Antigravity CLI (agy)"
        public static let aiAgyNotFound = "Install Antigravity CLI (agy)"
        public static let aiModel = "Model"
        public static func aiModelLabel(_ tool: String) -> String { "\(tool) model" }
        public static let aiToolClaude = "Claude"
        public static let aiToolCodex = "Codex"
        public static let aiToolGemini = "Gemini"
        public static let aiCodexDefault = "Codex default"
        public static let aiEffort = "Effort"
        public static func aiModelDisplayName(_ id: String) -> String {
            switch id {
            case "claude-opus-5-5": return "Claude Opus 5.5"
            case "claude-sonnet-5-5": return "Claude Sonnet 5.5"
            case "claude-fable-5-2": return "Claude Fable 5.2"
            case "gpt-5.6-sol": return "GPT-5.6 Sol"
            case "gpt-6-astra": return "GPT-6 Astra"
            case "gemini-4-carbon": return "Gemini 4 Carbon"
            case "gemini-4-neon": return "Gemini 4 Neon"
            case "gemini-4-argon": return "Gemini 4 Argon"
            case "gemini-3.8-flash": return "Gemini 3.8 Flash"
            default: return id
            }
        }
        public static func aiEffortName(_ effort: String) -> String {
            switch effort {
            case "low": return "Low"
            case "medium": return "Medium"
            case "high": return "High"
            case "xhigh": return "Extra high"
            case "max": return "Max"
            default: return effort
            }
        }
        public static let aiNoTools = "Claude, Codex, and Gemini (agy) were not found on this Mac. Install any of them, or set up a custom command below."
        public static let aiCustomTitle = "Custom command"
        public static let aiCustomSub = "Use any other command line tool on this Mac"
        public static let aiCustomButton = "Configure…"
        public static let aiCustomSheetTitle = "Custom AI command"
        public static let aiCustomLead = "Enter the command HTML Clay should run when you rewrite selected text. HTML Clay sends your selection and request to this command and puts its reply into the page."
        public static let aiCommand = "Command"
        public static let aiCommandPlaceholder = "ollama run qwen2.5-coder:14b"
        public static let aiCommandNote = "Write {prompt} where the request text belongs. If you leave {prompt} out, HTML Clay sends the request straight into the command."
        public static let aiSaveCommand = "Save command"
        public static let aiSaveFailed = "That command could not be saved."
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
        public static let documentsLine = ".htmlclay files open in your default browser."
    }
    public enum YourName {
        public static let heading = "Your name"
        public static let useName = "Use my name in HTML files"
        public static let offSub = "Shown in HTML files that support showing your name beside your edits and comments."
        public static let name = "Name"
        public static let placeholder = "Your name"
        public static let accountSub = "Your Hyperclay name"
        public static let change = "Change on Hyperclay ↗"
        public static let note = "Saved in supported HTML files beside your edits and comments, and visible when shared."
        public static let wouldChange = "Build 1: this would open your Hyperclay profile."
    }
    public enum Files {
        public static let title = ".htmlclay files"
        public static let switchTitle = "Open .htmlclay files in HTML Clay"
        public static let headline = "Single files that save their own changes"
        public static let lead = "Double-click any .htmlclay file in Finder. It opens in your default browser, and every edit saves right back into that same file on your Mac."
        public static let opensTitle = "HTML Clay opens .htmlclay files"
        public static let opensSub = "Double-click any .htmlclay file to open it in your browser and save changes back to disk."
        public static let defaultPill = "Default"
        public static let otherTitle = "Another app opens .htmlclay files"
        public static let otherUnknownSub = "HTML Clay is not currently the default app for .htmlclay files."
        public static let makeDefault = "Make HTML Clay the default"
        public static let openWithTitle = "Show HTML Clay in Open With for .html files"
        public static let openWithSub = "Right-click any regular .html file in Finder and choose Open With to edit and save it in HTML Clay. Your default app for .html files stays the same."
        public static let openWithMac = "macOS always includes HTML Clay in Open With for .html files while HTML Clay is installed on your Mac."
        public static let openWithTip = "Tip: Right-click any regular .html file in Finder and choose Open With › HTML Clay."
        public static let stopTitle = "Stop opening .htmlclay files"
        public static let stopSubOn = "Turns off double-click opening in Finder so macOS hands .htmlclay files to another app."
        public static let stopSubOff = "HTML Clay is not currently set to open .htmlclay files."
        public static let turnOff = "Turn off…"
        public static let stopConfirmTitle = "Stop opening .htmlclay files with HTML Clay?"
        public static let stopConfirmBody = "Double-clicking a .htmlclay file in Finder will no longer open it in HTML Clay. Your files do not change, and you can make HTML Clay the default again here anytime."
        public static let stopConfirm = "Stop opening"
        public static let cancel = "Cancel"
        public static func otherSub(_ app: String) -> String {
            "Currently opens in \(app). Turn on to make HTML Clay the default."
        }
    }
    public enum Updates {
        public static let youAreOnLatest = "You're on the latest version"
        public static let unavailableTitle = "Automatic updates are off for this build"
        public static func unavailableBuildSub(_ version: String) -> String {
            "HTML Clay \(version) · Test and development builds don't auto-update"
        }
        public static func unavailableMoveSub(_ version: String) -> String {
            "HTML Clay \(version) · Move HTML Clay to Applications to enable updates"
        }
        public static func latest(_ version: String) -> String { "HTML Clay \(version)" }
        public static func latest(_ version: String, checked: String) -> String { "HTML Clay \(version) · Checked \(checked)" }
        public static let justNow = "just now"
        public static let checkFailed = "Couldn't check for updates"
        public static func checkFailedSub(_ version: String, tried: String) -> String { "HTML Clay \(version) · Tried \(tried)" }
        public static let checking = "Checking for updates…"
        public static func checkingYouHave(_ version: String) -> String { "You have HTML Clay \(version)" }
        public static func readySub(_ current: String) -> String { "You have \(current) · Downloaded and ready to install" }
        public static func ready(_ version: String) -> String { "HTML Clay \(version) is ready" }
        public static func youHave(_ version: String, checked: String) -> String { "You have \(version) · checked \(checked)" }
        public static let restart = "Restart to update"
        public static func current(_ version: String) -> String { "HTML Clay \(version)" }
        public static func upToDate(checked: String) -> String { "Up to date · checked \(checked)" }
        public static let checkNow = "Check now"
        public static let automatic = "Update automatically"
        public static let restartNote = "HTML Clay finishes saves and sync before it restarts."
        public static let whatsNew = "What's new"
        public static let changelogRowTitle = "See our latest updates"
        public static let viewChangelog = "View changelog"
        public static let versionDot = "."
        public static func wouldRestart(_ version: String) -> String { "Build 1: this would restart HTML Clay into \(version)." }
        public static let wouldCheck = "Build 1: this would check for a new version."
        public static let whatsNewURL = "https://htmlclay.com/changelog"
        /// What's new opens the changelog page at the running version's entry.
        public static func whatsNewLink(_ version: String) -> URL? {
            let bare = version.hasPrefix("v") ? String(version.dropFirst()) : version
            return URL(string: whatsNewURL + "#v" + bare)
        }
        // copy: dialog title when an AI edit or a document's program is still running at Restart to update, max 50 chars
        public static let runningWorkTitle = "Something is still running"
        // copy: dialog body for the same, says restarting stops it, max 140 chars
        public static let runningWorkBody = "An AI edit or a program started by an HTML file is still running. Restarting now stops it."
        // copy: the destructive button in that dialog, max 20 chars
        public static let restartAnyway = "Restart anyway"
        // copy: notice title when Restart to update was chosen but HTML Clay has not restarted after 40 seconds, max 50 chars
        public static let notStartedTitle = "The update hasn't started yet"
        // copy: notice body for the same, what the person can do, max 140 chars
        public static let notStartedBody = "Quit and reopen HTML Clay to finish the update, or keep working and it will install the next time you quit."
        // copy: notice title when HTML Clay could not get ready to restart for an update, max 50 chars
        public static let restartFailedTitle = "HTML Clay couldn't restart to update"
        // copy: notice body for the same, says nothing was lost and it keeps running, max 140 chars
        public static let restartFailedBody = "Nothing was lost and HTML Clay is still running. Try Restart to update again in a moment."
        // copy: Updates pane status row, the status menu row and the panel while Sparkle downloads a found update, %@ is the version, max 40 chars
        public static func downloading(_ version: String) -> String { "Downloading HTML Clay \(version)…" }
        // copy: alert title after a Check for updates the person asked for found nothing newer, max 50 chars
        public static let latestAlertTitle = "HTML Clay is up to date"
        // copy: alert body for the same, %@ is this version, max 140 chars
        public static func latestAlertBody(_ version: String) -> String { "HTML Clay \(version) is the newest version." }
        // copy: alert title when a Check for updates the person asked for could not check, max 50 chars
        public static let failedAlertTitle = "HTML Clay couldn't check for updates"
        // copy: alert body for the same, what to do, max 140 chars
        public static let failedAlertBody = "The update server could not be reached, or the download stopped. Check your connection and try again in a moment."
    }
    public enum Advanced {
        public static let port = "Port"
        public static let portSub = "The number at the end of every local web address on this Mac, such as htmlclay.localhost:1212."
        public static let portNumber = "1212"
        public static let changePort = "Change port…"
        public static let changePortTitle = "Change HTML Clay's port?"
        public static let changePortField = "New port"
        public static let changePortAction = "Change port"
        public static let portRange = "Enter a port number between 1024 and 65535."
        public static let appLog = "App log"
        public static let appLogSub = "Errors from trusting folders, running programs, and opening or saving files."
        public static let syncLog = "Sync log"
        public static let syncLogSub = "Uploads, downloads, and connection details for Hyperclay."
        public static func portCurrent(_ port: Int) -> String { "Currently \(port)" }
        public static func portMoveBody(_ port: Int) -> String {
            "Every HTML Clay address on this Mac will use the new port, so you will need to update your bookmarks. Anything a page saved in your browser stays with the old address and starts fresh at the new one. Visits to port \(port) will keep forwarding to the new port."
        }
        public static func openDocuments(_ documents: [String]) -> String { documents.joined(separator: ", ") }
        public static let liveService = "Serving folders"
        public static let liveServiceSub = "Runs served and synced folders"
        public static let running = "Running"
        public static let starting = "Starting"
        public static let stopped = "Stopped"
        public static let off = "Not running"
        public static let storage = "Storage"
        public static let versions = "Versions of HTML files outside served folders"
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
        public static func callsIt(_ program: String) -> String { "The HTML file calls it “\(program)”." }
        public static let foundHere = "Found in a folder where you keep programs."
        public static let runsAsYou = "A program runs as you. It can read, change or delete any file you can, and use the internet. Allow only a program you trust."
        public static let moreOptions = "More options"
        public static func only(_ document: String) -> String { "Only \(document)" }
        public static func allowFile(_ name: String) -> String { "Allow \(name)" }
        public static func runNamed(document: String, program: String) -> String { "Let \(document) run “\(program)”?" }
        public static func useAIEditing(document: String) -> String { "Let \(document) use AI editing?" }
        public static func aiEditTeamLead(_ folder: String) -> String { "This HTML file is in \(folder), a team folder where others can edit HTML files later." }
        public static let noFileLead = "HTML Clay didn't find this program on your Mac. The HTML file should come with instructions. If you're not sure, choose Don't allow: the HTML file still opens and saves."
        public static let chooseAFile = "Choose a file…"
        public static func fromTeam(_ folder: String) -> String { "From \(folder). Its editors can change this HTML file later." }
        public static let teamAsksEachTime = "Team HTML files always ask one at a time."
        public static let teamCodeChanged = "This page's code changed since you last allowed it."
        public static let allowForThisDocument = "Allow for this HTML file"
        public static func builtinTitle(document: String, program: String) -> String {
            "Let \(document) use \(program)?"
        }
        public static func builtinLead(program: String, folder: String) -> String {
            "\(program) will work only for this HTML file and only inside \(folder). After allowing, go back to the page and try again."
        }
        public static func aiToolsTitle(_ document: String, level: String = "") -> String {
            level == "change"
                ? "Which folder may your AI agent read and change for \(document)?"
                : "Which folder may your AI agent read for \(document)?"
        }
        public static let aiToolsLead = "Your AI agent reads the folder you choose, and nothing outside it, to answer this page. It cannot change files or run commands. The page sees its answer."
        /// The folder question's lead for the level the page asked: "change" lets the agent edit files.
        public static func aiToolsLead(level: String) -> String { level == "change" ? aiToolsChangeLead : aiToolsLead }
        public static let aiToolsChangeLead = "Your AI agent can read, edit and create files in the folder you choose, and nothing outside it, to do what this page asks. It cannot run commands. Choose a folder whose changes you can undo, such as one kept in git."
        public static func programFolderTitle(program: String, document: String, level: String = "") -> String {
            level == "change"
                ? "Which folder may the program \(program) in \(document) read and change?"
                : "Which folder may the program \(program) in \(document) read?"
        }
        public static let programFolderLead = "The program can read files and git history in the folder you choose, and nothing outside it. It cannot reach the internet or start other programs. The page sees what it returns."
        public static let programFolderChangeLead = "The program can read, edit and create files only in the folder you choose. It cannot reach the internet or start other programs. Choose a folder whose changes you can undo, such as one kept in git."
        /// The folder question's lead for a page's own program at the level it was given: "change" lets it edit files.
        public static func programFolderLead(level: String) -> String { level == "change" ? programFolderChangeLead : programFolderLead }
        public static let chooseAFolder = "Choose a folder…"
        public static func unknownTitle(_ document: String) -> String { "Make sure you trust \(document) before allowing it" }
        public static func unknownLead(_ folder: String) -> String {
            "Once allowed, this page can read and change the files in \(folder), and send anything it sees anywhere on the internet. Allowing this HTML file holds until the page's code changes."
        }
        public static func unknownProgram(_ name: String) -> String { "If you allow this page, the question about running “\(name)” comes next." }
        public static let unknownDownloaded = "This file was downloaded from the internet."
        public static let unknownUnrecorded = "HTML Clay has no record of where this file came from."
        public static let unknownBrokenSignature = "This file's signature does not match its code."
        public static let trustLikeMine = "Trust it like a file I made"
        public static let useThisFile = "Use this file?"
        public static let riskyNote = "Programs that just arrived are the most common way to be tricked."
        public static let chooseAnother = "Choose another"
        public static let allowAnyway = "Allow anyway"
        public static func readFolder(_ document: String) -> String { "Let \(document) read a folder?" }
        public static func readNote(_ folder: String) -> String { "Allow once lasts as long as this HTML file can save. Trust lets every HTML file in \(folder) save and link." }
        public static let allowOnce = "Allow once"
        public static let trustThisFolder = "Trust this folder"
        public static func trustOwnFolder(_ document: String) -> String { "Trust the folder \(document) is in?" }
        public static let ownFolderNote = "HTML files there will save, link to each other and keep one address."
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
        public static let willSave = " will save as you edit, link to the others, and share one address. HTML files in "
        public static let canChange = " can also change each other."
        public static func inside(_ name: String, count: Int) -> String {
            count == 1 ? "\(name) already has 1 trusted folder inside. It keeps its own address." : "\(name) already has \(count) trusted folders inside. They keep their own address."
        }
        public static func insideTrusted(_ name: String, around: String) -> String { "\(name) is inside \(around). Its HTML files get a new address, and nothing they stored in your browser moves with them." }
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
            case .teamDocument: return "A team HTML file"
            case .riskyFile: return "A risky file"
            case .folderRead: return "Reading outside the folder"
            case .folderReadAgain: return "A page asks to trust its own folder"
            case .builtinProgram: return "A page asks to use a built-in program"
            case .unknownDocument: return "An HTML file nobody vouches for"
            case .aiTools: return "A page asks your AI agent to read a folder"
            }
        }
        public static func wouldOpen(_ name: String) -> String { "Build 1: this would open \(name) in your browser." }
        public static func wouldShowInFinder(_ name: String) -> String { "Build 1: this would show \(name) in Finder." }
        public static func wouldExport(_ name: String) -> String { "Build 1: this would export \(name) as a zip file." }
        public static func wouldRecover(_ name: String) -> String { "Build 1: this would ask where to save a recovered copy of \(name)." }
        public static func persona(_ persona: Persona) -> String {
            switch persona {
            case .newPerson: return "A new person"
            case .fiveDocuments: return "Five HTML files, one file gone"
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
            public static let errCardTitle = "Five HTML files are waiting"
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
            public static let input = "Search HTML files"
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
        public static let title = "Connect Hyperclay"
        public static let introLead = "Hyperclay is a platform that lets you sync your local HTML files to the web, so you can open and collaborate on them from anywhere. "
        public static let signUpLink = "Sign up ↗"
        public static let signUpURL = "https://hyperclay.com"
        public static let wouldOpenSignUp = "Build 1: this would open Hyperclay to sign up."
        public static let username = "Username"
        public static let syncKey = "Sync key"
        public static let keyPlaceholder = "hcsk_…"
        public static let paste = "Paste"
        public static let refused = "That sync key wasn't accepted. Get a new one on Hyperclay."
        public static let getKey = "Get a sync key ↗"
        public static let nothingSent = "Nothing is sent to Hyperclay until you press Connect."
        public static let connect = "Connect"
        public static let wouldOpenKeys = "Build 1: this would open the sync key page on Hyperclay."
        public static let keyPageURL = "https://hyperclay.com/dashboard"
        public static let connecting = "Connecting…"
    }
    public enum AccountFolder {
        public static func title(_ username: String) -> String { "Where should @\(username) live on this Mac?" }
        public static let newFolder = "A new folder "
        public static let existing = "A folder I already have…"
        public static func willDownload(documents: Int, size: String) -> String { "\(documents) HTML files, \(size) will download." }
        public static let willStart = "It will be trusted, served at "
        public static let willEnd = ", and kept in step with Hyperclay."
        public static let back = "Back"
        public static let setUpAndSync = "Set up and sync"
        public static let wouldChooseExisting = "Build 1: this would ask you to choose a folder you already have."
        // copy: account set-up sheet, what a first sync into a folder already there will do; differing files wait in Needs you (review 2), max 100 chars
        public static func willSync(_ counts: SyncCounts) -> String {
            if counts.toUpload == 1 { return "1 file to upload, \(counts.toDownload) to download, \(counts.differ) different. You choose in Needs you. Nothing is deleted." }
            return "\(counts.toUpload) files to upload, \(counts.toDownload) to download, \(counts.differ) different. You choose in Needs you. Nothing is deleted."
        }
        // copy: account set-up sheet, the button after the counts show, max 20 chars
        public static let syncThese = "Sync these files"
        // copy: set-up sheets, while the folder is being set up, max 20 chars
        public static let settingUp = "Setting up…"
    }
    public enum TeamSetup {
        // copy: under the folder path on the team set-up sheet, before any refusal: a team's first sync needs an empty folder, max 60 chars
        public static let emptyFolder = "Choose an empty folder or make a new one."
        // copy: the team set-up sheet's button while the first download runs: it hides the sheet, the download goes on (review 2), max 12 chars
        public static let close = "Close"
        // copy: a note after a set-up whose first download could not finish; the folder and its files stay, and Set up works again, max 100 chars
        public static let firstDownloadFailed = "The first download didn't finish, so nothing is syncing yet. Try Set up again."
        // copy: the same note when the Mac lacks room for the team's files, max 100 chars
        public static let firstDownloadDiskFull = "There isn't enough free space on this Mac. Free up space, then try Set up again."
        // copy: the same note when hyperclay.com could not be reached, max 100 chars
        public static let firstDownloadOffline = "HTML Clay couldn't reach Hyperclay. Check your connection, then try Set up again."
        public static func firstDownloadFailed(_ reason: String?) -> String {
            switch reason {
            case "disk-full": return firstDownloadDiskFull
            case "offline": return firstDownloadOffline
            default: return firstDownloadFailed
            }
        }
        public static func title(_ name: String) -> String { "Set up \(name)" }
        public static let opensAt = "Opens at "
        public static let trustCheck = "Trust the HTML files in this folder. They can change each other."
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
        public static let lead = "No HTML file may run it until you allow one. The first HTML file that asks still gets a request."
        public static let file = "File"
        public static let nameHint = "The name HTML files ask for. Lowercase letters, numbers and hyphens."
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
        public static func comesAlongText(conflicts: Int) -> String { "Versions and \(conflicts) open conflicts come along, still waiting for you. Old bookmarks redirect. Anything a page kept only in browser storage stays at the old address." }
        public static let notNow = "Not now"
        public static let moveFolders = "Move folders"
        // copy: move sheet, what comes along, when the number of conflicts is not known, max 200 chars
        public static let comesAlongLive = "Versions and open conflicts come along. Old bookmarks redirect. Anything a page kept only in browser storage stays at the old address."
        // copy: move sheet, under a folder that stayed with Hyperclay Local after a move, max 90 chars each
        public static func stayed(_ code: String) -> String {
            switch code {
            case "": return ""
            case "not-connected": return "Stayed: your sync key doesn't reach this account. Connect it in Sync, then move again."
            case "local-first-download-unfinished": return "Stayed: Hyperclay Local hasn't finished its first download. Open it and let it finish."
            case "folder-missing": return "Stayed: this folder can't be found where it was. Put it back, then move again."
            case "folder-in-use": return "Stayed: another app is using this folder. Close that app, then move again."
            case "account-already-synced": return "Stayed: this account already syncs to another folder here. Disconnect that one first."
            case "local-running": return "Stayed: Hyperclay Local opened during the move. Quit it, then move again."
            // copy: the four reasons below, from the milestone 5 review (copy pass 5)
            case "folder-trusted-here": return "HTML Clay already has this folder. Remove it from Folders in Settings and move again."
            case "nested-trusted": return "A folder in Folders is inside or contains this one. Remove that folder and move again."
            case "overlaps-served": return "HTML Clay serves a folder in or containing this one. Stop serving that one and move again."
            case "account-gone": return "This folder synced with a team your sync key cannot reach. It stays in Hyperclay Local."
            default: return "Stayed with Hyperclay Local. Try moving again, or check the app log in Settings."
            }
        }
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
        public static func uploading(_ files: Int) -> String { "Uploading \(files) files to Hyperclay. Your files are already saved on this Mac." }
        public static let quitNow = "Quit now"
        public static func cantSaveTitle(_ document: String) -> String { "\(document) can't save" }
        public static let cantSaveMessage = "The file is no longer writable. Quitting now loses the last change."
        public static let showDocument = "Show HTML file"
        public static let retry = "Retry"
        public static let quitAnyway = "Quit anyway"
        public static let unsavedTitle = "You have unsaved edits"
        public static func unsavedBody(_ lines: [String]) -> String {
            "Edits in these HTML files aren't saved yet:\n\n\(lines.joined(separator: "\n"))\n\nSave any open page in its browser tab (Ctrl+S or Cmd+S), then check again. Quitting anyway loses unsaved edits."
        }
        public static let cancel = "Cancel"
        public static let noAnswer = "Pages edited live that HTML Clay could not check"
        public static let checkAndQuit = "Check and quit"
        public static let discardAndQuit = "Discard and quit"
        public static let keepOpen = "Keep open"
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
            case .keyStopped: return "Your sync key no longer works"
            case .addedToTeam: return "You were added to Marketing"
            case .wentBack: return "HTML Clay went back to 2.0"
            case .cantOpen: return "Can't open trip-plan.htmlclay"
            }
        }
        public static func body(_ note: SampleNotification) -> String {
            switch note {
            case .conflict: return "Sync held this file. Choose what to keep in Needs you."
            case .keyStopped: return "Syncing stopped. Your files are safe. Replace your sync key."
            case .addedToTeam: return "Set it up on this Mac in Settings › Folders."
            case .wentBack: return "2.1 didn't start correctly."
            case .cantOpen: return "HTML Clay only opens files in your home folder."
            }
        }
        public static let fallbackNote = "Shown here because notifications are off for HTML Clay Preview."
        // copy: live notifications, one title and body per kind; folder is the synced folder's name, max 70 chars
        public static func title(_ notice: HostNotice, folder: String?) -> String {
            switch notice.kind {
            case .conflict: return folder.map { "\($0): \(notice.name) changed in both places" } ?? "\(notice.name) changed in both places"
            case .keyStopped: return "Your sync key no longer works"
            case .addedToTeam: return "You were added to \(notice.name)"
            }
        }
        public static func body(_ notice: HostNotice) -> String {
            switch notice.kind {
            case .conflict: return "Sync held this file. Choose what to keep in Needs you."
            case .keyStopped: return "Syncing stopped. Your files are safe. Replace your sync key."
            case .addedToTeam: return "Set it up on this Mac in Settings › Folders."
            }
        }
        // copy: live notification card button, max 16 chars
        public static func open(_ notice: HostNotice) -> String {
            switch notice.kind {
            case .conflict: return "Review"
            case .keyStopped: return "Replace key"
            case .addedToTeam: return "Set up"
            }
        }
        // copy: live notification card, why it shows in the corner instead of Notification Center, max 80 chars
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
    public static func offline(_ changes: Int) -> String { "Can't reach Hyperclay. \(changes) changes will sync when you're back online." }
    public static func updateReady(_ version: String) -> String { "HTML Clay \(version) is ready" }
    public static let restart = "Restart"
    public static let finishing = "Finishing saves and sync…"
    public static let restartsSoon = "HTML Clay restarts in a moment"
    public static let hostStopped = "HTML Clay stopped working."
    public static let hostStoppedSub = "Your files are safe. Open HTML files can't save until it restarts."
    public static let notReady = "HTML Clay is still starting. Try again in a moment."
}
extension Strings.FolderPage {
    public static let pausedAll = "Syncing is paused for all folders"
    public static func offline(_ changes: Int) -> String { "Offline, \(changes) changes waiting" }
    public static func viewerSub(_ team: String) -> String { "You're now a viewer on \(team). Syncing stopped. Your files are still here." }
    public static let askOwner = "Ask a team owner to make you an editor."
    public static let open = "Open ↗"
    public static func missingTitle(_ path: String) -> String { "\(path) isn't there any more" }
    public static let missingSub = "Syncing stopped. Nothing was deleted on Hyperclay."
    public static let setUpAgain = "Set up again…"
    public static func wasTeam(_ team: String) -> String { "Was \(team)" }
    public static let wasTeamSub = "Served on this Mac only. Programs still ask one HTML file at a time, as for team folders."
    public static let reconnect = "Reconnect…"
    public static func wouldSetUpAgain(_ name: String) -> String { "Build 1: this would set up \(name) again in a new folder." }
}
extension Strings { public enum History {
    public static func title(_ folder: String) -> String { "Folder history · \(folder)" }
    public static let showSyncLog = "Show sync log"
    public static let today = "Today"
    // copy: folder history rows, one per kind, the file's path after it, max 60 chars
    public static func entry(_ kind: HistoryEntry.Kind, _ path: String) -> String {
        switch kind {
        case .uploaded: return "Uploaded \(path)"
        case .downloaded: return "Downloaded \(path)"
        case .renamed: return "Renamed \(path)"
        case .removed: return "Removed \(path)"
        case .conflict: return "Conflict in \(path)"
        case .settled: return "Settled a conflict in \(path)"
        }
    }
    // copy: folder history, when sync has done nothing in this folder yet, max 60 chars
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
