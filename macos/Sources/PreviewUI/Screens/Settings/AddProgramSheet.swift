import SwiftUI
import PreviewCore

struct AddProgramSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var sheet: ProgramSheetModel

    init(kind: ProgramSheetModel.Kind, path: String = String(), model: AppModel) {
        _sheet = State(initialValue: ProgramSheetModel(kind: kind, model: model, path: path))
    }

    var body: some View {
        let ready = sheet.ready
        KilnDialog(sheet.isAdding ? Strings.AddProgram.title : Strings.AddProgram.relinkTitle) {
            KilnLead(Text(sheet.isAdding ? Strings.AddProgram.lead : Strings.AddProgram.relinkLead))
                .padding(.top, 6)
            KilnField(Strings.AddProgram.file) {
                if sheet.path.isEmpty {
                    KilnInput(text: .constant(String()), placeholder: Strings.Sheets.choose)
                } else {
                    KilnInputBox(sheet.path)
                }
                Button(Strings.Sheets.choose) {
                    Task { _ = await sheet.chooseAnother() }
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
            .padding(.top, 14)
            if let problem = sheet.problem {
                KilnFieldNote(Text(problem), error: true)
            }
            if sheet.isAdding {
                KilnField(Strings.ServeSheet.name) {
                    KilnInput(text: Binding(get: { sheet.name }, set: { sheet.setName($0) }), placeholder: Strings.ServeSheet.name, mono: true)
                }
                .padding(.top, 10)
                if sheet.nameProblem == .taken {
                    KilnFieldNote(Text(Strings.Sheets.taken(sheet.name)), error: true)
                } else {
                    KilnFieldNote(Text(Strings.AddProgram.nameHint), error: sheet.nameProblem == .invalid)
                }
            }
            warnings
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(sheet.isAdding ? Strings.AddProgram.add : Strings.AddProgram.useFile) {
                Task {
                    if await sheet.confirm() { model.sheet = nil }
                }
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!ready)
            .opacity(ready ? 1 : 0.42)
        }
        .task(id: sheet.path) {
            await sheet.check()
        }
    }

    @ViewBuilder private var warnings: some View {
        let lines = sheet.warningLines
        if !lines.isEmpty {
            KilnGroupTitle(sheet.isAdding ? Strings.AddProgram.beforeYouAdd : Strings.AddProgram.beforeYouUse)
            KilnCard(kind: .warn) {
                ForEach(Array(lines.enumerated()), id: \.offset) { index, line in
                    if index > 0 {
                        KilnRowDivider()
                    }
                    KilnRow(title: line, leading: {
                        Image(systemName: Symbols.warning)
                            .font(.system(size: 13))
                            .foregroundStyle(kiln.warnFg)
                    }, trailing: {
                        EmptyView()
                    })
                }
            }
        }
    }
}
