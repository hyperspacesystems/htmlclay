import AppKit

@main
@MainActor
enum Entry {
    private static var delegate: AppDelegate?

    static func main() {
        let app = NSApplication.shared
        if CommandLine.arguments.contains("--self-check") {
            exit(SelfCheck.run())
        }
        if CommandLine.arguments.contains("--film-check") {
            exit(FilmCheck.run())
        }
        let delegate = AppDelegate()
        Entry.delegate = delegate
        app.delegate = delegate
        app.setActivationPolicy(.accessory)
        app.run()
    }
}
