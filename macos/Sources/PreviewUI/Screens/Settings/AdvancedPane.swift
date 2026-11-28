import SwiftUI
import PreviewCore

struct AdvancedPane: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    @State private var confirmingClear = false
    @State private var confirmingOff = false
    @State private var limitDraft: String?
    @State private var limitProblem: String?
    @FocusState private var limitFocused: Bool

    // What the person just chose, until the host's next snapshot confirms it.
    private var offlineOn: Bool {
        if case let .setOfflineCopies(on)? = model.pending("prefs.offlineCopies") { return on }
        return model.snapshot.preferences.offlineCopies
    }

    private var offlinePolicy: OfflinePolicy {
        if case let .setOfflinePolicy(policy)? = model.pending("prefs.offlinePolicy") { return policy }
        return model.snapshot.preferences.offlinePolicy
    }

    private var offlineAvailable: Bool {
        model.isSample || model.snapshot.offline.available
    }

    private var controlsDisabled: Bool {
        !offlineAvailable || (!model.isSample && !model.can(HostActionType.prefsSet))
    }

    private var displayedLimitText: String {
        limitDraft ?? OfflinePolicy.formatLimitMB(offlinePolicy.limitMB)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.Settings.title(.advanced))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            let service = SettingsControls.serviceStatus(model.snapshot.host.service)
            KilnCard {
                KilnRow(title: Strings.Advanced.port, subtitle: Strings.Advanced.portSub) {
                    HStack(spacing: 10) {
                        Text(SettingsControls.port(isSample: model.isSample, port: model.snapshot.host.port))
                            .font(.system(size: 13, design: .monospaced))
                            .foregroundStyle(kiln.ink2)
                        Button(Strings.Advanced.changePort) {
                            model.sheet = .changePort
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                        .disabled(!model.isSample && !model.can(HostActionType.setPort))
                    }
                }
                KilnRowDivider()
                KilnRow(title: Strings.Advanced.liveService, subtitle: Strings.Advanced.liveServiceSub) {
                    HStack(spacing: 6) {
                        StatusDot(model.isSample ? .green : service.dot)
                        Text(model.isSample ? Strings.Advanced.running : service.text)
                            .font(.system(size: 13))
                            .foregroundStyle(kiln.ink2)
                        if SettingsControls.offersServiceRestart(model.snapshot.host.service,
                                                                 hostCan: model.can(HostActionType.serviceRestart),
                                                                 isSample: model.isSample) {
                            Button(Strings.Panel.restart) { model.send(.restartService) }
                                .buttonStyle(KilnButtonStyle(.plain))
                        }
                    }
                }
            }
            offlineGroup
            if SettingsControls.showsStorageTotals(isSample: model.isSample) {
                KilnGroupTitle(Strings.Advanced.storage)
                KilnCard {
                    KilnRow(title: Strings.Advanced.versions) {
                        folderTrailing(Strings.Advanced.versionsSize, note: Strings.Advanced.wouldShowVersions)
                    }
                    KilnRowDivider()
                    KilnRow(title: Strings.Advanced.uploads) {
                        folderTrailing(Strings.Advanced.uploadsSize, note: Strings.Advanced.wouldShowUploads)
                    }
                }
            }
            KilnGroupTitle(Strings.Advanced.troubleshooting)
            KilnCard {
                KilnRow(title: Strings.Advanced.appLog, subtitle: Strings.Advanced.appLogSub) {
                    Button(Strings.Advanced.showAppLog) {
                        model.send(.revealLog)
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                    .disabled(!model.isSample && !model.can(HostActionType.appRevealLog))
                }
                if SettingsControls.showsSyncLog(model.snapshot.sync) {
                    KilnRowDivider()
                    KilnRow(title: Strings.Advanced.syncLog, subtitle: Strings.Advanced.syncLogSub) {
                        Button(Strings.Advanced.showSyncLog) {
                            model.send(.revealSyncLog)
                        }
                        .buttonStyle(KilnButtonStyle(.plain))
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .onChange(of: limitFocused) { _, focused in
            if !focused { commitLimitMB() }
        }
        .onDisappear {
            if limitDraft != nil { commitLimitMB() }
        }
        .alert(Strings.Offline.clearConfirmTitle, isPresented: $confirmingClear) {
            Button(Strings.Offline.cancel, role: .cancel) {}
            Button(Strings.Offline.clearConfirmButton, role: .destructive) { model.send(.clearOfflineCopies) }
        } message: {
            Text(Strings.Offline.clearConfirmBody)
        }
        .alert(Strings.Offline.offConfirmTitle, isPresented: $confirmingOff) {
            Button(Strings.Offline.cancel, role: .cancel) {}
            Button(Strings.Offline.offConfirmButton, role: .destructive) { model.send(.setOfflineCopies(false)) }
        } message: {
            Text(Strings.Offline.offConfirmBody)
        }
    }

    @ViewBuilder private var offlineGroup: some View {
        KilnGroupTitle(Strings.Offline.title)
        KilnCard {
            KilnRow(title: Strings.Offline.switchTitle, subtitle: Strings.Offline.help) {
                Toggle(isOn: Binding(
                    get: { offlineOn },
                    set: { on in
                        if on { model.send(.setOfflineCopies(true)) } else { confirmingOff = true }
                    }
                )) {
                    Text(Strings.Offline.switchTitle)
                }
                .toggleStyle(KilnSwitchStyle())
                .disabled(controlsDisabled)
            }
            if offlineOn && offlineAvailable {
                KilnRowDivider()
                defaultChoice
                KilnRowDivider()
                fullChoice
                KilnRowDivider()
                KilnRow(title: Strings.Offline.usage(bytes: model.snapshot.offline.bytes, files: model.snapshot.offline.files)) {
                    Button(Strings.Offline.clear) { confirmingClear = true }
                        .buttonStyle(KilnButtonStyle(.plain))
                        .disabled(model.snapshot.offline.files == 0 || !offlineAvailable || (!model.isSample && !model.can(HostActionType.offlineClear)))
                }
            }
        }
        Text(offlineAvailable ? Strings.Offline.privacyNote : Strings.Offline.unavailable)
            .font(.system(size: 12.5))
            .foregroundStyle(kiln.muted)
            .fixedSize(horizontal: false, vertical: true)
            .padding(.horizontal, 2)
            .padding(.top, 8)
    }

    private var defaultChoice: some View {
        let selected = offlinePolicy.mode == .default
        return VStack(alignment: .leading, spacing: 0) {
            modeHeader(
                selected: selected,
                title: Strings.Offline.modeDefault,
                subtitle: Strings.Offline.modeDefaultHelp
            ) {
                selectMode(.default)
            }
            if selected {
                VStack(alignment: .leading, spacing: 0) {
                    KilnRowDivider()
                    HStack(spacing: 12) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(Strings.Offline.space)
                                .font(.system(size: 13, weight: .medium))
                                .foregroundStyle(kiln.ink)
                            Text(Strings.Offline.spaceHelp)
                                .font(.system(size: 12))
                                .foregroundStyle(kiln.muted)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        KilnSegmented(selection: Binding(
                            get: { offlinePolicy.space },
                            set: { space in updatePolicy { $0.space = space } }
                        ), options: [
                            (value: OfflineSpace.mb500, label: Strings.Offline.space500),
                            (value: OfflineSpace.gb2, label: Strings.Offline.space2),
                            (value: OfflineSpace.gb10, label: Strings.Offline.space10),
                        ])
                        .disabled(controlsDisabled)
                    }
                    .padding(.top, 8)
                    .padding(.bottom, 2)
                }
                .padding(.top, 10)
                .padding(.leading, 25)
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .background(selected ? kiln.card2 : Color.clear)
    }

    private var fullChoice: some View {
        let selected = offlinePolicy.mode == .full
        return VStack(alignment: .leading, spacing: 0) {
            modeHeader(
                selected: selected,
                title: Strings.Offline.modeFull,
                subtitle: Strings.Offline.modeFullHelp
            ) {
                selectMode(.full)
            }
            if selected {
                VStack(alignment: .leading, spacing: 0) {
                    KilnRowDivider()
                    HStack(spacing: 12) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(Strings.Offline.limitsSwitch)
                                .font(.system(size: 13, weight: .medium))
                                .foregroundStyle(kiln.ink)
                            Text(Strings.Offline.limitsHelp)
                                .font(.system(size: 12))
                                .foregroundStyle(kiln.muted)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        Toggle(isOn: Binding(
                            get: { offlinePolicy.limited },
                            set: { on in
                                limitDraft = nil
                                limitProblem = nil
                                updatePolicy { $0.limited = on }
                            }
                        )) {
                            Text(Strings.Offline.limitsSwitch)
                        }
                        .toggleStyle(KilnSwitchStyle())
                        .disabled(controlsDisabled)
                    }
                    .padding(.top, 8)
                    .padding(.bottom, offlinePolicy.limited ? 8 : 2)
                    if offlinePolicy.limited {
                        KilnRowDivider()
                        mediaSwitchRow(Strings.Offline.mediaImages, isOn: offlinePolicy.images) { on in
                            updatePolicy { $0.images = on }
                        }
                        KilnRowDivider()
                        mediaSwitchRow(Strings.Offline.mediaVideo, isOn: offlinePolicy.video) { on in
                            updatePolicy { $0.video = on }
                        }
                        KilnRowDivider()
                        mediaSwitchRow(Strings.Offline.mediaAudio, isOn: offlinePolicy.audio) { on in
                            updatePolicy { $0.audio = on }
                        }
                        KilnRowDivider()
                        customSpaceSubrow
                    }
                }
                .padding(.top, 10)
                .padding(.leading, 25)
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .background(selected ? kiln.card2 : Color.clear)
    }

    private func modeHeader(selected: Bool, title: String, subtitle: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack(alignment: .top, spacing: 10) {
                KilnRadio(on: selected)
                    .padding(.top, 2)
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                        .font(.system(size: 14, weight: selected ? .semibold : .medium))
                        .foregroundStyle(kiln.ink)
                    Text(subtitle)
                        .font(.system(size: 12.5))
                        .foregroundStyle(kiln.muted)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(controlsDisabled)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    private func mediaSwitchRow(_ label: String, isOn: Bool, set: @escaping (Bool) -> Void) -> some View {
        HStack(spacing: 12) {
            Text(label)
                .font(.system(size: 13, weight: .medium))
                .foregroundStyle(kiln.ink)
                .frame(maxWidth: .infinity, alignment: .leading)
            Toggle(isOn: Binding(get: { isOn }, set: set)) {
                Text(label)
            }
            .toggleStyle(KilnSwitchStyle())
            .disabled(controlsDisabled)
        }
        .padding(.vertical, 7)
    }

    private var customSpaceSubrow: some View {
        let text = displayedLimitText
        return VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 12) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(Strings.Offline.customSpace)
                        .font(.system(size: 13, weight: .medium))
                        .foregroundStyle(kiln.ink)
                    Text(Strings.Offline.customSpaceHelp)
                        .font(.system(size: 12))
                        .foregroundStyle(kiln.muted)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                HStack(spacing: 6) {
                    KilnInput(
                        text: Binding(
                            get: { displayedLimitText },
                            set: { next in
                                guard next != displayedLimitText else { return }
                                limitDraft = next
                                limitProblem = nil
                            }
                        ),
                        placeholder: Strings.Offline.customSpacePlaceholder,
                        mono: !text.isEmpty
                    )
                    .frame(width: 104)
                    .focused($limitFocused)
                    .onSubmit { commitLimitMB() }
                    .disabled(controlsDisabled)
                    .accessibilityLabel(Text(Strings.Offline.customSpaceAria))
                    Text(Strings.Offline.customSpaceUnit)
                        .font(.system(size: 12.5))
                        .foregroundStyle(kiln.muted)
                }
            }
            if let problem = limitProblem {
                Text(problem)
                    .font(.system(size: 12))
                    .foregroundStyle(kiln.danger)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(.top, 8)
        .padding(.bottom, 2)
    }

    private func selectMode(_ mode: OfflineMode) {
        guard offlinePolicy.mode != mode else { return }
        limitDraft = nil
        limitProblem = nil
        updatePolicy { $0.mode = mode }
    }

    private func updatePolicy(_ mutate: (inout OfflinePolicy) -> Void) {
        var next = offlinePolicy
        mutate(&next)
        guard next != offlinePolicy else { return }
        model.send(.setOfflinePolicy(next))
    }

    private func commitLimitMB() {
        guard let draft = limitDraft else { return }
        limitDraft = nil
        guard let parsed = OfflinePolicy.parseLimitMB(draft) else {
            limitProblem = Strings.Offline.customSpaceRange
            return
        }
        limitProblem = nil
        updatePolicy { $0.limitMB = parsed }
    }

    private func folderTrailing(_ size: String, note: String) -> some View {
        HStack(spacing: 10) {
            Text(size)
                .font(.system(size: 13))
                .foregroundStyle(kiln.muted)
            if SettingsControls.showsStorageTotals(isSample: model.isSample) {
                Button(Strings.Advanced.showInFinder) {
                    model.say(note)
                }
                .buttonStyle(KilnButtonStyle(.plain))
            }
        }
    }
}
