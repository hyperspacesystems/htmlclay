import Foundation

public enum ServeRules {
    public static func addressName(_ address: String) -> String {
        address.components(separatedBy: ".").first ?? ""
    }
    public static func defaultName(for path: String) -> String {
        (path as NSString).lastPathComponent.lowercased().replacingOccurrences(of: " ", with: "-")
    }
    public static func address(for name: String) -> String {
        name.lowercased() + SampleReducer.addressSuffix
    }
    public static func path(folderID: String?, pick: Int, in snapshot: HostSnapshot) -> String {
        if let folderID, let folder = snapshot.folder(folderID) { return folder.path }
        return SampleData.servePaths[pick % SampleData.servePaths.count]
    }
    public static func firstPick(in snapshot: HostSnapshot) -> Int {
        SampleData.servePaths.firstIndex { pathProblem($0, folderID: nil, in: snapshot) == nil } ?? 0
    }
    public static func startName(folderID: String?, in snapshot: HostSnapshot) -> String {
        if let folderID, let folder = snapshot.folder(folderID) {
            let current = addressName(folder.address)
            return current.isEmpty ? defaultName(for: folder.path) : current
        }
        return defaultName(for: path(folderID: nil, pick: firstPick(in: snapshot), in: snapshot))
    }
    public static func pathProblem(_ path: String, folderID: String?, in snapshot: HostSnapshot) -> String? {
        if snapshot.localMoveOffer && SampleData.hyperclayLocalPaths.contains(path) { return Strings.ServeSheet.localStillServing }
        for folder in snapshot.folders where folder.served && folder.id != folderID && !folder.path.isEmpty {
            if folder.path == path { return Strings.ServeSheet.alreadyServed(folder.name) }
            if path.hasPrefix(folder.path + "/") { return Strings.ServeSheet.insideServed(folder.name) }
        }
        return nil
    }
    public static func nameProblem(_ name: String, folderID: String?, in snapshot: HostSnapshot) -> String? {
        let wanted = name.lowercased()
        guard !wanted.isEmpty else { return nil }
        let taken = snapshot.folders.contains { $0.id != folderID && $0.served && addressName($0.address) == wanted }
        return taken ? Strings.Sheets.taken(name) : nil
    }
    public static func canServe(path: String, name: String, folderID: String?, in snapshot: HostSnapshot) -> Bool {
        !name.isEmpty && pathProblem(path, folderID: folderID, in: snapshot) == nil && nameProblem(name, folderID: folderID, in: snapshot) == nil
    }
}

/// The refusals a person can simply try again: the host was not there, or it was
/// interrupted. Those keep the sheet's button live; anything else keeps the sheet open.
public enum SheetRules {
    public static func isRetryable(_ refusal: HostRefusal) -> Bool {
        refusal.retryable || ["interrupted", "unreachable", "host-error"].contains(refusal.code)
    }
}

public enum MoveRules {
    public static func ordered(_ checked: Set<String>) -> [String] {
        SampleData.localMoveFolders.map(\.id).filter { checked.contains($0) }
    }
    public static func canMove(blockers: Int, checked: Set<String>, key: String) -> Bool {
        blockers == 0 && !checked.isEmpty && !key.isEmpty
    }
}

public enum TrustRules {
    /// The host's code for a folder inside a trusted folder: the one trust warning the person has to accept.
    public static let insideTrusted = "inside-trusted"
    public static func isMainFolder(_ path: String) -> Bool {
        Strings.Request.mainFolders.contains(path)
    }
    public static func trustedInside(_ path: String, in snapshot: HostSnapshot) -> Int {
        snapshot.localFolders.filter { $0.path.hasPrefix(path + "/") }.count
    }
    public static func firstPick(in snapshot: HostSnapshot) -> TrustCandidate? {
        SampleData.trustCandidates.first { candidate in !snapshot.folders.contains { $0.path == candidate.path } }
    }
    public static func pick(after path: String, in snapshot: HostSnapshot) -> TrustCandidate? {
        SampleData.trustCandidates.first { candidate in
            candidate.path != path && !isMainFolder(candidate.path) && !snapshot.folders.contains { $0.path == candidate.path }
        }
    }
}

public enum ConnectRules {
    public static func accepts(_ key: String) -> Bool {
        key.hasPrefix("hcsk_") && key.count >= 12
    }
}

public enum TeamSetupRules {
    public static func samplePaths(for folder: Folder) -> [String] {
        ["~/hyperclay/" + folder.name, "~/Documents", "~/hyperclay/panphora/" + ServeRules.defaultName(for: folder.name)]
    }
    public static func address(for folder: Folder) -> String {
        ServeRules.address(for: ServeRules.defaultName(for: folder.name))
    }
    public static func problem(_ path: String, in snapshot: HostSnapshot) -> String? {
        if TrustRules.isMainFolder(path) { return Strings.TeamSetup.notEmpty }
        if let outer = snapshot.folders.first(where: { !$0.path.isEmpty && path.hasPrefix($0.path + "/") }) { return Strings.TeamSetup.inside(outer.name) }
        return nil
    }
}

public enum ProgramNameProblem: Equatable, Sendable { case invalid, taken }

public enum ProgramRules {
    public static func defaultName(for path: String) -> String {
        ((path as NSString).lastPathComponent as NSString).deletingPathExtension.lowercased()
    }
    public static func nameProblem(_ name: String, in snapshot: HostSnapshot) -> ProgramNameProblem? {
        let allowed = CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyz0123456789-")
        if name.isEmpty || name.unicodeScalars.contains(where: { !allowed.contains($0) }) { return .invalid }
        if snapshot.programs.contains(where: { $0.name == name }) { return .taken }
        return nil
    }
}
