import SwiftUI

public struct KilnLink: View {
    @Environment(\.kiln) private var kiln
    private let text: String
    private let danger: Bool
    private let action: () -> Void
    public init(_ text: String, danger: Bool = false, action: @escaping () -> Void) {
        self.text = text
        self.danger = danger
        self.action = action
    }
    public var body: some View {
        Button(action: action) {
            Text(text)
                .font(.system(size: 13.5, weight: .medium))
                .foregroundStyle(danger ? kiln.danger : kiln.link)
        }
        .buttonStyle(.plain)
    }
}
