import SwiftUI

public struct KilnInput: View {
    @Environment(\.kiln) private var kiln
    @Binding private var text: String
    private let placeholder: String
    private let mono: Bool
    private let secure: Bool
    public init(text: Binding<String>, placeholder: String, mono: Bool = false, secure: Bool = false) {
        self._text = text
        self.placeholder = placeholder
        self.mono = mono
        self.secure = secure
    }
    public var body: some View {
        field
            .textFieldStyle(.plain)
            .font(.system(size: 13.5, design: mono ? .monospaced : .default))
            .foregroundStyle(kiln.ink)
            .padding(.horizontal, 10)
            .padding(.vertical, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(kiln.inputBg, in: RoundedRectangle(cornerRadius: 6))
            .overlay(RoundedRectangle(cornerRadius: 6).strokeBorder(kiln.inputBorder, lineWidth: 1))
    }
}

extension KilnInput {
    @ViewBuilder fileprivate var field: some View {
        if secure {
            SecureField(placeholder, text: $text)
        } else {
            TextField(placeholder, text: $text)
        }
    }
}

public struct KilnInputBox: View {
    @Environment(\.kiln) private var kiln
    private let text: String
    private let mono: Bool
    public init(_ text: String, mono: Bool = false) {
        self.text = text
        self.mono = mono
    }
    public var body: some View {
        Text(text)
            .font(.system(size: 13.5, design: mono ? .monospaced : .default))
            .foregroundStyle(kiln.ink)
            .lineLimit(1)
            .truncationMode(.middle)
            .padding(.horizontal, 10)
            .padding(.vertical, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(kiln.inputBg, in: RoundedRectangle(cornerRadius: 6))
            .overlay(RoundedRectangle(cornerRadius: 6).strokeBorder(kiln.inputBorder, lineWidth: 1))
    }
}
