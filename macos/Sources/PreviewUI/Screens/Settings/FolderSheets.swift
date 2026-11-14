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
                    KilnInput(text: Binding(get: { sheet.name }, set: { sheet.setName($0) }), placeholder: Strings.ServeSheet.name, mono: true)
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
        self.init(sheet: TrustSheetModel(name: name, path: path, model: model))
    }

    init(sheet: TrustSheetModel) {
        _sheet = State(initialValue: sheet)
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
            newAddressWarning
        } buttons: {
            Button(Strings.TrustSheet.chooseAnother) {
                Task {
                    if !(await sheet.chooseAnother()) { model.sheet = nil }
                }
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(sheet.trustTitle) {
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
            newAddressWarning
            if let problem = sheet.problem {
                KilnFieldNote(Text(problem), error: true)
            }
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(sheet.trustTitle) {
                Task {
                    if await sheet.trust() { model.sheet = nil }
                }
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!sheet.ready)
            .opacity(sheet.ready ? 1 : 0.42)
        }
    }

    @ViewBuilder private var newAddressWarning: some View {
        if let warning = sheet.newAddressWarning {
            KilnCard(kind: .warn) {
                HStack(alignment: .top, spacing: 11) {
                    Image(systemName: Symbols.warning)
                        .font(.system(size: 13))
                        .foregroundStyle(kiln.warnFg)
                    Text(warning)
                        .font(.system(size: 13))
                        .foregroundStyle(kiln.warnFg)
                        .multilineTextAlignment(.leading)
                        .fixedSize(horizontal: false, vertical: true)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
            }
            .padding(.top, 14)
        }
    }
}

struct MergeSheet: View {
    @Environment(AppModel.self) private var model
    @State private var sheet: MergeSheetModel

    init(childID: String, model: AppModel) {
        _sheet = State(initialValue: MergeSheetModel(childID: childID, model: model))
    }

    var body: some View {
        let ready = sheet.ready
        KilnDialog(Strings.FolderPage.mergeTitle(child: childName, parent: parentName)) {
            KilnLead(Text(Strings.FolderPage.mergeMessage(child: childName,
                                                          parent: parentName,
                                                          documents: childDocumentCount)))
                .padding(.top, 6)
            if let problem = sheet.problem {
                KilnFieldNote(Text(problem), error: true)
            }
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.FolderPage.mergeAction(parent: parentName)) {
                Task { await sheet.confirm() }
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!ready)
            .opacity(ready ? 1 : 0.42)
        }
    }

    private var child: Folder? { model.snapshot.folder(sheet.childID) }
    private var childName: String { child?.name ?? String() }
    private var parentName: String { child?.parentID.flatMap(model.snapshot.folder)?.name ?? String() }
    private var childDocumentCount: Int { child?.documentCount ?? 0 }
}

struct FolderRelinkSheet: View {
    @Environment(AppModel.self) private var model
    @State private var sheet: FolderRelinkModel

    init(folderID: String, path: String, model: AppModel) {
        _sheet = State(initialValue: FolderRelinkModel(folderID: folderID, path: path, model: model))
    }

    var body: some View {
        let ready = sheet.ready
        KilnDialog(Strings.FolderPage.relinkTitle(folderName)) {
            KilnLead(Text(Strings.FolderPage.relinkLead))
                .padding(.top, 6)
            KilnField(Strings.ServeSheet.folder) {
                KilnInputBox(sheet.path)
            }
            .padding(.top, 14)
            ForEach(sheet.warningLines, id: \.self) { line in
                KilnNoteRow(line)
            }
            if let problem = sheet.problem {
                KilnFieldNote(Text(problem), error: true)
            }
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.Programs.findIt) {
                Task {
                    if await sheet.confirm() { model.sheet = nil }
                }
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!ready)
            .opacity(ready ? 1 : 0.42)
        }
        .task { await sheet.check() }
    }

    private var folderName: String {
        model.snapshot.folder(sheet.folderID)?.name ?? String()
    }
}
