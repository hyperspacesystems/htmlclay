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
