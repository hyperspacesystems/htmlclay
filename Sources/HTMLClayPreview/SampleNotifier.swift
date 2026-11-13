import AppKit
import UserNotifications
import PreviewCore

@MainActor
final class SampleNotifier: NSObject, UNUserNotificationCenterDelegate {
    private let windows: WindowManager

    init(windows: WindowManager) {
        self.windows = windows
        super.init()
    }

    func send(_ note: SampleNotification) {
        guard Bundle.main.bundleIdentifier != nil else { windows.showNotificationFallback(note); return }
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        center.requestAuthorization(options: [.alert]) { granted, _ in
            Task { @MainActor in
                if granted { self.post(note, center: center) } else { self.windows.showNotificationFallback(note) }
            }
        }
    }

    private func post(_ note: SampleNotification, center: UNUserNotificationCenter) {
        let content = UNMutableNotificationContent()
        content.title = Strings.Notify.title(note)
        content.body = Strings.Notify.body(note)
        center.add(UNNotificationRequest(identifier: note.rawValue + "-" + UUID().uuidString, content: content, trigger: nil))
    }

    nonisolated func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification, withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .list])
    }
}
