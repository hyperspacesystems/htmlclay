import SwiftUI
import WebKit
import PreviewCore

public struct PanelActions {
    public var openDashboard: () -> Void
    public var openSettings: (SettingsPane) -> Void
    public init(openDashboard: @escaping () -> Void = {}, openSettings: @escaping (SettingsPane) -> Void = { _ in }) {
        self.openDashboard = openDashboard; self.openSettings = openSettings
    }
}

public struct PanelView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    private let actions: PanelActions
    public init(actions: PanelActions = PanelActions()) { self.actions = actions }
    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Button(Strings.Panel.openDashboard, action: actions.openDashboard)
                .buttonStyle(KilnPrimaryButtonStyle())
            if model.snapshot.documents.isEmpty {
                KilnCard { KilnRow(title: Strings.Panel.noDocumentsTitle) { EmptyView() } }
            } else {
                Text(Strings.Panel.recentDocuments).font(.system(size: 13, weight: .semibold)).foregroundStyle(kiln.ink2)
                KilnCard {
                    ForEach(model.snapshot.documents) { document in
                        KilnRow(title: document.name, subtitle: document.place) { EmptyView() }
                    }
                }
            }
        }
        .padding(12)
        .frame(width: 382)
    }
}

public struct SettingsView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    public init() {}
    public var body: some View {
        @Bindable var model = model
        HStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 2) {
                ForEach(SettingsPane.allCases, id: \.self) { pane in
                    Button { model.settingsPane = pane } label: {
                        HStack {
                            Text(Strings.Settings.title(pane)).font(.system(size: 13.5, weight: model.settingsPane == pane ? .semibold : .medium))
                            Spacer()
                            if model.snapshot.count(in: pane) > 0 { KilnBadge(model.snapshot.count(in: pane)) }
                        }
                        .padding(.horizontal, 9).padding(.vertical, 6)
                        .background(model.settingsPane == pane ? kiln.sideActive : .clear, in: RoundedRectangle(cornerRadius: 7))
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                }
                Spacer()
            }
            .padding(.top, 40).padding(.horizontal, 9).padding(.bottom, 14)
            .frame(width: 214)
            .background(kiln.side, in: RoundedRectangle(cornerRadius: 10))
            .padding(.leading, 8).padding(.vertical, 8)

            VStack(alignment: .leading, spacing: 12) {
                Text(Strings.Settings.title(model.settingsPane)).font(.system(size: 18, weight: .bold))
                KilnCard {
                    ForEach(model.snapshot.folders) { folder in
                        KilnRow(title: folder.name, subtitle: folder.path) {
                            Toggle(isOn: Binding(get: { folder.served }, set: { model.send(.setServed(folderID: folder.id, on: $0)) })) { EmptyView() }
                                .toggleStyle(KilnSwitchStyle())
                        }
                    }
                }
                Spacer()
            }
            .padding(.horizontal, 24).padding(.top, 18).padding(.bottom, 24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .frame(width: 840, height: 490)
    }
}

public struct FilmView: NSViewRepresentable {
    private let html: String?
    public init(html: String?) { self.html = html }
    public func makeNSView(context: Context) -> WKWebView {
        let view = WKWebView(frame: .zero, configuration: WKWebViewConfiguration())
        view.setValue(false, forKey: "drawsBackground")
        if let html { view.loadHTMLString(html, baseURL: nil) }
        return view
    }
    public func updateNSView(_ view: WKWebView, context: Context) {}
}

public struct WelcomeView: View {
    @Environment(\.kiln) private var kiln
    private let film: String?
    private let done: () -> Void
    private let openSettings: () -> Void
    public init(film: String?, done: @escaping () -> Void = {}, openSettings: @escaping () -> Void = {}) {
        self.film = film; self.done = done; self.openSettings = openSettings
    }
    public var body: some View {
        VStack(spacing: 14) {
            FilmView(html: film).frame(width: 104, height: 104)
            Text(Strings.Welcome.title).font(.system(size: 23, weight: .bold))
            HStack {
                Button(Strings.Welcome.openSettings, action: openSettings)
                Button(Strings.Welcome.allSet, action: done).buttonStyle(KilnPrimaryButtonStyle()).frame(width: 140)
            }
        }
        .padding(34)
        .frame(width: 760)
    }
}

public struct RequestView: View {
    private let item: WaitingItem
    private let answer: (Bool) -> Void
    public init(item: WaitingItem, answer: @escaping (Bool) -> Void = { _ in }) { self.item = item; self.answer = answer }
    public var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text(item.title).font(.system(size: 16, weight: .bold))
            HStack {
                Spacer()
                Button(Strings.Request.dontAllow) { answer(false) }
                Button(Strings.Request.allow) { answer(true) }
            }
        }
        .padding(22)
        .frame(width: 460)
    }
}

@MainActor
public enum ScreenCatalog {
    public static func all(model: AppModel, film: String?) -> [(name: String, view: AnyView)] {
        var screens: [(name: String, view: AnyView)] = [
            ("panel", AnyView(KilnRoot { PanelView() }.environment(model))),
            ("settings", AnyView(KilnRoot { SettingsView() }.environment(model))),
            ("welcome", AnyView(KilnRoot { WelcomeView(film: film) }.environment(model))),
            ("components", AnyView(KilnRoot { ComponentGallery() }.environment(model))),
        ]
        if let item = model.snapshot.waiting.first {
            screens.append(("request", AnyView(KilnRoot { RequestView(item: item) }.environment(model))))
        }
        return screens
    }
}
