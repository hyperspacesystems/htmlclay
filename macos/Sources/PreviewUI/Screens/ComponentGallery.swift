import SwiftUI
import PreviewCore

public struct ComponentGallery: View {
    @Environment(\.kiln) private var kiln
    @State private var switchOn = true
    @State private var segment = 0
    @State private var input = Strings.Preview.Gallery.inputValue
    private let squircleSymbols: [String] = [
        Symbols.folder, Symbols.doc, Symbols.program, Symbols.sync, Symbols.person,
        Symbols.update, Symbols.advanced, Symbols.plus, Symbols.more,
    ]
    public init() {}
    public var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                Text(Strings.Preview.Gallery.title)
                    .font(.system(size: 18, weight: .bold))
                KilnPath(Strings.Preview.Gallery.path)
                PreviewNote(Strings.Preview.Gallery.note)

                KilnGroupTitle(Strings.Preview.Gallery.cards)
                KilnCard {
                    KilnRow(
                        title: Strings.Preview.Gallery.plainRowTitle,
                        subtitle: Strings.Preview.Gallery.plainRowSubtitle,
                        pills: [.trusted, .served]
                    ) { EmptyView() }
                    KilnRowDivider()
                    KilnRow(
                        title: Strings.Preview.Gallery.rowTitle,
                        subtitle: Strings.Preview.Gallery.rowSubtitle,
                        pills: [.synced]
                    ) {
                        Squircle(.teal, symbol: Symbols.folder)
                    } trailing: {
                        StatusDot(.green)
                    }
                    KilnRowDivider()
                    KilnRow(title: Strings.Preview.Gallery.switchTitle, subtitle: Strings.Preview.Gallery.switchSubtitle) {
                        Toggle(isOn: $switchOn) { Text(Strings.Preview.Gallery.switchTitle) }
                            .toggleStyle(KilnSwitchStyle())
                    }
                    KilnRowDivider()
                    KilnRow(title: Strings.Preview.Gallery.badgeTitle) {
                        KilnBadge(3)
                    }
                }

                KilnCard(kind: .warn) {
                    KilnRow(
                        title: Strings.Preview.Gallery.warnCardTitle,
                        subtitle: Strings.Preview.Gallery.warnCardSubtitle,
                        pills: [.warn]
                    ) {
                        Image(systemName: Symbols.warning)
                    } trailing: {
                        EmptyView()
                    }
                }

                KilnCard(kind: .err) {
                    KilnRow(
                        title: Strings.Preview.Gallery.errCardTitle,
                        subtitle: Strings.Preview.Gallery.errCardSubtitle,
                        pills: [.err]
                    ) {
                        Image(systemName: Symbols.warning)
                    } trailing: {
                        EmptyView()
                    }
                }

                KilnGroupTitle(Strings.Preview.Gallery.buttons)
                KilnCard {
                    VStack(alignment: .leading, spacing: 10) {
                        HStack(spacing: 8) {
                            Button(Strings.Preview.Gallery.plainButton) {}
                                .buttonStyle(KilnButtonStyle())
                            Button(Strings.Preview.Gallery.largeButton) {}
                                .buttonStyle(KilnButtonStyle(.large))
                            Button(Strings.Preview.Gallery.primaryButton) {}
                                .buttonStyle(KilnButtonStyle(.primary))
                            Button(Strings.Preview.Gallery.dangerButton) {}
                                .buttonStyle(KilnButtonStyle(.danger))
                        }
                        Button {} label: {
                            HStack(spacing: 6) {
                                Image(systemName: Symbols.plus)
                                Text(Strings.Preview.Gallery.withSymbol)
                            }
                        }
                        .buttonStyle(KilnButtonStyle())
                        Button {} label: {
                            HStack(spacing: 8) {
                                Text(Strings.Preview.Gallery.openDashboard)
                            }
                        }
                        .buttonStyle(KilnButtonStyle(.openDashboard))
                    }
                    .padding(14)
                }

                KilnGroupTitle(Strings.Preview.Gallery.pills)
                KilnCard {
                    HStack(spacing: 6) {
                        ForEach(Array(KilnPillLevel.allCases.enumerated()), id: \.offset) { item in
                            KilnPill(item.element, text: pillLabel(item.element))
                        }
                    }
                    .padding(14)
                }

                KilnGroupTitle(Strings.Preview.Gallery.squircles)
                KilnCard {
                    HStack(spacing: 10) {
                        ForEach(Array(SquircleColor.allCases.enumerated()), id: \.offset) { item in
                            Squircle(item.element, symbol: squircleSymbols[item.offset])
                        }
                        Squircle(.cobalt, symbol: Symbols.doc, size: .large)
                        Squircle(.amber, symbol: Symbols.update, size: .extraLarge)
                    }
                    .padding(14)
                }

                KilnGroupTitle(Strings.Preview.Gallery.statusDots)
                KilnCard {
                    HStack(spacing: 12) {
                        ForEach(Array(StatusDotKind.allCases.enumerated()), id: \.offset) { item in
                            StatusDot(item.element)
                        }
                    }
                    .padding(14)
                }

                KilnGroupTitle(Strings.Preview.Gallery.segmented)
                KilnCard {
                    VStack(alignment: .leading, spacing: 10) {
                        KilnSegmented(selection: $segment, options: [
                            (value: 0, label: Strings.Preview.Gallery.segmentAll),
                            (value: 1, label: Strings.Preview.Gallery.segmentNeedsYou),
                            (value: 2, label: Strings.Preview.Gallery.segmentSynced),
                        ])
                        KilnInput(text: $input, placeholder: Strings.Preview.Gallery.input)
                    }
                    .padding(14)
                }

                KilnGroupTitle(Strings.Preview.Gallery.nesting)
                NestCard(level: .outer) {
                    VStack(alignment: .leading, spacing: 0) {
                        Text(Strings.Preview.Gallery.outerFrame)
                            .font(.system(size: 14, weight: .medium))
                            .foregroundStyle(kiln.ink)
                        NestCard(level: .mid) {
                            VStack(alignment: .leading, spacing: 0) {
                                Text(Strings.Preview.Gallery.midFrame)
                                    .font(.system(size: 14, weight: .medium))
                                    .foregroundStyle(kiln.ink)
                                NestCard(level: .inner) {
                                    Text(Strings.Preview.Gallery.innerFrame)
                                        .font(.system(size: 13))
                                        .foregroundStyle(kiln.muted)
                                }
                            }
                        }
                    }
                }

                KilnGroupTitle(Strings.Preview.Gallery.links)
                KilnCard {
                    VStack(alignment: .leading, spacing: 10) {
                        KilnLink(Strings.Preview.Gallery.link) {}
                        KilnLink(Strings.Preview.Gallery.dangerLink, danger: true) {}
                        HStack(spacing: 8) {
                            KilnBackButton {}
                            Text(Strings.Preview.Gallery.back)
                                .font(.system(size: 13.5))
                                .foregroundStyle(kiln.ink2)
                        }
                    }
                    .padding(14)
                }
            }
            .padding(24)
            .frame(width: 560, alignment: .leading)
        }
    }
    private func pillLabel(_ level: KilnPillLevel) -> String {
        switch level {
        case .trusted: return Strings.Levels.trusted
        case .served: return Strings.Levels.served
        case .synced: return Strings.Levels.synced
        case .warn: return Strings.Levels.warning
        case .err: return Strings.Levels.error
        }
    }
}
