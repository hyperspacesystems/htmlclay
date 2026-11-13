import SwiftUI
import PreviewCore

struct AddProgramSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var pick = 0
    @State private var name = ProgramRules.defaultName(for: SampleData.programFiles[0])

    var body: some View {
        let path = SampleData.programFiles[pick % SampleData.programFiles.count]
        let problem = ProgramRules.nameProblem(name, in: model.snapshot)
        KilnDialog(Strings.AddProgram.title) {
            KilnLead(Text(Strings.AddProgram.lead))
                .padding(.top, 6)
            KilnField(Strings.AddProgram.file) {
                KilnInputBox(path)
                Button(Strings.Sheets.choose) {
                    pick += 1
                    name = ProgramRules.defaultName(for: SampleData.programFiles[pick % SampleData.programFiles.count])
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
            .padding(.top, 14)
            KilnField(Strings.ServeSheet.name) {
                KilnInput(text: $name, placeholder: Strings.ServeSheet.name, mono: true)
            }
            .padding(.top, 10)
            if problem == .taken {
                KilnFieldNote(Text(Strings.Sheets.taken(name)), error: true)
            } else {
                KilnFieldNote(Text(Strings.AddProgram.nameHint), error: problem == .invalid)
            }
        } buttons: {
            Button(Strings.Sheets.cancel) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.AddProgram.add) {
                model.send(.addProgram(name: name, path: path))
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(problem != nil)
            .opacity(problem == nil ? 1 : 0.42)
        }
    }
}
