/**
 * The failure code a thrown IndexedDB error means to a caller.
 *
 * Only one of these is about this device being full (`quota`); the rest are about
 * the database being unreachable, refused, or mid-upgrade somewhere else. None of
 * them is a save result, and none of them is durable.
 *
 * @param {*} error
 * @returns {string}
 */
export function codeFor(error) {
  const name = error?.name || "";
  if (name === "QuotaExceededError" || name === "NS_ERROR_DOM_QUOTA_REACHED") return "quota";
  if (name === "VersionError") return "unknown-schema";
  if (name === "DataCloneError") return "uncloneable";
  if (name === "AbortError") return "aborted";
  if (name === "TransactionInactiveError" || name === "InvalidStateError") return "aborted";
  if (name === "SecurityError" || name === "UnknownError" || name === "NotSupportedError") return "unavailable";
  return "error";
}
