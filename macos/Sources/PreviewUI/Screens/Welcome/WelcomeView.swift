import AppKit
import SwiftUI
import PreviewCore

public struct WelcomeView: View {
    @Environment(\.kiln) private var kiln
    private let film: String?
    private let menuBarIcon: NSImage?
    private let openDashboard: () -> Void
    private let openFile: () -> Void

    private let columns = [
        GridItem(.flexible(), spacing: 32, alignment: .leading),
        GridItem(.flexible(), spacing: 32, alignment: .leading),
    ]

    public init(film: String?, menuBarIcon: NSImage?, openFile: @escaping () -> Void = {}, openDashboard: @escaping () -> Void = {}) {
        self.film = film
        self.menuBarIcon = menuBarIcon
        self.openFile = openFile
        self.openDashboard = openDashboard
    }

    public var body: some View {
        VStack(spacing: 0) {
            icon
                .frame(width: 88, height: 88)
                .padding(.bottom, 12)
            Text(Strings.Welcome.title)
                .font(.system(size: 24, weight: .bold))
                .foregroundStyle(kiln.ink)
                .multilineTextAlignment(.center)
                .padding(.bottom, 22)
            LazyVGrid(columns: columns, alignment: .leading, spacing: 12) {
                capabilityCell(.sky, symbol: Symbols.copy, title: Strings.Welcome.versionsTitle)
                capabilityCell(.cobalt, symbol: Symbols.doc, title: Strings.Welcome.filesTitle)
                capabilityCell(.amber, symbol: Symbols.program, title: Strings.Welcome.moreTitle)
                capabilityCell(.plum, symbol: Symbols.sparkles, title: Strings.Welcome.localTitle)
            }
            .frame(maxWidth: 516)
            .padding(.bottom, 34)
            HStack(spacing: 10) {
                Button(action: openFile) {
                    Text(Strings.Welcome.openFile)
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(KilnButtonStyle(.plain))
                Button(action: openDashboard) {
                    Text(Strings.Welcome.openDashboard)
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(KilnButtonStyle(.primary))
            }
            .frame(maxWidth: 420)
            HStack(spacing: 6) {
                Text(Strings.Welcome.menuBarHint)
                    .font(.system(size: 12.5))
                    .foregroundStyle(kiln.muted)
                if let menuBarIcon {
                    Image(nsImage: menuBarIcon)
                        .renderingMode(.template)
                        .resizable()
                        .frame(width: 16, height: 16)
                        .foregroundStyle(kiln.ink2)
                }
            }
            .padding(.top, 16)
        }
        .padding(.top, 32)
        .padding(.horizontal, 40)
        .padding(.bottom, 26)
        .frame(width: 600)
    }

    private func capabilityCell(_ color: SquircleColor, symbol: String, title: String) -> some View {
        HStack(alignment: .center, spacing: 10) {
            Squircle(color, symbol: symbol, size: .medium)
            Text(title)
                .font(.system(size: 13.5, weight: .medium))
                .foregroundStyle(kiln.ink)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    @ViewBuilder private var icon: some View {
        if let film {
            FilmView(html: film)
        } else {
            Squircle(.amber, symbol: Symbols.doc, size: .extraLarge)
        }
    }
}
