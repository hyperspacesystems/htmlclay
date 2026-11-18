import SwiftUI
import PreviewCore

public struct ChangePortSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var sheet: ChangePortModel

    public init(model: AppModel) {
        _sheet = State(initialValue: ChangePortModel(model: model))
    }

    public var body: some View {
        KilnDialog(Strings.Advanced.changePortTitle) {
            KilnLead(Text(Strings.Advanced.portMoveBody(sheet.currentPort)))
                .padding(.top, 6)
                .padding(.bottom, 14)
            KilnField(Strings.Advanced.changePortField) {
                KilnInput(text: Binding(
                    get: { sheet.text },
                    set: { sheet.setText($0) }
                ), placeholder: String(sheet.currentPort), mono: true)
            }
            KilnFieldNote(Text(Strings.Advanced.portCurrent(sheet.currentPort)))
            if let problem = sheet.problem {
                KilnFieldNote(Text(problem), error: true)
                if !sheet.documents.isEmpty {
                    KilnFieldNote(Text(Strings.Advanced.openDocuments(sheet.documents)), error: true)
                }
            }
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(sheet.actionTitle) {
                Task {
                    if await sheet.save() { model.sheet = nil }
                }
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!sheet.ready)
        }
        .onExitCommand { model.sheet = nil }
    }
}
