# Rules

- macOS only. Never UIKit names. Never raise the platform in `Package.swift`.
- Never use `ObservableObject`, `@Published`, `@StateObject`, `@ObservedObject`, `@EnvironmentObject`, `import Combine`, `NavigationSplitView`, `NavigationStack`, `MenuBarExtra`, `SettingsLink`. State is `@Observable` with `@Environment(AppModel.self)`.
- No double quote character anywhere under `Sources/PreviewUI/Screens/` or `Sources/PreviewUI/Components/`. Text comes from `Strings` (in PreviewCore), symbol names from `Symbols`. `scripts/check.sh` fails otherwise.
- Colors only from `kiln` or `KilnFixed`. Never `Color.white`, `Color.black`, `.white`, `.black`, `Color.primary`, `Color.accentColor` or any system color. Never a pure `#FFFFFF` or `#000000` value.
- No comments in code. No new dependencies.
