import Foundation

/// The one path every file open takes: from Finder, the panel, or the menu.
/// It holds opens until the host runs, and tells the person each time the host
/// says no, so a refusal is never written where nobody can see it.
@MainActor
public final class FileOpener {
    /// Called with the file's name and the host's refusal when an open is refused (not for duplicate/queued).
    public var onRefused: ((_ name: String, _ refusal: HostRefusal) -> Void)?
    private let model: AppModel
    private var pending: [String] = []
    private var ready = false

    public init(model: AppModel) { self.model = model }

    /// Opens now when ready, else queues (no duplicates, order kept).
    public func open(_ paths: [String]) {
        guard ready else {
            for path in paths where !pending.contains(path) { pending.append(path) }
            return
        }
        for path in paths { openOne(path) }
    }

    /// The host became ready: open everything queued, in order.
    public func hostReady() {
        ready = true
        let paths = pending
        pending = []
        for path in paths { openOne(path) }
    }

    /// The host stopped: hold new opens until hostReady.
    public func hostGone() { ready = false }

    public var queued: [String] { pending }

    private func openOne(_ path: String) {
        Task {
            guard case let .refused(refusal) = await model.submit(.openFile(path: path)) else { return }
            if refusal.code == "duplicate" || refusal.code == "queued" { return }
            onRefused?((path as NSString).lastPathComponent, refusal)
        }
    }
}
