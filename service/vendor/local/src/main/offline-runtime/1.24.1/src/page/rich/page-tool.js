import { cloneJson } from "../../live/ops.js";
import { validateArtifact, validateState } from "./tool-runtime.js";
export { capabilityAdvertised } from "../tool-capability.js";

const CONTEXT_LIMIT = 16384;
const bytes = (value) => new TextEncoder().encode(JSON.stringify(value)).length;

/**
 * @typedef {{protocol: 'blockeditor-tool/1', requestId: string, prompt: string,
 *   context: {title: string, outline: unknown[], tables: unknown[]},
 *   target: null | {toolId: string, rev: string},
 *   current: null | {artifact: object, state: unknown}}} ToolRequest
 */
/** @typedef {{requestId: string, artifact: object, state: unknown}} ToolResult */
/** @typedef {{requestId: string, code: string, message: string}} ToolFailure */

export class ToolRequestError extends Error {
  constructor(code, message = code) {
    super(message);
    this.name = "ToolRequestError";
    this.code = code;
  }
}

function trimmedContext(context) {
  const copy = cloneJson(context);
  if (!Array.isArray(copy.tables)) copy.tables = [];
  if (!Array.isArray(copy.outline)) copy.outline = [];
  while (bytes(copy) > CONTEXT_LIMIT) {
    const table = [...copy.tables].reverse().find((item) => Array.isArray(item.rows) && item.rows.length);
    if (table) table.rows.pop();
    else if (copy.outline.length) copy.outline.pop();
    else break;
  }
  if (bytes(copy) > CONTEXT_LIMIT) throw new ToolRequestError("context-too-large");
  return copy;
}

// The wire sees the request id as the host's own column, a UUID: a made-up string is
// refused before the model is asked.
const freshRequestId = () =>
  globalThis.crypto?.randomUUID?.() ?? `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;

export function createToolClient({ transport, newId = freshRequestId }) {
  let active = null;
  const discard = (candidate) => {
    if (!candidate || candidate.discarded || candidate.done) return;
    candidate.discarded = true;
    if (active === candidate) active = null;
    transport.cancel(candidate.requestId);
    candidate.resolve({ requestId: candidate.requestId, code: "discarded", message: "Candidate discarded" });
  };
  return {
    request(prompt, context, target = null, current = null) {
      const trimmed = trimmedContext(context);
      if (active) discard(active);
      const requestId = newId();
      const candidate = { requestId, discarded: false, done: false };
      candidate.promise = new Promise((resolve) => { candidate.resolve = resolve; });
      active = candidate;
      const request = { protocol: "blockeditor-tool/1", requestId, prompt, context: trimmed, target, current };
      candidate.request = request;
      const finish = (reply) => {
        if (candidate.discarded || candidate.done || reply?.requestId !== requestId) return;
        candidate.done = true;
        if (active === candidate) active = null;
        if (reply?.artifact !== undefined) {
          const artifact = validateArtifact(reply.artifact);
          const state = validateState(reply.state);
          const invalid = !artifact.ok ? artifact : !state.ok ? state : null;
          candidate.resolve(invalid ? { requestId, code: invalid.code, message: invalid.code } : reply);
        } else candidate.resolve(typeof reply?.code === "string" ? reply : { requestId, code: "invalid-result", message: "invalid-result" });
      };
      try {
        const failed = (error) => finish({
          requestId,
          code: typeof error?.code === "string" ? error.code : "transport-error",
          message: String(error?.message ?? error),
        });
        Promise.resolve(transport.request(request)).then(finish, failed);
      } catch (error) {
        finish({ requestId, code: typeof error?.code === "string" ? error.code : "transport-error", message: String(error?.message ?? error) });
      }
      return candidate;
    },
    discard,
    async wait(candidate, { signal } = {}) {
      if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
      let abort;
      const outcome = await (signal ? Promise.race([
        candidate.promise,
        new Promise((_, reject) => { abort = () => reject(new DOMException("Aborted", "AbortError")); signal.addEventListener("abort", abort, { once: true }); }),
      ]).finally(() => signal.removeEventListener("abort", abort)) : candidate.promise);
      if (outcome?.code) throw new ToolRequestError(outcome.code, outcome.message);
      return outcome;
    },
  };
}
