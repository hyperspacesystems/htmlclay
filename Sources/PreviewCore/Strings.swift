import Foundation

public enum Strings {
    public enum App {
        public static let name = "HTML Clay Preview"
    }
    public enum Panel {
        public static let openDashboard = "Open dashboard"
        public static let recentDocuments = "Recent documents"
        public static let noDocumentsTitle = "No documents yet"
        public static let openAFile = "Open a file…"
    }
    public enum Menu {
        public static let settings = "Settings…"
        public static let quit = "Quit HTML Clay"
        public static func needsYou(_ count: Int) -> String { "Needs you (\(count))" }
    }
    public enum Welcome {
        public static let title = "HTML Clay is ready"
        public static let allSet = "I'm all set"
        public static let openSettings = "Open Settings"
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
        public static let menuTitle = "Preview"
        public static let pretend = "Pretend I am"
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
