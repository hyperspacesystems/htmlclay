import SwiftUI
import PreviewCore

public struct RequestActions {
    public var answer: (WaitingItem, ProgramRequest, String, String) -> Void
    public var close: () -> Void
    public init(answer: @escaping (WaitingItem, ProgramRequest, String, String) -> Void = { _, _, _, _ in },
                close: @escaping () -> Void = {}) {
        self.answer = answer; self.close = close
    }
}

public struct RequestView: View {
    private let itemID: String
    private let actions: RequestActions
    @Environment(AppModel.self) private var model
    @Environment(\.kiln) private var kiln
    public init(itemID: String, actions: RequestActions = RequestActions()) {
        self.itemID = itemID
        self.actions = actions
    }
    public var body: some View {
        if let item = model.snapshot.waiting.first(where: { $0.id == itemID }),
           let request = model.snapshot.requests.first(where: { $0.id == item.subjectID }) {
            let p = RequestQueue.position(of: itemID, in: model.snapshot)
            RequestBody(request: request, item: item, position: p.position, total: p.total, actions: actions)
                .id(RequestIdentity(request))
        } else {
            VStack(spacing: 0) {
                RequestTitleBar(position: 1, total: 1)
                Text(Strings.Request.ended)
                    .font(.system(size: 13.5))
                    .foregroundStyle(kiln.ink2)
                    .padding(22)
            }
            .frame(width: 460)
            .task {
                try? await Task.sleep(nanoseconds: 1_500_000_000)
                actions.close()
            }
        }
    }
}
