import Foundation

/// Makes a persona real on a throwaway home: folders and documents on disk, then
/// the host's own actions, so nothing here is hand-written sample data.
public enum ProfileSeeder {
    public enum Refusal: Error, Equatable, CustomStringConvertible {
        case sampleOnly(persona: Persona, milestone: Int)
        case noFolder(persona: Persona)
        case outsideHome(path: String)
        case refused(action: String, code: String, text: String)

        public var description: String {
            switch self {
            case let .sampleOnly(persona, milestone):
                return "\(persona.rawValue) is sample only until milestone \(milestone)"
            case let .noFolder(persona):
                return "\(persona.rawValue) has no folder to seed"
            case let .outsideHome(path):
                return "the sample folder \(path) is not under ~"
            case let .refused(action, code, text):
                return "\(action) was refused: \(code) \(text)"
            }
        }
    }

    /// Makes the persona's folders and files under home and sends the real actions. Throws on a refusal, naming the action.
    public static func seed(_ persona: Persona, home: URL, host: HostClient) async throws {
        switch persona {
        case .newPerson:
            return
        case .fiveDocuments, .syncedWithTeams, .localUser:
            throw Refusal.sampleOnly(persona: persona, milestone: persona.milestone ?? 0)
        case .trustedFolder, .servedFolder:
            let sample = SampleData.snapshot(for: persona)
            guard let folder = folder(for: persona, in: sample) else { throw Refusal.noFolder(persona: persona) }
            let path = try place(folder, under: home)
            try write(documents(in: folder, of: sample), to: path)
            try await send(.trustFolder(name: folder.name, path: path.path), as: "folder.trust", to: host)
            guard persona == .servedFolder else { return }
            try await send(.serveFolder(path: path.path, name: servedName(folder)), as: "folder.serve", to: host)
        }
    }

    private static func folder(for persona: Persona, in sample: HostSnapshot) -> Folder? {
        switch persona {
        case .servedFolder: return sample.folders.first { $0.served }
        default: return sample.folders.first { $0.parentID == nil && !$0.served }
        }
    }

    private static func documents(in folder: Folder, of sample: HostSnapshot) -> [String] {
        var seen: Set<String> = []
        return sample.documents.filter { document in
            let placed = document.place == folder.path
                || URL(fileURLWithPath: document.place).lastPathComponent == folder.name
            guard placed, !seen.contains(document.name) else { return false }
            seen.insert(document.name)
            return true
        }.map(\.name)
    }

    private static func servedName(_ folder: Folder) -> String {
        guard let label = folder.address.split(separator: ".").first, !label.isEmpty else { return folder.name }
        return String(label)
    }

    private static func place(_ folder: Folder, under home: URL) throws -> URL {
        guard folder.path.hasPrefix("~/") else { throw Refusal.outsideHome(path: folder.path) }
        return home.appendingPathComponent(String(folder.path.dropFirst(2)), isDirectory: true)
    }

    private static func write(_ names: [String], to directory: URL) throws {
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        for name in names {
            let body = "<!doctype html><title>\(name)</title><p>\(name)"
            try Data(body.utf8).write(to: directory.appendingPathComponent(name))
        }
    }

    private static func send(_ action: HostAction, as name: String, to host: HostClient) async throws {
        switch await host.perform(action) {
        case .done: return
        case let .refused(refusal): throw Refusal.refused(action: name, code: refusal.code, text: refusal.text)
        }
    }
}
