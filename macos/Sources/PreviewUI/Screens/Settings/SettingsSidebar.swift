import SwiftUI
import PreviewCore

public struct SettingsSidebar: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    private let active: SettingsPane

    public init(active: SettingsPane) { self.active = active }

    public static func badgeCount(for pane: SettingsPane, snapshot: HostSnapshot) -> Int? {
        let count = snapshot.count(in: pane)
        return count > 0 ? count : nil
    }

    public var body: some View {
        VStack(spacing: 2) {
            Color.clear
                .frame(height: 32)
            ForEach(SettingsPane.allCases.filter { SettingsControls.showsPane($0, in: model.snapshot) }, id: \.self) { pane in
                row(pane)
            }
            Spacer(minLength: 0)
        }
        .padding(.top, 12)
        .padding(.horizontal, 9)
        .padding(.bottom, 14)
        .frame(width: 214)
        .background(kiln.side, in: RoundedRectangle(cornerRadius: 10))
        .overlay(RoundedRectangle(cornerRadius: 10).strokeBorder(kiln.sideBorder, lineWidth: 1))
        .padding(.top, 8)
        .padding(.bottom, 8)
        .padding(.leading, 8)
    }

    private func row(_ pane: SettingsPane) -> some View {
        let selected = active == pane
        return Button {
            model.route = .pane(pane)
        } label: {
            HStack(spacing: 9) {
                Squircle(Self.color(for: pane), symbol: Self.symbol(for: pane))
                Text(Strings.Settings.title(pane))
                    .font(.system(size: 13.5, weight: .medium))
                    .foregroundStyle(selected ? kiln.sideSelectedInk : kiln.ink2)
                Spacer(minLength: 0)
                if let count = Self.badgeCount(for: pane, snapshot: model.snapshot) {
                    KilnBadge(count)
                }
            }
            .padding(.vertical, 6)
            .padding(.horizontal, 9)
            .background(selected ? kiln.sideSelected : Color.clear, in: RoundedRectangle(cornerRadius: 7))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .focusEffectDisabled()
    }

    private static func color(for pane: SettingsPane) -> SquircleColor {
        switch pane {
        case .general: return .slate
        case .aiEditing: return .plum
        case .folders: return .sky
        case .programs: return .amber
        case .sync: return .cobalt
        case .updates: return .jade
        case .advanced: return .slate
        }
    }

    private static func symbol(for pane: SettingsPane) -> String {
        switch pane {
        case .general: return Symbols.gear
        case .aiEditing: return Symbols.sparkles
        case .folders: return Symbols.folder
        case .programs: return Symbols.program
        case .sync: return Symbols.sync
        case .updates: return Symbols.update
        case .advanced: return Symbols.advanced
        }
    }
}
