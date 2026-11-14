import SwiftUI
import PreviewCore

struct ServeSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var sheet: ServeSheetModel

    init(folderID: String?, model: AppModel) {
        _sheet = State(initialValue: ServeSheetModel(folderID: folderID, model: model))
    }

    var body: some View {
        let ready = sheet.ready
        KilnDialog(Strings.ServeSheet.title) {
            KilnLead(Text(Strings.ServeSheet.leadStart) + Text(Strings.ServeSheet.leadMono).monospaced() + Text(Strings.ServeSheet.leadEnd))
                .padding(.top, 6)
            KilnField(Strings.ServeSheet.folder) {
                if sheet.path.isEmpty {
                    KilnInput(text: .constant(String()), placeholder: Strings.Sheets.choose)
                } else {
                    KilnInputBox(sheet.path)
                }
                if sheet.nameEditable {
                    Button(Strings.Sheets.choose) {
                        Task {
                            await sheet.choose()
                            await sheet.check()
                        }
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
            }
            .padding(.top, 14)
            if let pathProblem = sheet.pathProblem {
                KilnFieldNote(Text(pathProblem), error: true)
            }
            KilnField(Strings.ServeSheet.name) {
                if sheet.nameEditable {
                    KilnInput(text: Bindable(sheet).name, placeholder: Strings.ServeSheet.name, mono: true)
                } else {
                    KilnInputBox(sheet.name)
                }
            }
            .padding(.top, 10)
            if let nameProblem = sheet.nameProblem {
                KilnFieldNote(Text(nameProblem), error: true)
            }
            if let otherProblem = sheet.otherProblem {
                KilnFieldNote(Text(otherProblem), error: true)
            }
            KilnFieldNote(Text(Strings.ServeSheet.addressPrefix) + Text(sheet.address.isEmpty ? ServeRules.address(for: sheet.name) : sheet.address).monospaced())
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.ServeSheet.serve) {
                Task {
                    if await sheet.serve() { model.sheet = nil }
                }
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!ready)
            .opacity(ready ? 1 : 0.42)
        }
        .task(id: [sheet.path, sheet.name]) {
            try? await Task.sleep(nanoseconds: 250_000_000)
            await sheet.check()
        }
    }
}

struct TrustSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var sheet: TrustSheetModel

    init(name: String, path: String, model: AppModel) {
        _sheet = State(initialValue: TrustSheetModel(name: name, path: path, model: model))
    }

    var body: some View {
        Group {
            if sheet.isMainFolder {
                mainFolder
            } else {
                confirm
            }
        }
        .task { await sheet.check() }
    }

    private var mainFolder: some View {
        KilnAlertBox(title: Strings.TrustSheet.mainTitle(sheet.name),
                     message: Text(Strings.TrustSheet.mainStart(sheet.name)) + Text(Strings.TrustSheet.htmlclay).monospaced() + Text(Strings.TrustSheet.mainEnd)) {
            EmptyView()
        } buttons: {
            Button(Strings.TrustSheet.chooseAnother) {
                Task {
                    if !(await sheet.chooseAnother()) { model.sheet = nil }
                }
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.Request.trustNamed(sheet.name)) {
                Task {
                    if await sheet.trust() { model.sheet = nil }
                }
            }
            .buttonStyle(KilnButtonStyle(.plain))
            .disabled(!sheet.ready)
            .opacity(sheet.ready ? 1 : 0.42)
        }
    }

    private var confirm: some View {
        KilnAlertBox(title: Strings.TrustSheet.title(sheet.name),
                     message: Text(Strings.TrustSheet.every) + Text(Strings.TrustSheet.htmlclay).monospaced() + Text(Strings.TrustSheet.fileIn) + Text(sheet.path).monospaced() + Text(Strings.TrustSheet.willSave) + Text(sheet.name).monospaced() + Text(Strings.TrustSheet.canChange)) {
            if !sheet.insideNames.isEmpty {
                KilnCard {
                    HStack(spacing: 11) {
                        Squircle(.sky, symbol: Symbols.folder)
                        Text(Strings.TrustSheet.inside(sheet.name, count: sheet.insideNames.count))
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
            if let problem = sheet.problem {
                KilnFieldNote(Text(problem), error: true)
            }
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.Request.trustNamed(sheet.name)) {
                Task {
                    if await sheet.trust() { model.sheet = nil }
                }
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!sheet.ready)
            .opacity(sheet.ready ? 1 : 0.42)
        }
    }
}
