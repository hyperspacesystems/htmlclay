import Foundation

public enum SampleData {
    public static func snapshot(for persona: Persona) -> HostSnapshot {
        switch persona {
        case .newPerson:
            return HostSnapshot()
        case .fiveDocuments:
            return HostSnapshot(documents: [
                RecentDocument(id: "budget", name: "budget.htmlclay", place: "Downloads"),
                RecentDocument(id: "recipe", name: "recipe-box.htmlclay", place: "Documents", fileMissing: true),
            ])
        case .trustedFolder:
            return HostSnapshot(
                documents: [RecentDocument(id: "todo", name: "todo.htmlclay", place: "Documents/todo")],
                folders: [Folder(id: "notes", name: "Notes", path: "~/Documents/Notes")],
                waiting: [WaitingItem(id: "req-1", kind: .programRequest, pane: .programs, title: "todo.htmlclay wants to run a program", earnsDot: true)])
        case .servedFolder:
            return HostSnapshot(folders: [Folder(id: "prototypes", name: "prototypes", path: "~/Sites/prototypes", served: true)])
        case .syncedWithTeams, .localUser:
            return HostSnapshot(
                folders: [Folder(id: "panphora", name: "panphora", path: "~/hyperclay/panphora", served: true, synced: true, fromAccount: true)],
                waiting: [
                    WaitingItem(id: "c-1", kind: .conflict, pane: .folders, title: "index.html changed in both places", earnsDot: true),
                    WaitingItem(id: "c-2", kind: .conflict, pane: .folders, title: "about.html changed in both places", earnsDot: true),
                ],
                syncConnected: true)
        }
    }
}

public enum SampleReducer {
    public static func apply(_ action: HostAction, to snapshot: HostSnapshot) -> HostSnapshot {
        var next = snapshot
        switch action {
        case let .setServed(folderID, on):
            guard let i = next.folders.firstIndex(where: { $0.id == folderID }) else { return next }
            next.folders[i].served = on
            if !on { next.folders[i].synced = false }
        case let .stopTrusting(folderID):
            next.folders.removeAll { $0.id == folderID }
        case let .answerRequest(waitingID, _):
            next.waiting.removeAll { $0.id == waitingID }
        }
        return next
    }
}

@MainActor
public final class SampleHost: HostClient {
    public private(set) var persona: Persona
    public private(set) var current: HostSnapshot
    private var listeners: [@MainActor (HostSnapshot) -> Void] = []

    public init(persona: Persona) {
        self.persona = persona
        self.current = SampleData.snapshot(for: persona)
    }

    public func pretend(_ persona: Persona) {
        self.persona = persona
        current = SampleData.snapshot(for: persona)
        listeners.forEach { $0(current) }
    }

    public func load() async throws -> HostSnapshot { current }

    public func perform(_ action: HostAction) async throws -> HostSnapshot {
        current = SampleReducer.apply(action, to: current)
        listeners.forEach { $0(current) }
        return current
    }

    public func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void) {
        listeners.append(onChange)
    }
}
