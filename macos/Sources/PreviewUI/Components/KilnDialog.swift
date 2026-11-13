import AppKit
import SwiftUI

public struct KilnAppIcon: View {
    private let size: CGFloat
    public init(size: CGFloat) { self.size = size }
    public var body: some View {
        Image(nsImage: NSImage(named: NSImage.applicationIconName) ?? NSImage())
            .resizable()
            .frame(width: size, height: size)
    }
}

public struct KilnDialog<Content: View, Buttons: View>: View {
    @Environment(\.kiln) private var kiln
    private let title: String
    private let titleSize: CGFloat
    private let width: CGFloat
    private let content: Content
    private let buttons: Buttons
    public init(_ title: String, titleSize: CGFloat = 16, width: CGFloat = 460, @ViewBuilder content: () -> Content, @ViewBuilder buttons: () -> Buttons) {
        self.title = title
        self.titleSize = titleSize
        self.width = width
        self.content = content()
        self.buttons = buttons()
    }
    public var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(title)
                .font(.system(size: titleSize, weight: .bold))
                .foregroundStyle(kiln.ink)
                .fixedSize(horizontal: false, vertical: true)
            content
            HStack(spacing: 8) {
                Spacer(minLength: 0)
                buttons
            }
            .padding(.top, 18)
        }
        .padding(.top, 16)
        .padding(.horizontal, 22)
        .padding(.bottom, 20)
        .frame(width: width, alignment: .leading)
        .background(kiln.win)
    }
}

public struct KilnLead: View {
    @Environment(\.kiln) private var kiln
    private let text: Text
    public init(_ text: Text) { self.text = text }
    public var body: some View {
        text
            .font(.system(size: 13.5))
            .foregroundStyle(kiln.ink2)
            .lineSpacing(3)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}

public struct KilnField<Content: View>: View {
    @Environment(\.kiln) private var kiln
    private let label: String
    private let content: Content
    public init(_ label: String, @ViewBuilder content: () -> Content) {
        self.label = label
        self.content = content()
    }
    public var body: some View {
        HStack(spacing: 10) {
            Text(label)
                .font(.system(size: 13.5))
                .foregroundStyle(kiln.ink2)
                .frame(width: 78, alignment: .trailing)
            content
        }
    }
}

public struct KilnFieldNote: View {
    @Environment(\.kiln) private var kiln
    private let text: Text
    private let error: Bool
    public init(_ text: Text, error: Bool = false) {
        self.text = text
        self.error = error
    }
    public var body: some View {
        text
            .font(.system(size: 13))
            .foregroundStyle(error ? kiln.danger : kiln.muted)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.leading, 88)
            .padding(.top, 4)
    }
}

public struct KilnAlertBox<Extra: View, Buttons: View>: View {
    @Environment(\.kiln) private var kiln
    private let title: String
    private let message: Text
    private let extra: Extra
    private let buttons: Buttons
    public init(title: String, message: Text, @ViewBuilder extra: () -> Extra, @ViewBuilder buttons: () -> Buttons) {
        self.title = title
        self.message = message
        self.extra = extra()
        self.buttons = buttons()
    }
    public var body: some View {
        VStack(spacing: 0) {
            KilnAppIcon(size: 48)
            Text(title)
                .font(.system(size: 16, weight: .bold))
                .foregroundStyle(kiln.ink)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, 10)
            message
                .font(.system(size: 13.5))
                .foregroundStyle(kiln.ink2)
                .multilineTextAlignment(.center)
                .lineSpacing(3)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, 6)
            extra
            HStack(spacing: 8) { buttons }
                .padding(.top, 16)
        }
        .padding(.top, 20)
        .padding(.horizontal, 22)
        .padding(.bottom, 18)
        .frame(width: 410)
        .background(kiln.win)
    }
}
