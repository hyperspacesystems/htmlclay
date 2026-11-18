import Foundation
import PreviewCore
import SparkleBridge

@MainActor
final class SparkleDriver: UpdateDriver {
    private var bridge: HCSparkleBridge?

    /// Nil when the framework cannot load; the reason goes to stderr.
    static func make(bundle: Bundle = .main) -> SparkleDriver? {
        let path = bundle.bundleURL.appendingPathComponent("Contents/Frameworks/Sparkle.framework").path
        if let reason = HCSparkleBridge.loadFramework(atPath: path) {
            FileHandle.standardError.write(Data("HTML Clay: updates are off: \(reason)\n".utf8))
            return nil
        }
        return SparkleDriver()
    }

    func start(events: @escaping @MainActor (UpdateEvent) -> Void, skipReason: @escaping @MainActor (String) -> String?) {
        let bridge = HCSparkleBridge(events: { kind, text in
            let event: UpdateEvent
            switch kind {
            case .started: event = .started
            case .startFailed: event = .startFailed(text)
            case .ready: event = .ready(text)
            case .aborted: event = .aborted(text)
            case .installFailed: event = .installFailed(text)
            case .checkSkipped: event = .checkSkipped(text)
            case .checking: event = .checking
            case .upToDate: event = .upToDate
            case .checkEnded: event = .checkEnded
            @unknown default: return
            }
            Task { @MainActor in events(event) }
        }, skipReason: { version in
            MainActor.assumeIsolated { skipReason(version) }
        })
        self.bridge = bridge
        bridge.start()
    }

    func installNow() { bridge?.installNow() }
    func checkNow() { bridge?.checkNow() }

    var automatic: Bool {
        get { bridge?.automatic ?? true }
        set { bridge?.automatic = newValue }
    }
}
