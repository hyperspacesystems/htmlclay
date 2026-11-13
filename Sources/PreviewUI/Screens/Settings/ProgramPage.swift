import SwiftUI
import PreviewCore

struct ProgramPage: View {
    let programID: String
    let actions: SettingsActions
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln

    static func pathLine(_ program: Program) -> String {
        Strings.ProgramPage.pathLine(path: program.path, ready: !program.fileMissing, name: program.name)
    }

    static func place(of document: String, snapshot: HostSnapshot) -> String? {
        snapshot.documents.first { $0.name == document }?.place
    }

    var body: some View {
        if let program = model.snapshot.program(programID) {
            page(program)
        } else {
            ProgramsPane(actions: actions)
        }
    }

    private func page(_ program: Program) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            header(program)
            KilnPath(Self.pathLine(program))
            KilnCard {
                KilnRow(title: Strings.ProgramPage.runsAsYou, leading: {
                    Image(systemName: Symbols.warning)
                        .font(.system(size: 13))
                        .foregroundStyle(KilnFixed.statusAmber)
                }, trailing: {
                    EmptyView()
                })
            }
            KilnGroupTitle(Strings.ProgramPage.whoMayRun)
            whoMayRunCard(program)
            removedGroup(program)
            footer(program)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func header(_ program: Program) -> some View {
        HStack(spacing: 10) {
            KilnBackButton {
                model.route = .pane(.programs)
            }
            Text(ProgramRow.fileName(program))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
            Spacer(minLength: 0)
            Button(Strings.ProgramPage.showInFinder) {
                model.say(Strings.Preview.wouldShowInFinder(program.path))
            }
            .buttonStyle(KilnButtonStyle(.plain))
        }
        .padding(.bottom, 12)
    }

    private func whoMayRunCard(_ program: Program) -> some View {
        KilnCard {
            KilnRow(title: Strings.ProgramPage.allTrustedTitle, subtitle: Strings.ProgramPage.allTrustedSub, trailing: {
                Toggle(isOn: Binding(
                    get: { program.allTrustedFolders },
                    set: { model.send(.setProgramAllTrusted(programID: program.id, on: $0)) }
                )) {
                    EmptyView()
                }
                .toggleStyle(KilnSwitchStyle())
            })
            ForEach(program.allowedDocuments, id: \.self) { document in
                KilnRowDivider()
                KilnRow(title: document, subtitle: Self.place(of: document, snapshot: model.snapshot), leading: {
                    Image(systemName: Symbols.doc)
                        .font(.system(size: 14))
                        .foregroundStyle(kiln.muted)
                }, trailing: {
                    Button(Strings.ProgramPage.removeAccess) {
                        model.send(.removeProgramAccess(programID: program.id, document: document))
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                })
            }
        }
    }

    @ViewBuilder private func removedGroup(_ program: Program) -> some View {
        if !program.removedDocuments.isEmpty {
            KilnGroupTitle(Strings.ProgramPage.accessRemoved)
            KilnCard {
                ForEach(Array(program.removedDocuments.enumerated()), id: \.element) { index, document in
                    if index > 0 {
                        KilnRowDivider()
                    }
                    KilnRow(title: document, subtitle: Self.place(of: document, snapshot: model.snapshot), leading: {
                        Image(systemName: Symbols.doc)
                            .font(.system(size: 14))
                            .foregroundStyle(kiln.muted)
                    }, trailing: {
                        Button(Strings.ProgramPage.allowAgain) {
                            model.say(Strings.ProgramPage.wouldAllowAgain(document))
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                    })
                }
            }
        }
    }

    private func footer(_ program: Program) -> some View {
        HStack {
            KilnLink(Strings.ProgramPage.chooseAnotherFile) {
                model.say(Strings.ProgramPage.wouldChooseFile)
            }
            Spacer(minLength: 0)
            KilnLink(Strings.ProgramPage.removeAllAccess, danger: true) {
                removeAllAccess(program)
            }
        }
        .padding(.top, 14)
    }

    private func removeAllAccess(_ program: Program) {
        for document in program.allowedDocuments {
            model.send(.removeProgramAccess(programID: program.id, document: document))
        }
        if program.allTrustedFolders {
            model.send(.setProgramAllTrusted(programID: program.id, on: false))
        }
    }
}
