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

    /// The file rows, the runs-as-you card and Show in Finder belong to a
    /// program a person added; a built-in one has none of them.
    static func showsFileControls(_ program: Program) -> Bool {
        !ProgramControls.isAIEditing(program) && !ProgramControls.isBuiltinProgram(program)
    }

    static func title(_ program: Program) -> String {
        ProgramControls.isBuiltinProgram(program)
            ? ProgramControls.programTitle(program.id)
            : ProgramControls.fileName(program)
    }

    static func place(of document: String, snapshot: HostSnapshot) -> String? {
        let name = ProgramControls.documentLabel(document)
        return snapshot.documents.first { $0.name == name }?.place
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
            if Self.showsFileControls(program) {
                KilnPath(Self.pathLine(program))
            }
            if ProgramControls.isBuiltinProgram(program) {
                builtinCard(program)
            } else {
                KilnCard {
                    KilnRow(title: Self.showsFileControls(program) ? Strings.ProgramPage.runsAsYou : Strings.ProgramPage.aiEditNote, leading: {
                        if Self.showsFileControls(program) {
                            Image(systemName: Symbols.warning)
                                .font(.system(size: 13))
                                .foregroundStyle(KilnFixed.statusAmber)
                        }
                    }, trailing: {
                        EmptyView()
                    })
                }
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
            Text(Self.title(program))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
            Spacer(minLength: 0)
            if Self.showsFileControls(program), model.isSample || model.can(HostActionType.programReveal) {
                Button(Strings.ProgramPage.showInFinder) {
                    if model.isSample {
                        model.say(Strings.Preview.wouldShowInFinder(program.path))
                    } else {
                        model.send(.revealProgram(programID: program.id))
                    }
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
        }
        .padding(.bottom, 12)
    }

    @ViewBuilder private func builtinCard(_ program: Program) -> some View {
        KilnCard {
            KilnRow(title: Strings.Programs.enableBuiltin(ProgramControls.programTitle(program.id)),
                    subtitle: ProgramControls.builtinEnableSub(program.id),
                    leading: {
                        Squircle(BuiltinProgramRow.color(program.id), symbol: BuiltinProgramRow.symbol(program.id))
                    }) {
                Toggle(isOn: Binding(
                    get: { program.isOn },
                    set: { model.send(.setProgramOn(programID: program.id, on: $0)) }
                )) {
                    EmptyView()
                }
                .toggleStyle(KilnSwitchStyle())
            }
            KilnRowDivider()
            KilnRow(title: Strings.Programs.example(ProgramControls.exampleName(program.id)),
                    subtitle: ProgramControls.exampleSub(program.id),
                    leading: {
                        Image(systemName: Symbols.doc)
                            .font(.system(size: 14))
                            .foregroundStyle(kiln.muted)
                    }) {
                Button(Strings.Programs.openExample) {
                    model.send(.openProgramExample(programID: program.id))
                }
                .buttonStyle(KilnButtonStyle(.plain))
                .disabled(!program.isOn)
            }
        }
    }

    private func whoMayRunCard(_ program: Program) -> some View {
        KilnCard {
            if Self.showsFileControls(program) || ProgramControls.isBuiltinProgram(program) {
                KilnRow(title: Strings.ProgramPage.allTrustedTitle, subtitle: Strings.ProgramPage.allTrustedSub, trailing: {
                    Toggle(isOn: Binding(
                        get: { ProgramControls.allTrusted(program, pending: model.pending(AppModel.allTrustedSlot(program.id))) },
                        set: { model.send(.setProgramAllTrusted(programID: program.id, on: $0)) }
                    )) {
                        EmptyView()
                    }
                    .toggleStyle(KilnSwitchStyle())
                    .disabled(!model.can(HostActionType.programSetAllTrusted))
                })
            }
            if program.allowedDocuments.isEmpty, ProgramControls.isBuiltinProgram(program) {
                KilnRowDivider()
                KilnNoteRow(Strings.Programs.noPermissionYet)
            }
            ForEach(program.allowedDocuments, id: \.self) { document in
                KilnRowDivider()
                KilnRow(title: ProgramControls.documentLabel(document), subtitle: Self.place(of: document, snapshot: model.snapshot), leading: {
                    Image(systemName: Symbols.doc)
                        .font(.system(size: 14))
                        .foregroundStyle(kiln.muted)
                }, trailing: {
                    if model.can(HostActionType.programRemoveAccess) {
                        Button(Strings.ProgramPage.removeAccess) {
                            model.send(.removeProgramAccess(programID: program.id, document: document))
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                    }
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
                    KilnRow(title: ProgramControls.documentLabel(document), subtitle: Self.place(of: document, snapshot: model.snapshot), leading: {
                        Image(systemName: Symbols.doc)
                            .font(.system(size: 14))
                            .foregroundStyle(kiln.muted)
                    }, trailing: {
                        if model.can(HostActionType.programAskAgain) {
                            Button(Strings.ProgramPage.allowAgain) {
                                allowAgain(document, program: program)
                            }
                            .buttonStyle(KilnButtonStyle(.plain))
                        }
                    })
                }
            }
        }
    }

    private func footer(_ program: Program) -> some View {
        HStack {
            if Self.showsFileControls(program), model.isSample || model.can(HostActionType.programRelink) {
                KilnLink(Strings.ProgramPage.chooseAnotherFile) {
                    ProgramFlow.chooseFile(program, model: model)
                }
            }
            Spacer(minLength: 0)
            if model.can(HostActionType.programRemoveAccess) {
                KilnLink(Strings.ProgramPage.removeAllAccess, danger: true) {
                    Task { _ = await ProgramFlow.removeAllAccess(program, model: model) }
                }
            }
        }
        .padding(.top, 14)
    }

    private func allowAgain(_ document: String, program: Program) {
        Task { _ = await ProgramFlow.allowAgain(document, program: program, model: model) }
    }
}
