import Foundation
import Observation

/// What the Serve sheet holds and asks: the host rules on the folder and name,
/// and a refusal keeps what the person typed.
@MainActor
@Observable
public final class ServeSheetModel {
    public let folderID: String?
    public private(set) var path: String
    public var name: String
    public let nameEditable: Bool
    public private(set) var address = ""
    public private(set) var pathProblem: String?
    public private(set) var nameProblem: String?
    public private(set) var otherProblem: String?
    public private(set) var checking = false
    public private(set) var serving = false
    public var ready: Bool {
        !path.isEmpty && !(nameEditable && name.isEmpty) && pathProblem == nil && nameProblem == nil && otherProblem == nil && !checking && !serving
    }

    private let model: AppModel
    private var checkToken = 0

    public init(folderID: String?, model: AppModel) {
        self.folderID = folderID
        self.model = model
        let snapshot = model.snapshot
        if let folderID {
            let folder = snapshot.folder(folderID)
            let known = folder.map { $0.fullPath.isEmpty ? $0.path : $0.fullPath } ?? ""
            let current = ServeRules.addressName(folder?.address ?? "")
            path = known
            name = current.isEmpty ? ServeRules.defaultName(for: known) : current
            nameEditable = false
        } else {
            let first = model.folderChooser?.first(in: snapshot) ?? ""
            path = first
            name = first.isEmpty ? "" : ServeRules.defaultName(for: first)
            nameEditable = true
        }
    }

    /// Asks the host about the current path and name; sets address or the problem under the right field.
    public func check() async {
        checkToken += 1
        let token = checkToken
        guard !path.isEmpty else {
            pathProblem = nil
            nameProblem = nil
            otherProblem = nil
            address = ""
            checking = false
            return
        }
        checking = true
        let answer = await model.check(action)
        guard token == checkToken else { return }
        checking = false
        switch answer {
        case let .success(check):
            address = check.address
            pathProblem = nil
            nameProblem = nil
            otherProblem = nil
        case let .failure(refusal):
            address = ""
            note(refusal)
        }
    }

    /// Lets the person pick another folder; a new folder's name becomes its default name (lowercased last component, spaces to hyphens).
    public func choose() async {
        guard let next = await model.folderChooser?.choose(after: path.isEmpty ? nil : path, in: model.snapshot) else { return }
        path = next
        name = ServeRules.defaultName(for: next)
        address = ""
        pathProblem = nil
        nameProblem = nil
        otherProblem = nil
    }

    /// Serves. True when the host did it (the sheet closes); false keeps every field and shows the refusal.
    public func serve() async -> Bool {
        serving = true
        switch await model.submit(action) {
        case .done:
            return true
        case let .refused(refusal):
            serving = false
            guard refusal.code != "duplicate", refusal.code != "queued" else { return false }
            note(refusal)
            return false
        }
    }

    private var action: HostAction {
        if let folderID { return .setServed(folderID: folderID, on: true) }
        return .serveFolder(path: path, name: name.lowercased())
    }

    private func note(_ refusal: HostRefusal) {
        pathProblem = refusal.field == "path" ? refusal.text : nil
        nameProblem = refusal.field == "name" ? refusal.text : nil
        otherProblem = refusal.field == "path" || refusal.field == "name" ? nil : refusal.text
    }
}
