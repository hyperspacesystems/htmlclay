import SwiftUI

public struct KilnPalette: Sendable {
    public let win: Color
    public let side: Color
    public let sideBorder: Color
    public let sideActive: Color
    public let sideActiveRing: Color
    public let sideSelected: Color
    public let sideSelectedInk: Color
    public let card: Color
    public let card2: Color
    public let cardSunk: Color
    public let cardBorder: Color
    public let rowSep: Color
    public let ink: Color
    public let ink2: Color
    public let muted: Color
    public let faint: Color
    public let btnBg: [Color]
    public let btnBorder: Color
    public let btnFg: Color
    public let btnPlainBg: Color
    public let inputBg: Color
    public let inputBorder: Color
    public let menuBg: Color
    public let menuBorder: Color
    public let menuHi: Color
    public let menuHiFg: Color
    public let warnBg: Color
    public let warnBorder: Color
    public let warnFg: Color
    public let errBg: Color
    public let errBorder: Color
    public let errFg: Color
    public let danger: Color
    public let dangerBg: Color
    public let dangerBorder: Color
    public let link: Color
    public let switchOff: Color
    public let segBg: Color
    public let segOn: Color
    public let scrim: Color
    public let lvlTrustBg: Color
    public let lvlTrustFg: Color
    public let lvlServeBg: Color
    public let lvlServeFg: Color
    public let lvlSyncBg: Color
    public let lvlSyncFg: Color
}

public extension KilnPalette {
    static let light = KilnPalette(
        win: Color(css: 0xF4F2EC),
        side: Color(css: 0xE7E3DA),
        sideBorder: Color(css: 0x1C1A16, alpha: 0.09),
        sideActive: Color(css: 0xD9D0BF),
        sideActiveRing: Color(css: 0xB0680C, alpha: 0.34),
        sideSelected: Color(css: 0x0064E1),
        sideSelectedInk: Color(css: 0xF6F7F9),
        card: Color(css: 0xFCFBF7),
        card2: Color(css: 0xF5F2EA),
        cardSunk: Color(css: 0xEAE6DC),
        cardBorder: Color(css: 0x1C1A16, alpha: 0.11),
        rowSep: Color(css: 0x1C1A16, alpha: 0.08),
        ink: Color(css: 0x1A1C23),
        ink2: Color(css: 0x434654),
        muted: Color(css: 0x696D7D),
        faint: Color(css: 0x878B9B),
        btnBg: [Color(css: 0xFCFBF8), Color(css: 0xEFECE3)],
        btnBorder: Color(css: 0x1A1C23, alpha: 0.19),
        btnFg: Color(css: 0x1B1D24),
        btnPlainBg: Color(css: 0xEBE7DD),
        inputBg: Color(css: 0xFCFBF8),
        inputBorder: Color(css: 0x1A1C23, alpha: 0.22),
        menuBg: Color(css: 0xF9F7F2, alpha: 0.98),
        menuBorder: Color(css: 0x1A1C23, alpha: 0.16),
        menuHi: Color(css: 0xEAD7B7),
        menuHiFg: Color(css: 0x3B2304),
        warnBg: Color(css: 0xFAF0DC),
        warnBorder: Color(css: 0xC47A12, alpha: 0.34),
        warnFg: Color(css: 0x543305),
        errBg: Color(css: 0xFAE6E6),
        errBorder: Color(css: 0xC4383E, alpha: 0.30),
        errFg: Color(css: 0x872026),
        danger: Color(css: 0xC22F36),
        dangerBg: Color(css: 0xC22F36, alpha: 0.10),
        dangerBorder: Color(css: 0xC22F36, alpha: 0.28),
        link: Color(css: 0x9C5A08),
        switchOff: Color(css: 0xCCC7BC),
        segBg: Color(css: 0xDFDBD0),
        segOn: Color(css: 0xFCFBF8),
        scrim: Color(css: 0x181920, alpha: 0.26),
        lvlTrustBg: Color(css: 0xE5E1D7),
        lvlTrustFg: Color(css: 0x444754),
        lvlServeBg: Color(css: 0xD9E9F9),
        lvlServeFg: Color(css: 0x19538C),
        lvlSyncBg: Color(css: 0xD8EFE0),
        lvlSyncFg: Color(css: 0x1A6837))

    static let dark = KilnPalette(
        win: Color(css: 0x1F2128),
        side: Color(css: 0x16171D),
        sideBorder: Color(css: 0xF6F7F9, alpha: 0.06),
        sideActive: Color(css: 0x2B303F),
        sideActiveRing: Color(css: 0xF5A629, alpha: 0.34),
        sideSelected: Color(css: 0x0A68D6),
        sideSelectedInk: Color(css: 0xF6F7F9),
        card: Color(css: 0x282B34),
        card2: Color(css: 0x22252E),
        cardSunk: Color(css: 0x191B22),
        cardBorder: Color(css: 0xF6F7F9, alpha: 0.08),
        rowSep: Color(css: 0xF6F7F9, alpha: 0.065),
        ink: Color(css: 0xF1F2F6),
        ink2: Color(css: 0xC5C9D7),
        muted: Color(css: 0x9499AC),
        faint: Color(css: 0x73788B),
        btnBg: [Color(css: 0x393C48), Color(css: 0x30333E)],
        btnBorder: Color(css: 0xF6F7F9, alpha: 0.14),
        btnFg: Color(css: 0xF0F1F5),
        btnPlainBg: Color(css: 0x272A34),
        inputBg: Color(css: 0x171920),
        inputBorder: Color(css: 0xF6F7F9, alpha: 0.16),
        menuBg: Color(css: 0x2C2F3A, alpha: 0.98),
        menuBorder: Color(css: 0xF6F7F9, alpha: 0.16),
        menuHi: Color(css: 0xF5A629, alpha: 0.20),
        menuHiFg: Color(css: 0xFAD698),
        warnBg: Color(css: 0x31281E),
        warnBorder: Color(css: 0xF2A43A, alpha: 0.30),
        warnFg: Color(css: 0xF2DEC0),
        errBg: Color(css: 0x352024),
        errBorder: Color(css: 0xE8545C, alpha: 0.32),
        errFg: Color(css: 0xF5C2C6),
        danger: Color(css: 0xF47272),
        dangerBg: Color(css: 0xE0444C, alpha: 0.12),
        dangerBorder: Color(css: 0xE0444C, alpha: 0.30),
        link: Color(css: 0xF3AC40),
        switchOff: Color(css: 0x3D414F),
        segBg: Color(css: 0x171920),
        segOn: Color(css: 0x343846),
        scrim: Color(css: 0x0B0C13, alpha: 0.50),
        lvlTrustBg: Color(css: 0x313543),
        lvlTrustFg: Color(css: 0xC8CCD9),
        lvlServeBg: Color(css: 0x1E3147),
        lvlServeFg: Color(css: 0x8BC2F4),
        lvlSyncBg: Color(css: 0x1D3528),
        lvlSyncFg: Color(css: 0x89D8A5))
}
