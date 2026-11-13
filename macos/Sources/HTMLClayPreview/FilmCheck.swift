import AppKit
import WebKit

@MainActor
enum FilmCheck {
    static func run() -> Int32 {
        guard let html = WindowManager.film() else {
            print("FILM-CHECK FAILED: icon-film.html is not in the bundle")
            return 1
        }
        let view = WKWebView(frame: NSRect(x: 0, y: 0, width: 104, height: 104))
        view.loadHTMLString(html, baseURL: nil)
        var answer: String?
        let deadline = Date().addingTimeInterval(8)
        while answer == nil && Date() < deadline {
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
            guard !view.isLoading else { continue }
            view.evaluateJavaScript("window.iconAnim ? [typeof iconAnim.play, iconAnim.playing, iconAnim.LAST.toFixed(2)].join(' ') : 'none'") { value, _ in
                answer = value as? String
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        }
        guard let answer, answer.hasPrefix("function ") else {
            print("FILM-CHECK FAILED: \(answer ?? "no answer before the deadline")")
            return 1
        }
        print("FILM-CHECK OK \(answer)")
        return 0
    }
}
