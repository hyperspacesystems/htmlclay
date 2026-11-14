public enum RequestQueue {
    public static func items(_ snapshot: HostSnapshot) -> [WaitingItem] {
        snapshot.waiting.filter { $0.kind == .programRequest }
    }
    public static func position(of id: String, in snapshot: HostSnapshot) -> (position: Int, total: Int) {
        let all = items(snapshot)
        return ((all.firstIndex { $0.id == id } ?? 0) + 1, all.count)
    }
    public static func next(after id: String, in snapshot: HostSnapshot) -> WaitingItem? {
        items(snapshot).first { $0.id != id }
    }
    public static func counter(position: Int, total: Int) -> String? {
        total >= 2 ? Strings.Request.counter(position, total) : nil
    }
}

/// Which question the Mac app raises on its own. The dashboard can ask for the
/// request window and nothing else: it sends `request.show`, the host adds one
/// to that question's attention, and the Mac app raises the window for a count
/// that went up. A question the person already looked at is not raised again,
/// and a question that is gone asks for nothing.
public enum RequestAttention {
    public static func raised(from previous: HostSnapshot, to next: HostSnapshot) -> ProgramRequest? {
        var seen: [String: Int] = [:]
        for request in previous.requests { seen[request.id] = request.attention ?? 0 }
        return next.requests.first { ($0.attention ?? 0) > (seen[$0.id] ?? 0) }
    }
}
