import SwiftUI

public struct KilnInput: View {
    @Environment(\.kiln) private var kiln
    @Binding private var text: String
    private let placeholder: String
    public init(text: Binding<String>, placeholder: String) {
        self._text = text
        self.placeholder = placeholder
    }
    public var body: some View {
        TextField(placeholder, text: $text)
            .textFieldStyle(.plain)
            .font(.system(size: 13.5))
            .foregroundStyle(kiln.ink)
            .padding(.horizontal, 10)
            .padding(.vertical, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(kiln.inputBg, in: RoundedRectangle(cornerRadius: 6))
            .overlay(RoundedRectangle(cornerRadius: 6).strokeBorder(kiln.inputBorder, lineWidth: 1))
    }
}
