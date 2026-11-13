import SwiftUI
import PreviewCore

struct MoveSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var checked = Set(SampleData.localMoveFolders.map(\.id))
    @State private var localRunning = true
    @State private var unsaved = true
    @State private var key = String()

    var body: some View {
        let blockers = (localRunning ? 1 : 0) + (unsaved ? 1 : 0)
        let ready = MoveRules.canMove(blockers: blockers, checked: checked, key: key)
        KilnDialog(Strings.Move.title, titleSize: 17, width: 520) {
            KilnCard {
                ForEach(Array(SampleData.localMoveFolders.enumerated()), id: \.element.id) { index, folder in
                    if index > 0 {
                        KilnRowDivider()
                    }
                    folderRow(folder)
                }
            }
            .padding(.top, 12)
            if blockers > 0 {
                KilnGroupTitle(Strings.Move.beforeYouMove)
                KilnCard(kind: .warn) {
                    if localRunning {
                        KilnRow(title: Strings.Move.localRunning) {
                            Button(Strings.Move.quitLocal) {
                                model.say(Strings.Move.wouldQuitLocal)
                                localRunning = false
                            }
                            .buttonStyle(KilnButtonStyle(.plain))
                        }
                    }
                    if localRunning && unsaved {
                        KilnRowDivider()
                    }
                    if unsaved {
                        KilnRow(title: Strings.Move.unsaved(SampleData.localUnsavedDocument)) {
                            Button(Strings.Move.showIt) {
                                model.say(Strings.Move.wouldShowUnsaved(SampleData.localUnsavedDocument))
                                unsaved = false
                            }
                            .buttonStyle(KilnButtonStyle(.plain))
                        }
                    }
                }
            }
            KilnGroupTitle(Strings.Move.comesAlong)
            Text(Strings.Move.comesAlongText(conflicts: SampleData.conflictsComingAlong))
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
                .lineSpacing(2)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, 3)
            KilnField(Strings.Connect.syncKey) {
                KilnInput(text: $key, placeholder: Strings.Connect.keyPlaceholder, mono: true)
                Button(Strings.Connect.paste) {
                    key = SampleData.syncKey
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
            .padding(.top, 14)
        } buttons: {
            Button(Strings.Move.notNow) {
                model.sheet = nil
            }
            .buttonStyle(KilnButtonStyle(.plain))
            Button(Strings.Move.moveFolders) {
                model.send(.moveFromLocal(folderIDs: MoveRules.ordered(checked)))
                model.sheet = .movedFromLocal
            }
            .buttonStyle(KilnButtonStyle(.primary))
            .disabled(!ready)
            .opacity(ready ? 1 : 0.42)
        }
    }

    private func folderRow(_ folder: LocalMoveFolder) -> some View {
        Button {
            toggle(folder.id)
        } label: {
            HStack(spacing: 11) {
                KilnCheck(on: checked.contains(folder.id))
                VStack(alignment: .leading, spacing: 2) {
                    Text(folder.name)
                        .font(.system(size: 14, weight: .medium))
                        .foregroundStyle(kiln.ink)
                    Text(Strings.Move.mapping(port: folder.port, address: folder.address))
                        .font(.system(size: 12.5, design: .monospaced))
                        .foregroundStyle(kiln.muted)
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
        if checked.contains(id) {
            checked.remove(id)
        } else {
            checked.insert(id)
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
