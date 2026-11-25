import AppKit
import UserNotifications
import PreviewCore
import AppShell

/// Shows the host's notices (08-sync.md decision 19) in Notification Center, or
/// as a card in the corner when notifications are off. Clicking one opens what
/// it is about.
@MainActor
final class HostNotifier: NSObject, UNUserNotificationCenterDelegate {
    private static let targetKey = "target"
    private let windows: WindowManager
    private let model: AppModel

    init(windows: WindowManager, model: AppModel) {
        self.windows = windows
        self.model = model
        super.init()
    }

    func show(_ notice: HostNotice) {
        guard Bundle.main.bundleIdentifier != nil else { windows.showNoticeFallback(notice); return }
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        center.requestAuthorization(options: [.alert]) { granted, _ in
            Task { @MainActor in
                if granted { self.post(notice, center: center) } else { self.windows.showNoticeFallback(notice) }
            }
        }
    }

    private func post(_ notice: HostNotice, center: UNUserNotificationCenter) {
        let content = UNMutableNotificationContent()
        content.title = Strings.Notify.title(notice, folder: notice.folderID.flatMap { model.snapshot.folder($0)?.name })
        content.body = Strings.Notify.body(notice)
        content.userInfo = [Self.targetKey: notice.kind.rawValue]
        center.add(UNNotificationRequest(identifier: notice.id, content: content, trigger: nil))
    }

    nonisolated func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification, withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .list])
    }

    nonisolated func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse, withCompletionHandler completionHandler: @escaping () -> Void) {
        let raw = response.notification.request.content.userInfo[Self.targetKey] as? String
        Task { @MainActor in
            if let kind = raw.flatMap(HostNotice.Kind.init(rawValue:)) {
                self.windows.open(HostNotice(id: "", kind: kind).opens)
            }
            completionHandler()
        }
    }
}
