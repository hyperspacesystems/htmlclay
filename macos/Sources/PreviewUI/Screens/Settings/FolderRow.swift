import SwiftUI
import PreviewCore

struct FolderRow: View {
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    private let folder: Folder
    private let parent: Folder?
    private let waiting: Int

    init(folder: Folder, parent: Folder? = nil, waiting: Int = 0) {
        self.folder = folder
        self.parent = parent
        self.waiting = waiting
    }

    static func pills(for folder: Folder) -> [KilnPillLevel] {
        var levels: [KilnPillLevel] = [.trusted]
        if folder.served { levels.append(.served) }
        if folder.synced { levels.append(.synced) }
        return levels
    }

    var body: some View {
        if folder.onThisMac {
            Button {
                model.route = .folder(folder.id)
            } label: {
                KilnRow(title: folder.name, subtitle: subtitle, pills: Self.pills(for: folder)) {
                    leading
                } trailing: {
                    badge
                    chevron
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
        } else {
            KilnRow(title: folder.name, subtitle: folder.status) {
                Squircle(.slate, symbol: Symbols.folder)
            } trailing: {
                if folder.cannotSetUp {
                    Text(Strings.Sync.viewOnly)
                        .font(.system(size: 13))
                        .foregroundStyle(kiln.muted)
                } else {
                    Button(Strings.Folders.setUp) {
                        model.sheet = .teamSetup(folderID: folder.id)
                    }
                    .buttonStyle(KilnButtonStyle(.plain))
                }
            }
        }
    }

    private var leading: some View {
        HStack(spacing: 4) {
            if parent != nil {
                Text(Strings.Folders.elbow)
                    .font(.system(size: 13))
                    .foregroundStyle(kiln.faint)
            }
            Squircle(squircleColor, symbol: Symbols.folder)
        }
    }

    @ViewBuilder private var badge: some View {
        if waiting > 0 {
            KilnBadge(waiting)
        }
    }

    private var chevron: some View {
        Image(systemName: Symbols.chevronRight)
            .font(.system(size: 11))
            .foregroundStyle(kiln.faint)
    }

    private var squircleColor: SquircleColor {
        switch folder.origin {
        case .local: return folder.served ? .teal : .sky
        case .account: return .cobalt
        case .team: return .plum
        }
    }

    private var subtitle: String? {
        if let parent {
            return Strings.Folders.inside(parent: parent.name, path: folder.path)
        }
        switch folder.origin {
        case .local: return folder.served ? Strings.Folders.onlyOnThisMac : folder.path
        case .account, .team: return folder.status
        }
    }
}
