import PreviewCore

public struct PanelActions {
    public var openMenu: () -> Void
    public var openDashboard: () -> Void
    public var openSettings: (SettingsPane) -> Void
    public var answer: (WaitingItem) -> Void
    public var quit: () -> Void
    public init(openMenu: @escaping () -> Void = {}, openDashboard: @escaping () -> Void = {}, openSettings: @escaping (SettingsPane) -> Void = { _ in }, answer: @escaping (WaitingItem) -> Void = { _ in }, quit: @escaping () -> Void = {}) {
        self.openMenu = openMenu
        self.openDashboard = openDashboard; self.openSettings = openSettings; self.answer = answer; self.quit = quit
    }
}
