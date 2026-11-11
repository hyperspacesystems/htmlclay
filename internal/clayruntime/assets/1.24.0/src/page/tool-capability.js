// A host that lists blockeditor-tool in this document's helper list can answer tool
// requests. Old clients look for a blockeditor-tool/1 entry in `extensions`, which no host
// adds, so they never show a dead /ai. The check stays in the core: the slash menu
// asks it before the tool view part loads.
export const capabilityAdvertised = (meta) =>
  Array.isArray(meta?.document?.helpers) &&
  meta.document.helpers.some((helper) =>
    helper?.name === "blockeditor-tool" && (helper.state === "ready" || helper.state === "unavailable"));
