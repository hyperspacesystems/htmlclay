import SwiftUI
import Foundation
import PreviewCore

struct RequestTitleBar: View {
    let position: Int
    let total: Int
    @Environment(\.kiln) private var kiln
    var body: some View {
        ZStack {
            Text(Strings.Request.windowTitle)
                .font(.system(size: 13, weight: .semibold))
                .foregroundStyle(kiln.ink2)
            if let counter = RequestQueue.counter(position: position, total: total) {
                HStack(spacing: 0) {
                    Spacer()
                    Text(counter)
                        .font(.system(size: 12, weight: .medium))
                        .monospacedDigit()
                        .foregroundStyle(kiln.muted)
                        .padding(.trailing, 12)
                }
            }
        }
        .frame(maxWidth: .infinity)
        .frame(height: 28)
    }
}

struct RequestBody: View {
    let request: ProgramRequest
    let item: WaitingItem
    let position: Int
    let total: Int
    let actions: RequestActions
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var expanded = false
    @State private var chosen = String()

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            RequestTitleBar(position: position, total: total)
            VStack(alignment: .leading, spacing: 0) {
                variant
            }
            .padding(.top, 10)
            .padding(.horizontal, 22)
            .padding(.bottom, 18)
        }
        .frame(width: 460)
    }

    @ViewBuilder private var variant: some View {
        switch request.kind {
        case .found: foundContent
        case .noFile: noFileContent
        case .teamDocument: teamDocumentContent
        case .riskyFile: riskyFileContent
        case .folderRead: folderReadContent
        case .folderReadAgain: folderReadAgainContent
        }
    }

    private var foundContent: some View {
        VStack(alignment: .leading, spacing: 0) {
            titleLine(Strings.Request.title(request.document))
            leadLine(Strings.Request.callsIt(request.program))
            card(warn: false) {
                RequestFileRow(color: .amber, symbol: Symbols.program, title: fileName, path: request.path, extra: Strings.Request.foundHere, warn: false)
            }
            noteLine(Strings.Request.runsAsYou)
            moreOptions
                .padding(.top, 12)
            buttonsRow {
                if let deny = choice(RequestAnswer.deny) { button(deny) }
                if let allow = choice(RequestAnswer.allowDocument) { button(allow, sending: chosenAnswer) }
            }
        }
    }

    private var noFileContent: some View {
        VStack(alignment: .leading, spacing: 0) {
            titleLine(Strings.Request.runNamed(document: request.document, program: request.program))
            leadLine(Strings.Request.noFileLead)
            buttonsRow {
                if let deny = choice(RequestAnswer.deny) { button(deny) }
                if let pick = choice(RequestAnswer.chooseFile) { button(pick) }
            }
        }
    }

    private var teamDocumentContent: some View {
        VStack(alignment: .leading, spacing: 0) {
            titleLine(Self.teamTitle(request))
            leadLine(Self.teamLead(request))
            if !Self.isAIEditing(request) {
                card(warn: false) {
                    RequestFileRow(color: .amber, symbol: Symbols.program, title: fileName, path: request.path, extra: nil, warn: false)
                }
            }
            noteLine(Strings.Request.teamAsksEachTime)
            buttonsRow {
                if let deny = choice(RequestAnswer.deny) { button(deny) }
                if let allow = choice(RequestAnswer.allowDocument) { button(allow) }
            }
        }
    }

    private var riskyFileContent: some View {
        VStack(alignment: .leading, spacing: 0) {
            titleLine(Strings.Request.useThisFile)
            card(warn: true) {
                RequestFileRow(color: .amber, symbol: Symbols.program, title: fileName, path: request.path,
                              extra: Strings.Request.detailText(request.detail), warn: true)
            }
            noteLine(Strings.Request.riskyNote)
            buttonsRow {
                if let pick = choice(RequestAnswer.chooseFile) { button(pick) }
                if let anyway = choice(RequestAnswer.allowAnyway) { button(anyway) }
            }
        }
    }

    private var folderReadContent: some View {
        VStack(alignment: .leading, spacing: 0) {
            titleLine(Strings.Request.readFolder(request.document))
            card(warn: false) {
                RequestFileRow(color: .sky, symbol: Symbols.folder, title: request.folder, path: request.path, extra: nil, warn: false)
            }
            noteLine(Strings.Request.readNote(request.folder))
            buttonsRow {
                if let deny = choice(RequestAnswer.deny) { button(deny) }
                if let once = choice(RequestAnswer.allowOnce) { button(once) }
                if let trust = choice(RequestAnswer.trustFolder) { button(trust) }
            }
        }
    }

    private var folderReadAgainContent: some View {
        VStack(alignment: .leading, spacing: 0) {
            titleLine(Strings.Request.trustOwnFolder(request.document))
            card(warn: false) {
                RequestFileRow(color: .sky, symbol: Symbols.folder, title: request.folder,
                              path: Strings.Request.folderLine(path: request.path, detail: request.detail),
                              extra: nil, warn: false)
            }
            noteLine(Strings.Request.ownFolderNote)
            buttonsRow {
                if let deny = choice(RequestAnswer.deny) { button(deny) }
                if let trust = choice(RequestAnswer.trustFolder) { button(trust) }
            }
        }
    }

    private var moreOptions: some View {
        VStack(alignment: .leading, spacing: 0) {
            if !options.isEmpty || chooseAnother {
                Button {
                    expanded.toggle()
                } label: {
                    HStack(spacing: 5) {
                        Image(systemName: expanded ? Symbols.chevronDown : Symbols.chevronRight)
                            .font(.system(size: 10, weight: .semibold))
                            .foregroundStyle(kiln.ink2)
                        Text(Strings.Request.moreOptions)
                            .font(.system(size: 13, weight: .medium))
                            .foregroundStyle(kiln.ink2)
                    }
                }
                .buttonStyle(.plain)
            }
            if expanded {
                KilnCard {
                    if let only = option(RequestAnswer.allowDocument) {
                        choiceRow(on: chosenAnswer == only.answer, title: only.title) { chosen = only.answer }
                    }
                    if let trusted = option(RequestAnswer.allowAllTrusted) {
                        KilnRowDivider()
                        choiceRow(on: chosenAnswer == trusted.answer, title: trusted.title) { chosen = trusted.answer }
                    }
                    if chooseAnother {
                        KilnRowDivider()
                        HStack(spacing: 0) {
                            KilnLink(Strings.ProgramPage.chooseAnotherFile) { chooseFile() }
                            Spacer()
                        }
                        .padding(.horizontal, 14)
                        .padding(.vertical, 10)
                    }
                }
                .padding(.top, 8)
            }
        }
    }

    private func choiceRow(on: Bool, title: String, choose: @escaping () -> Void) -> some View {
        Button(action: choose) {
            HStack(spacing: 11) {
                KilnRadio(on: on)
                Text(title)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(kiln.ink)
                Spacer()
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 10)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }

    private func titleLine(_ text: String) -> some View {
        Text(text)
            .font(.system(size: 16, weight: .bold))
            .foregroundStyle(kiln.ink)
            .fixedSize(horizontal: false, vertical: true)
    }

    private func leadLine(_ text: String) -> some View {
        Text(text)
            .font(.system(size: 13.5))
            .foregroundStyle(kiln.ink2)
            .fixedSize(horizontal: false, vertical: true)
            .padding(.top, 6)
    }

    private func noteLine(_ text: String) -> some View {
        Text(text)
            .font(.system(size: 13))
            .foregroundStyle(kiln.muted)
            .fixedSize(horizontal: false, vertical: true)
            .padding(.top, 12)
    }

    private func card<C: View>(warn: Bool, @ViewBuilder content: () -> C) -> some View {
        KilnCard(kind: warn ? .warn : .plain) { content() }
            .padding(.top, 14)
    }

    private func buttonsRow<C: View>(@ViewBuilder content: () -> C) -> some View {
        HStack(spacing: 8) {
            Spacer()
            content()
        }
        .padding(.top, 18)
    }

    private var fileName: String { RequestBody.fileName(request) }

    private var isSample: Bool { model.isSample }

    private var options: [String] { RequestControls.options(request, isSample: isSample) }

    private var chooseAnother: Bool {
        RequestControls.offers(RequestAnswer.chooseFile, in: request, isSample: isSample)
    }

    /// The answer the Allow button sends: the chosen option, or one document alone.
    private var chosenAnswer: String {
        options.contains(chosen) ? chosen : RequestAnswer.allowDocument
    }

    private func choice(_ answer: String) -> RequestChoice? {
        RequestControls.choice(answer, in: request, isSample: isSample)
    }

    private func option(_ answer: String) -> RequestChoice? {
        guard options.contains(answer) else { return nil }
        return RequestChoice(answer: answer, title: RequestControls.optionTitle(answer, in: request))
    }

    private func button(_ choice: RequestChoice, sending answer: String? = nil) -> some View {
        Button(choice.title) {
            if choice.answer == RequestAnswer.chooseFile {
                chooseFile()
            } else {
                send(answer ?? choice.answer)
            }
        }
        .buttonStyle(choice.look == .large ? KilnButtonStyle(.large) : KilnButtonStyle(.plain))
    }

    private func send(_ answer: String, path: String = String()) {
        actions.answer(item, request, answer, path)
    }

    private func chooseFile() {
        Task {
            guard let path = await model.folderChooser?.chooseProgram(), !path.isEmpty else { return }
            send(RequestAnswer.chooseFile, path: path)
        }
    }

    static func fileName(_ request: ProgramRequest) -> String {
        RequestControls.fileName(request)
    }

    static func isAIEditing(_ request: ProgramRequest) -> Bool { request.program == ProgramControls.aiEditID }

    static func teamTitle(_ request: ProgramRequest) -> String {
        isAIEditing(request)
            ? Strings.Request.useAIEditing(document: request.document)
            : Strings.Request.runNamed(document: request.document, program: request.program)
    }

    static func teamLead(_ request: ProgramRequest) -> String {
        isAIEditing(request) ? Strings.Request.aiEditTeamLead(request.folder) : Strings.Request.fromTeam(request.folder)
    }
}

private struct RequestFileRow: View {
    let color: SquircleColor
    let symbol: String
    let title: String
    let path: String
    let extra: String?
    let warn: Bool
    @Environment(\.kiln) private var kiln
    var body: some View {
        HStack(spacing: 11) {
            Squircle(color, symbol: symbol, size: .large)
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(warn ? kiln.warnFg : kiln.ink)
                if !path.isEmpty {
                    Text(path)
                        .font(.system(size: 12.5, weight: .regular, design: .monospaced))
                        .foregroundStyle(pathColor)
                }
                if let extra {
                    Text(extra)
                        .font(.system(size: 13))
                        .foregroundStyle(pathColor)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
    }
    private var pathColor: Color { warn ? kiln.warnFg.opacity(0.88) : kiln.muted }
}
