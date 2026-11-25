import SwiftUI

public struct KilnSwitchStyle: ToggleStyle {
    @Environment(\.kiln) private var kiln
    @Environment(\.isEnabled) private var isEnabled
    public init() {}
    public func makeBody(configuration: Configuration) -> some View {
        Button { configuration.isOn.toggle() } label: {
            Capsule()
                .fill(configuration.isOn ? KilnFixed.switchOn : kiln.switchOff)
                .frame(width: 38, height: 22)
                .overlay(alignment: configuration.isOn ? .trailing : .leading) {
                    Circle().fill(Color(css: 0xF6F7F9)).frame(width: 18, height: 18).padding(2)
                }
        }
        .buttonStyle(.plain)
        .opacity(isEnabled ? 1 : 0.45)
        .accessibilityRepresentation {
            Toggle(isOn: configuration.$isOn) { configuration.label }
        }
    }
}
