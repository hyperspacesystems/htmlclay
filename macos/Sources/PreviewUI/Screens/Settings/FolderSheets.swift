import SwiftUI
import PreviewCore

struct ServeSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    private let folderID: String?
    @State private var pick: Int
    @State private var name: String

    init(folderID: String?, snapshot: HostSnapshot) {
        self.folderID = folderID
        _pick = State(initialValue: ServeRules.firstPick(in: snapshot))
        _name = State(initialValue: ServeRules.startName(folderID: folderID, in: snapshot))
    }

    var body: some View {
        let path = ServeRules.path(folderID: folderID, pick: pick, in: model.snapshot)
        let pathProblem = ServeRules.pathProblem(path, folderID: folderID, in: model.snapshot)
        let nameProblem = ServeRules.nameProblem(name, folderID: folderID, in: model.snapshot)
        let ready = ServeRules.canServe(path: path, name: name, folderID: folderID, in: model.snapshot)
        KilnDialog(Strings.ServeSheet.title) {
            KilnLead(Text(Strings.ServeSheet.leadStart) + Text(Strings.ServeSheet.leadMono).monospaced() + Text(Strings.ServeSheet.leadEnd))
                .padding(.top, 6)
            KilnField(Strings.ServeSheet.folder) {
                KilnInputBox(path)
                if folderID == nil {
                    Button(Strings.Sheets.choose) {
                        pick += 1
                        name = ServeRules.defaultName(for: ServeRules.path(folderID: nil, pick: pick, in: model.snapshot))
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
            }
            .padding(.top, 14)
            if let pathProblem {
                KilnFieldNote(Text(pathProblem), error: true)
            }
            KilnField(Strings.ServeSheet.name) {
                KilnInput(text: $name, placeholder: Strings.ServeSheet.name, mono: true)
            }
            .padding(.top, 10)
            if let nameProblem {
                KilnFieldNote(Text(nameProblem), error: true)
            }
            KilnFieldNote(Text(Strings.ServeSheet.addressPrefix) + Text(ServeRules.address(for: name)).monospaced())
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.ServeSheet.serve) {
                model.send(.serveFolder(path: path, name: name.lowercased()))
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!ready)
            .opacity(ready ? 1 : 0.42)
        }
    }
}

struct TrustSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    private let name: String
    private let path: String

    init(name: String, path: String) {
        self.name = name
        self.path = path
    }

    var body: some View {
        if TrustRules.isMainFolder(path) {
            mainFolder
        } else {
            confirm
        }
    }

    private var mainFolder: some View {
        KilnAlertBox(title: Strings.TrustSheet.mainTitle(name),
                     message: Text(Strings.TrustSheet.mainStart(name)) + Text(Strings.TrustSheet.htmlclay).monospaced() + Text(Strings.TrustSheet.mainEnd)) {
            EmptyView()
        } buttons: {
            Button(Strings.TrustSheet.chooseAnother) {
                if let next = TrustRules.pick(after: path, in: model.snapshot) {
                    model.sheet = .trustFolder(name: next.name, path: next.path)
                } else {
                    model.sheet = nil
                }
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.Request.trustNamed(name)) {
                trust()
            }
            .buttonStyle(KilnButtonStyle(.plain))
        }
    }

    private var confirm: some View {
        let inside = TrustRules.trustedInside(path, in: model.snapshot)
        return KilnAlertBox(title: Strings.TrustSheet.title(name),
                            message: Text(Strings.TrustSheet.every) + Text(Strings.TrustSheet.htmlclay).monospaced() + Text(Strings.TrustSheet.fileIn) + Text(path).monospaced() + Text(Strings.TrustSheet.willSave) + Text(name).monospaced() + Text(Strings.TrustSheet.canChange)) {
            if inside > 0 {
                KilnCard {
                    HStack(spacing: 11) {
                        Squircle(.sky, symbol: Symbols.folder)
                        Text(Strings.TrustSheet.inside(name, count: inside))
                            .font(.system(size: 13))
                            .foregroundStyle(kiln.muted)
                            .multilineTextAlignment(.leading)
                            .fixedSize(horizontal: false, vertical: true)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                }
                .padding(.top, 14)
            }
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.Request.trustNamed(name)) {
                trust()
            }
            .buttonStyle(KilnButtonStyle(.primary))
        }
    }

    private func trust() {
        model.send(.trustFolder(name: name, path: path))
        model.sheet = nil
    }
}
