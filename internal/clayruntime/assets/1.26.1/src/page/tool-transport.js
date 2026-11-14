// The blockeditor-tool helper over clay.wire. `clay.wire` is read when a request goes out,
// because the page plugin loads before the wire does: a page with exclude=wire must
// never pull the module in.
export class ToolTransportError extends Error {
  constructor(code) {
    super(code);
    this.name = "ToolTransportError";
    this.code = code;
  }
}

export function createToolTransport({ getWire = () => globalThis.window?.clay?.wire ?? null } = {}) {
  const handles = new Map();
  return {
    request(payload, { onStatus = null } = {}) {
      const wire = getWire();
      if (!wire || typeof wire.send !== "function") return Promise.reject(new ToolTransportError("no-wire"));
      const id = payload?.requestId;
      let handle;
      try {
        handle = wire.send(payload, { helper: "blockeditor-tool", document: "none", id, onStatus });
      } catch (error) {
        return Promise.reject(new ToolTransportError("no-wire"));
      }
      handles.set(id, handle);
      return Promise.resolve(handle.done).then((outcome) => {
        if (handles.get(id) === handle) handles.delete(id);
        // A reply is the wire's structured result; a failure keeps the host's own code
        // so the page can tell a refused prompt from a lost connection.
        if (outcome?.state === "done") {
          const result = outcome.result ?? {};
          return {
            requestId: typeof result.requestId === "string" ? result.requestId : id,
            artifact: result.artifact,
            state: result.state,
            model: result.model,
          };
        }
        return {
          requestId: id,
          code: outcome?.errorCode || (outcome?.state === "cancelled" ? "cancelled" : "transport-error"),
          message: outcome?.error || "the helper reported an error",
        };
      }, () => ({ requestId: id, code: "transport-error", message: "the helper reported an error" }));
    },
    cancel(id) {
      const handle = handles.get(id);
      handles.delete(id);
      handle?.cancel?.();
    },
  };
}
