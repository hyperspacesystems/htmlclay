import Foundation

@MainActor
public protocol HostClient: AnyObject {
    func load() async throws -> HostSnapshot
    func perform(_ action: HostAction) async throws -> HostSnapshot
    func subscribe(_ onChange: @escaping @MainActor (HostSnapshot) -> Void)
}
