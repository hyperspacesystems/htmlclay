// The live room's codec contract and its laws. A codec is one plain object; the
// protocol core in ./core reads documents and actions only through these members.
// This file has no ClayJS imports, so a codec package (sapjs) can copy it.
//
// read(bytes) -> { record, doc }            write(doc, record, extras) -> bytes
// docBytes(doc) -> bytes counted against the room's memory budget
// validate(action) -> false for a malformed action
// apply(doc, action) -> { out: "applied" | "refused", inverse }; changes doc only when applied
// size(action) -> bytes, an upper bound on the wire       ops(action) -> operation count, at least 1
// split({ action, saw, reviewed }, limits) -> { head: { action, saw, reviewed }, parts }: head
//   is the action, saw and reviewed saw with their payload emptied (a missing saw or reviewed
//   saw stays missing), each part a body of at most limits.opsPerFrame operations
// assemble(head, parts) -> { action, saw, reviewed }      partOps(part) -> operations in a part
// targets(action) -> ids of what the action writes        creates(action) -> ids it brings into being
// needs(action) -> ids that must exist                    removes(action) -> ids it takes away
// has(doc, id) -> whether an id exists
// seen(view, action) -> what the author's tab showed for the targets
// fits(doc, action) -> false when the action no longer has a place
// conflicts(doc, action, saw) -> what differs from saw; empty means no conflict
// unseen(doc, action, saw) -> values a removal would take away that its author did not see
// conflicts, unseen and review take a saw that may be stale or null (null: nothing was seen)
// review(action, saw, cells) -> the saw after a person looked at the conflicting cells
// holds(doc, action) -> whether the document already shows what the action writes
// describe(action, doc) -> { title, cells: [{ target, label, stored }] }
// current(doc, target) -> the value now                    preview(doc, actions) -> a new doc
//
// Every id (targets, creates, needs, removes) is a string: core compares ids with === and
// keys maps by them. A document is plain data that structuredClone copies, because core
// copies documents that way. A part may not use a field name the core's part frame uses
// (PART_FIELDS), because the core spreads the part into that frame. write returns a string.
// The core decides an edit was saved when the file has what it needs and lacks what it
// creates (law C8), so a codec may not refuse an action for a value; see the follow-ups.

export const MEMBERS = Object.freeze([
  "read",
  "write",
  "docBytes",
  "validate",
  "apply",
  "size",
  "ops",
  "split",
  "assemble",
  "partOps",
  "targets",
  "creates",
  "needs",
  "removes",
  "has",
  "seen",
  "fits",
  "conflicts",
  "unseen",
  "review",
  "holds",
  "describe",
  "current",
  "preview",
]);

export const PART_FIELDS = Object.freeze([
  "k",
  "gen",
  "of",
  "from",
  "key",
  "order",
  "i",
  "m",
  "size",
  "frame",
]);

const encoder = new TextEncoder();

function rng(seed) {
  let state = seed >>> 0 || 1;
  return () => {
    state = (Math.imul(state, 1664525) + 1013904223) >>> 0;
    return state / 2 ** 32;
  };
}

function canonical(value) {
  if (Array.isArray(value)) return value.map(canonical);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.keys(value)
      .sort()
      .map((key) => [key, canonical(value[key])]),
  );
}

const same = (a, b) =>
  JSON.stringify(canonical(a)) === JSON.stringify(canonical(b));

function checkParts(codec, action, saw, reviewed, ops, opsPerFrame, fail) {
  const { head, parts } = codec.split(
    { action, saw, reviewed },
    { opsPerFrame },
  );
  const whole = codec.assemble(head, parts);
  if (
    !same(whole.action, action) ||
    !same(whole.saw, saw) ||
    !same(whole.reviewed ?? null, reviewed)
  )
    fail(
      "assemble(split(action)) is not the action, its saw and its reviewed saw",
    );
  if (
    Boolean(head.saw) !== Boolean(saw) ||
    Boolean(head.reviewed) !== Boolean(reviewed)
  )
    fail("split's head drops or invents a saw or reviewed saw");
  if (!parts.length || parts.some((part) => codec.partOps(part) > opsPerFrame))
    fail("a part is over the operation limit, or there are no parts");
  if (parts.reduce((sum, part) => sum + codec.partOps(part), 0) !== ops)
    fail("the parts' operations do not add up to the action's");
  if (parts.some((part) => PART_FIELDS.some((name) => name in part)))
    fail(
      `a part uses a field the core's part frame owns (${PART_FIELDS.join(", ")})`,
    );
}

