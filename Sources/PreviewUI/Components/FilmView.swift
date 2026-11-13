import SwiftUI
import WebKit

public struct FilmView: NSViewRepresentable {
    private let html: String?
    public init(html: String?) { self.html = html }

    public func makeCoordinator() -> Coordinator { Coordinator() }

    public func makeNSView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .nonPersistent()
        let view = WKWebView(frame: .zero, configuration: configuration)
        view.navigationDelegate = context.coordinator
        view.setValue(false, forKey: WebKeys.drawsBackground)
        if let html { view.loadHTMLString(html, baseURL: nil) }
        return view
    }

    public func updateNSView(_ view: WKWebView, context: Context) {}

    public final class Coordinator: NSObject, WKNavigationDelegate {
        public func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
            decisionHandler(navigationAction.navigationType == .linkActivated ? .cancel : .allow)
        }
    }
}
