import SwiftUI
import PreviewCore

public struct CustomCommandSheet: View {
    @Environment(AppModel.self) private var model
    @State private var sheet: CustomCommandModel

    public init(model: AppModel) {
        _sheet = State(initialValue: CustomCommandModel(model: model))
    }

    public var body: some View {
        KilnDialog(Strings.General.aiCustomSheetTitle) {
            KilnLead(Text(Strings.General.aiCustomLead))
                .padding(.top, 6)
            KilnField(Strings.General.aiCommand) {
                KilnInput(text: Binding(
                    get: { sheet.command },
                    set: { sheet.setCommand($0) }
                ), placeholder: Strings.General.aiCommandPlaceholder, mono: true)
            }
            .padding(.top, 14)
            KilnFieldNote(Text(Strings.General.aiCommandNote))
            if let problem = sheet.problem {
                KilnFieldNote(Text(problem), error: true)
            }
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.General.aiSaveCommand) {
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