// Runs the codec laws on `runs` samples from `sample(random)`, which returns
// { doc, record, actions }: a document the codec's own actions can reach, and actions on it.
// C1 to C10 are the co-editing plan's laws; C11 to C17 hold the members core uses to judge
// conflicts, removals, holds and previews. Each action's saw is taken on the sample's first
// document, so later actions in the list make it stale. Returns the failures; a codec with
// any failure may not be registered.
export function checkCodec(
  codec,
  sample,
  { runs = 200, seed = 1, limits = { opsPerFrame: 4 }, cap = 20 } = {},
) {
  const failures = [];
  if (
    !codec ||
    typeof codec.name !== "string" ||
    !Number.isInteger(codec.version)
  )
    failures.push({
      law: "shape",
      detail: "a codec needs a name and a whole-number version",
    });
  const missing = MEMBERS.filter((name) => typeof codec?.[name] !== "function");
  if (missing.length)
    failures.push({ law: "shape", detail: `missing ${missing.join(", ")}` });
  if (failures.length) return failures;
  const random = rng(seed);
  const fail = (law, run, detail) =>
    failures.length < cap && failures.push({ law, run, detail });
  for (let run = 0; run < runs && failures.length < cap; run++) {
    const { doc, record, actions } = sample(random);
    const bytes = codec.write(doc, record);
    if (typeof bytes !== "string") {
      fail("C2", run, "write must return a string");
      continue;
    }
    const back = codec.read(bytes);
    if (
      !same(back.doc, doc) ||
      !same(back.record, record) ||
      codec.write(back.doc, back.record) !== bytes
    )
      fail("C2", run, "read(write(doc)) is not the same document and record");
    const first = codec.read(bytes).doc;
    const now = codec.read(bytes).doc;
    for (const action of actions) {
      if (!codec.validate(action)) {
        fail(
          "validate",
          run,
          `a sample action is refused as malformed: ${JSON.stringify(action)}`,
        );
        continue;
      }
      try {
        const ids = [
          codec.targets(action),
          codec.creates(action),
          codec.needs(action),
          codec.removes(action),
        ];
        if (
          ids.some(
            (list) =>
              !Array.isArray(list) || list.some((id) => typeof id !== "string"),
          )
        )
          fail(
            "ids",
            run,
            "targets, creates, needs and removes must be lists of strings",
          );
        const before = codec.write(now, record);
        const prior = codec.read(before).doc;
        const fits = codec.fits(now, action);
        const own = codec.seen(now, action);
        const saw = codec.seen(first, action);
        if (fits && codec.conflicts(now, action, own).length)
          fail("C5", run, "an action conflicts with what its own author saw");
        codec.conflicts(now, action, null);
        codec.unseen(now, action, saw);
        if (
          fits &&
          !same(own, saw) &&
          !codec.conflicts(now, action, saw).length &&
          !codec.unseen(now, action, saw).length
        )
          fail(
            "C11",
            run,
            "a change since the author looked shows no conflict and no unseen value",
          );
        const cells = codec.conflicts(now, action, saw);
        if (
          fits &&
          cells.length &&
          codec.conflicts(now, action, codec.review(action, saw, cells)).length
        )
          fail("C12", run, "after review the same cells still conflict");
        if (codec.size(action) < encoder.encode(JSON.stringify(action)).length)
          fail("C9", run, "size is smaller than the action's bytes");
        const ops = codec.ops(action);
        if (ops < codec.targets(action).length)
          fail(
            "ops",
            run,
            "ops counts fewer operations than the action has targets",
          );
        if (!Number.isInteger(ops) || ops < 1)
          fail("ops", run, "ops must be a whole number of at least 1");
        for (const opsPerFrame of new Set([limits.opsPerFrame, 1]))
          if (ops > opsPerFrame)
            for (const reviewed of [null, own && structuredClone(own)])
              checkParts(
                codec,
                action,
                own,
                reviewed,
                ops,
                opsPerFrame,
                (detail) => fail("C6", run, detail),
              );
        const needed = codec.needs(action).every((id) => codec.has(now, id));
        const absent = codec.creates(action).every((id) => !codec.has(now, id));
        const held = codec.holds(now, action);
        const twin = structuredClone(now);
        const reloaded = codec.read(before).doc;
        const previewed = codec.write(
          codec.preview(codec.read(before).doc, [action]),
          record,
        );
        const result = codec.apply(now, action);
        const after = codec.write(now, record);
        const applied = result.out === "applied";
        if (
          codec.apply(twin, action).out !== result.out ||
          codec.write(twin, record) !== after
        )
          fail(
            "C1",
            run,
            "the same action on a copy of the same document gave a different result",
          );
        if (
          codec.apply(reloaded, action).out !== result.out ||
          codec.write(reloaded, record) !== after
        )
          fail(
            "C3",
            run,
            "applying after a write and a read gave a different document",
          );
        if (!applied && after !== before)
          fail("C4", run, "a refused action changed the document");
        if (fits !== applied)
          fail("C7", run, `fits said ${fits} but apply said ${result.out}`);
        if (needed && absent && !applied)
          fail(
            "C8",
            run,
            "an action whose needs are present and whose creations are absent was refused",
          );
        if (applied && !needed)
          fail(
            "C8",
            run,
            "an action was applied although something it needs was missing",
          );
        if (applied && !codec.creates(action).every((id) => codec.has(now, id)))
          fail(
            "C8",
            run,
            "an applied action left something it creates missing",
          );
        if (previewed !== after)
          fail("C13", run, "preview does not show what apply does");
        if (held && after !== before)
          fail(
            "C14",
            run,
            "holds said the document already shows the action, but apply changed it",
          );
        if (applied && codec.removes(action).some((id) => codec.has(now, id)))
          fail("C15", run, "an applied action left something it removes in place");
        if (
          applied &&
          after !== before &&
          !ids[0].length &&
          !ids[1].length &&
          !ids[3].length
        )
          fail(
            "C17",
            run,
            "an action that changed the document names no target, creation or removal",
          );
        if (applied && !same(codec.read(after).doc, now))
          fail(
            "C2",
            run,
            "an applied action left a document that does not survive write and read",
          );
        if (applied) {
          const undone = structuredClone(now);
          if (
            !codec.validate(result.inverse) ||
            codec.apply(undone, result.inverse).out !== "applied" ||
            codec.write(undone, record) !== codec.write(prior, record)
          )
            fail(
              "C10",
              run,
              "the inverse did not give back the earlier document",
            );
        }
      } catch (error) {
        fail("throws", run, `${error.message} on ${JSON.stringify(action)}`);
      }
    }
  }
  return failures;
}
