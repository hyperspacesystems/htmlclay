import SwiftUI
import PreviewCore

struct MoveSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var unchecked = Set<String>()
    @State private var key = String()
    @State private var moving = false

    var body: some View {
        let folders = model.snapshot.moveFolders
        let blockers = model.snapshot.moveBlockers
        let checked = Set(folders.map(\.id)).subtracting(unchecked)
        let ready = !moving && MoveRules.canMove(blockers: blockers, checked: checked, key: key)
        KilnDialog(Strings.Move.title, titleSize: 17, width: 520) {
            KilnCard {
                ForEach(Array(folders.enumerated()), id: \.element.id) { index, folder in
                    if index > 0 {
                        KilnRowDivider()
                    }
                    folderRow(folder, on: checked.contains(folder.id))
                }
            }
            .padding(.top, 12)
            if blockers.localRunning {
                KilnGroupTitle(Strings.Move.beforeYouMove)
                KilnCard(kind: .warn) {
                    KilnRow(title: Strings.Move.localRunning) {
                        Button(Strings.Move.quitLocal) {
                            model.send(.quitLocal)
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                    }
                }
            }
            KilnGroupTitle(Strings.Move.comesAlong)
            Text(model.snapshot.isSample ? Strings.Move.comesAlongText(conflicts: SampleData.conflictsComingAlong) : Strings.Move.comesAlongLive)
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
                .lineSpacing(2)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, 3)
            if blockers.needsKey {
                KilnField(Strings.Connect.syncKey) {
                    KilnInput(text: $key, placeholder: Strings.Connect.keyPlaceholder, mono: true)
                    Button(Strings.Connect.paste) {
                        key = NSPasteboard.general.string(forType: .string) ?? String()
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
                .padding(.top, 14)
            }
        } buttons: {
            Button(Strings.Move.notNow) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.Move.moveFolders) {
                move(MoveRules.ordered(folders, checked: checked), connectFirst: blockers.needsKey)
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!ready)
            .opacity(ready ? 1 : 0.42)
        }
    }

    /// Connects with the key first when one is needed, then moves. A refusal
    /// keeps the sheet open and says why.
    @MainActor
    private func move(_ folderIDs: [String], connectFirst: Bool) {
        let sent = key
        key = String()
        moving = true
        Task {
            defer { moving = false }
            if connectFirst, case let .refused(answer) = await model.submit(.connect(username: String(), key: sent)) {
                model.say(answer.text.isEmpty ? answer.code : answer.text)
                return
            }
            if case let .refused(answer) = await model.submit(.moveFromLocal(folderIDs: folderIDs)) {
                model.say(answer.text.isEmpty ? answer.code : answer.text)
                return
            }
            model.sheet = .movedFromLocal
        }
    }

    private func folderRow(_ folder: MoveFolderInfo, on: Bool) -> some View {
        Button {
            toggle(folder.id)
        } label: {
            HStack(spacing: 11) {
                KilnCheck(on: on)
                VStack(alignment: .leading, spacing: 2) {
                    Text(folder.name)
                        .font(.system(size: 14, weight: .medium))
                        .foregroundStyle(kiln.ink)
                    Text(Strings.Move.mapping(port: folder.port, address: folder.address))
                        .font(.system(size: 12.5, design: .monospaced))
                        .foregroundStyle(kiln.muted)
                    if !folder.skip.isEmpty {
                        Text(Strings.Move.stayed(folder.skip))
                            .font(.system(size: 12.5))
                            .foregroundStyle(kiln.ink2)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                Spacer(minLength: 0)
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 10)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }

    private func toggle(_ id: String) {
        if unchecked.contains(id) {
            unchecked.remove(id)
        } else {
            unchecked.insert(id)
        }
    }
}

struct MovedSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    let actions: SettingsActions

    var body: some View {
        let conflicts = model.snapshot.waiting.filter { $0.kind == .conflict }.count
        KilnDialog(Strings.Move.movedTitle, titleSize: 17) {
            KilnCard {
                KilnRow(title: Strings.Move.switchesTitle, subtitle: Strings.Move.switchesWhere) {
                    EmptyView()
                }
                KilnRowDivider()
                KilnRow(title: Strings.Move.noticesTitle, subtitle: Strings.Move.noticesWhere) {
                    EmptyView()
                }
                KilnRowDivider()
                KilnRow(title: Strings.Move.activityTitle, subtitle: Strings.Move.activityWhere) {
                    EmptyView()
                }
                KilnRowDivider()
                KilnRow(title: Strings.Move.workspaceTitle, subtitle: Strings.Move.workspaceWhere) {
                    EmptyView()
                }
            }
            .padding(.top, 12)
            if conflicts > 0 {
                Text(Strings.Move.conflictsWaiting(conflicts))
                    .font(.system(size: 13.5))
                    .foregroundStyle(kiln.ink2)
                    .padding(.top, 12)
            }
        } buttons: {
            Button(Strings.Welcome.openSettings) {
                model.sheet = nil
                model.route = .pane(.folders)
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.Panel.openDashboard) {
                model.sheet = nil
                actions.openDashboard()
            }
            .buttonStyle(KilnButtonStyle(.primary))
        }
    }
}
