import SwiftUI
import PreviewCore

struct ProgramsPane: View {
    let actions: SettingsActions
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln

    static func waitingRequests(_ snapshot: HostSnapshot) -> [WaitingItem] {
        snapshot.waiting.filter { $0.kind == .programRequest }
    }

    static func waitingTitle(_ item: WaitingItem, snapshot: HostSnapshot) -> String {
        guard let request = snapshot.requests.first(where: { $0.id == item.subjectID }), !request.program.isEmpty else { return item.title }
        if request.program == ProgramControls.aiEditID { return Strings.Programs.aiEditWaiting(document: request.document) }
        return Strings.Programs.wants(document: request.document, program: request.program)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header
            KilnCard {
                KilnNoteRow(Strings.Programs.intro)
            }
            waitingGroup
            programsGroup
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var header: some View {
        HStack(spacing: 10) {
            Text(Strings.Settings.title(.programs))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
            Spacer(minLength: 0)
            if model.isSample || model.can(HostActionType.programAdd) {
                Button(Strings.Programs.addProgram) {
                    model.sheet = .addProgram
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
        }
        .padding(.bottom, 12)
    }

    @ViewBuilder private var waitingGroup: some View {
        let items = Self.waitingRequests(model.snapshot)
        if !items.isEmpty {
            KilnGroupTitle(Strings.Programs.waitingForYou)
            KilnCard(kind: .warn) {
                ForEach(Array(items.enumerated()), id: \.element.id) { index, item in
                    if index > 0 {
                        KilnRowDivider()
                    }
                    KilnRow(title: Self.waitingTitle(item, snapshot: model.snapshot), leading: {
                        Image(systemName: Symbols.program)
                            .font(.system(size: 13, weight: .medium))
                            .foregroundStyle(kiln.warnFg)
                    }, trailing: {
                        Button(Strings.Programs.answer) {
                            actions.answer(item)
                        }
                        .buttonStyle(KilnButtonStyle(.primary))
                    })
                }
            }
        }
    }

    @ViewBuilder private var programsGroup: some View {
        let programs = model.snapshot.programs
        if !programs.isEmpty {
            KilnGroupTitle(Strings.Programs.programs)
            KilnCard {
                ForEach(Array(programs.enumerated()), id: \.element.id) { index, program in
                    if index > 0 {
                        KilnRowDivider()
                    }
                    ProgramRow(program: program)
                }
            }
        }
    }
}

struct ProgramRow: View {
    let program: Program
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln

    static func fileName(_ program: Program) -> String {
        ProgramControls.fileName(program)
    }

    static func subtitle(_ program: Program) -> String {
        if ProgramControls.isAIEditing(program) {
            return program.allowedDocuments.count == 1
                ? Strings.Programs.aiEditAllowedFor(ProgramControls.documentLabel(program.allowedDocuments[0]))
                : Strings.Programs.aiEditAllowedCount(program.allowedDocuments.count)
        }
        if program.allTrustedFolders { return Strings.Programs.allTrusted }
        switch program.allowedDocuments.count {
        case 0: return Strings.Programs.noneYet
        case 1: return Strings.Programs.mayRun(ProgramControls.documentLabel(program.allowedDocuments[0]))
        default: return Strings.Programs.mayRunCount(program.allowedDocuments.count)
        }
    }

    var body: some View {
        if program.fileMissing {
            missingRow
        } else {
            Button {
                model.route = .program(program.id)
            } label: {
                KilnRow(title: Self.fileName(program), subtitle: Self.subtitle(program), leading: {
                    Squircle(.amber, symbol: Symbols.program)
                }, trailing: {
                    Image(systemName: Symbols.chevronRight)
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundStyle(kiln.faint)
                })
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
        }
    }

    private var missingRow: some View {
        HStack(spacing: 11) {
            Squircle(.red, symbol: Symbols.program)
            VStack(alignment: .leading, spacing: 0) {
                Text(Self.fileName(program))
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(kiln.ink)
                KilnPill(.err, text: Strings.Programs.fileMissing(program.allowedDocuments.count))
                    .padding(.top, 4)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            if model.isSample || model.can(HostActionType.programRelink) {
                Button(Strings.Programs.findIt) {
                    if model.isSample {
                        model.say(Strings.Programs.wouldFind(Self.fileName(program)))
                    } else {
                        ProgramFlow.chooseFile(program, model: model)
                    }
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }
}
