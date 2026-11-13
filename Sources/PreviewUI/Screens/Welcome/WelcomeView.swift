import AppKit
import SwiftUI
import PreviewCore

public struct WelcomeView: View {
    @Environment(\.kiln) private var kiln
    private let film: String?
    private let menuBarIcon: NSImage?
    private let done: () -> Void
    private let openSettings: () -> Void

    public init(film: String?, menuBarIcon: NSImage?, done: @escaping () -> Void = {}, openSettings: @escaping () -> Void = {}) {
        self.film = film
        self.menuBarIcon = menuBarIcon
        self.done = done
        self.openSettings = openSettings
    }

    public var body: some View {
        VStack(spacing: 0) {
            inner
                .padding(.top, 8)
                .padding(.horizontal, 34)
        }
        .padding(.top, 14)
        .padding(.horizontal, 24)
        .padding(.bottom, 26)
        .frame(width: 760)
    }

    private var inner: some View {
        VStack(spacing: 0) {
            icon
                .frame(width: 104, height: 104)
                .padding(.bottom, 14)
            Text(Strings.Welcome.title)
                .font(.system(size: 23, weight: .bold))
                .padding(.bottom, 6)
            Text(Strings.Welcome.lead)
                .font(.system(size: 14.5))
                .foregroundStyle(kiln.ink2)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 500)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.bottom, 18)
            KilnCard {
                KilnRow(title: Strings.Welcome.filesTitle, subtitle: Strings.Welcome.filesSub) {
                    Squircle(.cobalt, symbol: Symbols.doc, size: .large)
                } trailing: {
                    EmptyView()
                }
                KilnRowDivider()
                KilnRow(title: Strings.Welcome.versionsTitle, subtitle: Strings.Welcome.versionsSub) {
                    Squircle(.teal, symbol: Symbols.versions, size: .large)
                } trailing: {
                    EmptyView()
                }
                KilnRowDivider()
                KilnRow(title: Strings.Welcome.localTitle, subtitle: Strings.Welcome.localSub) {
                    Squircle(.jade, symbol: Symbols.mac, size: .large)
                } trailing: {
                    EmptyView()
                }
                KilnRowDivider()
                KilnRow(title: Strings.Welcome.moreTitle, subtitle: Strings.Welcome.moreSub) {
                    Squircle(.sky, symbol: Symbols.sparkles, size: .large)
                } trailing: {
                    EmptyView()
                }
            }
            .padding(.bottom, 18)
            HStack(spacing: 10) {
                Button(action: openSettings) {
                    Text(Strings.Welcome.openSettings).frame(maxWidth: .infinity)
                }
                .buttonStyle(KilnButtonStyle(.large))
                Button(action: done) {
                    Text(Strings.Welcome.allSet).frame(maxWidth: .infinity)
                }
                .buttonStyle(KilnButtonStyle(.primary))
            }
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
    }

    @ViewBuilder private var icon: some View {
        if let film {
            FilmView(html: film)
        } else {
            Squircle(.amber, symbol: Symbols.doc, size: .extraLarge)
        }
    }
}
