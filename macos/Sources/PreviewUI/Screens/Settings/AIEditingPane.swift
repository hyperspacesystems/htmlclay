import SwiftUI
import PreviewCore

struct AIEditingPane: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(Strings.Settings.title(.aiEditing))
                .font(.system(size: 18, weight: .bold))
                .foregroundStyle(kiln.ink)
                .padding(.bottom, 12)
            KilnCard {
                KilnRow(title: Strings.General.aiEnableTitle, subtitle: Strings.General.aiSub) {
                    Toggle(isOn: Binding(
                        get: { model.snapshot.preferences.aiEditing },
                        set: { model.send(.setAIEditing($0)) }
                    )) {
                        Text(Strings.General.aiEnableTitle)
                    }
                    .toggleStyle(KilnSwitchStyle())
                }
            }
            if model.snapshot.preferences.aiEditing {
                toolsTable
                    .padding(.top, 18)
                if SettingsControls.showsNoToolsNote(model.snapshot.preferences) {
                    Text(Strings.General.aiNoTools)
                        .font(.system(size: 12.5))
                        .foregroundStyle(kiln.muted)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.top, 14)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.bottom, 32)
    }

    @ViewBuilder private var toolsTable: some View {
        let rows = SettingsControls.aiToolRows(model.snapshot.preferences)
        let builtins = rows.filter { !$0.custom }
        let custom = rows.first { $0.custom }
        Grid(alignment: .leading, horizontalSpacing: 14, verticalSpacing: 10) {
            GridRow {
                Text(Strings.General.aiColTool)
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundStyle(kiln.muted)
                    .frame(minWidth: 168, maxWidth: .infinity, alignment: .leading)
                Text(Strings.General.aiColModel)
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundStyle(kiln.muted)
                    .frame(minWidth: 170, alignment: .leading)
                Text(Strings.General.aiColEffort)
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundStyle(kiln.muted)
                    .frame(minWidth: 115, alignment: .leading)
            }
            hairline
            ForEach(builtins, id: \.id) { row in
                GridRow(alignment: .center) {
                    toolCell(row)
                    modelPicker(row)
                    effortPicker(row)
                }
                .padding(.vertical, 2)
                hairline
            }
            if let custom {
                GridRow(alignment: .center) {
                    HStack(spacing: 10) {
                        Button {
                            model.send(.setAIEditTool(custom.id))
                        } label: {
                            KilnRadio(on: custom.selected)
                        }
                        .buttonStyle(.plain)
                        .disabled(custom.selected || !custom.selectable)
                        .accessibilityLabel(Text(custom.title))
                        .accessibilityAddTraits(custom.selected ? .isSelected : [])
                        Text(custom.title)
                            .font(.system(size: 14, weight: .medium))
                            .foregroundStyle(kiln.ink)
                    }
                    Text(custom.subtitle)
                        .font(custom.found ? .system(size: 12.5, design: .monospaced) : .system(size: 12.5))
                        .foregroundStyle(custom.found ? kiln.ink2 : kiln.muted)
                        .lineLimit(1)
                        .truncationMode(.middle)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    Button(Strings.General.aiCustomButton) {
                        model.sheet = .customCommand
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
                .padding(.vertical, 2)
            }
        }
    }

    private var hairline: some View {
        Rectangle()
            .fill(kiln.rowSep)
            .frame(height: 1)
            .gridCellUnsizedAxes(.horizontal)
    }

    private func toolCell(_ row: AIToolRow) -> some View {
        HStack(spacing: 10) {
            Button {
                model.send(.setAIEditTool(row.id))
            } label: {
                KilnRadio(on: row.selected)
            }
            .buttonStyle(.plain)
            .disabled(row.selected || !row.selectable)
            .accessibilityLabel(Text(row.title))
            .accessibilityAddTraits(row.selected ? .isSelected : [])
            VStack(alignment: .leading, spacing: 2) {
                Text(row.title)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(row.found ? kiln.ink : kiln.muted)
                Text(row.subtitle)
                    .font(.system(size: 12))
                    .foregroundStyle(kiln.muted)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func modelPicker(_ row: AIToolRow) -> some View {
        Picker(Strings.General.aiModelLabel(row.title), selection: Binding(
            get: { row.selectedModel },
            set: { next in
                if next != row.selectedModel {
                    model.send(.setAIEditModel(tool: row.id, model: next))
                }
            }
        )) {
            ForEach(row.models, id: \.self) { id in
                Text(Strings.General.aiModelDisplayName(id)).tag(id)
            }
        }
        .labelsHidden()
        .pickerStyle(.menu)
        .frame(minWidth: 170, alignment: .leading)
        .disabled(row.models.isEmpty)
    }

    private func effortPicker(_ row: AIToolRow) -> some View {
        Picker(Strings.General.aiEffort, selection: Binding(
            get: { row.selectedEffort },
            set: { next in
                if next != row.selectedEffort {
                    model.send(.setAIEditEffort(tool: row.id, effort: next))
                }
            }
        )) {
            ForEach(row.efforts, id: \.self) { effort in
                Text(Strings.General.aiEffortName(effort)).tag(effort)
            }
        }
        .labelsHidden()
        .pickerStyle(.menu)
        .frame(minWidth: 115, alignment: .leading)
        .disabled(row.efforts.isEmpty)
    }
}
