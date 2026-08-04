// Headless unit tests for the saved-views logic (CF-108).
// Pure functions, no DOM — run from desktop/frontend with: node --test *.test.js
const test = require("node:test");
const assert = require("node:assert/strict");
const P = require("./dist/presets.js");

const state = (o) => Object.assign(
  { stack: "", filter: "", minsev: "ok", group: false, view: "fleet" }, o);

test("normalize trims the name and coerces the knobs", () => {
  const p = P.normalize("  Prod edge  ", state({ minsev: "bad", group: 1, view: "dashboard", stack: "prod" }));
  assert.equal(p.name, "Prod edge");
  assert.equal(p.stack, "prod");
  assert.equal(p.minsev, "bad");
  assert.equal(p.group, true); // truthy → real boolean
  assert.equal(p.view, "dashboard");
});

test("normalize rejects an empty name and defaults bad enums", () => {
  assert.equal(P.normalize("   ", state()), null);
  const p = P.normalize("x", { minsev: "nonsense", view: "wat", stack: 42 });
  assert.equal(p.minsev, "ok");
  assert.equal(p.view, "fleet");
  assert.equal(p.stack, ""); // non-string dropped
});

test("upsert appends new and replaces same-name case-insensitively", () => {
  let list = [];
  list = P.upsert(list, P.normalize("prod", state({ filter: "a" })));
  list = P.upsert(list, P.normalize("staging", state()));
  assert.equal(list.length, 2);
  // "PROD" is the same view as "prod" — replace in place, don't grow the list.
  list = P.upsert(list, P.normalize("PROD", state({ filter: "b" })));
  assert.equal(list.length, 2);
  assert.equal(P.find(list, "prod").filter, "b");
  assert.equal(list[0].name, "PROD"); // replaced in place, order kept
});

test("upsert does not mutate the input array", () => {
  const orig = [P.normalize("a", state())];
  const next = P.upsert(orig, P.normalize("b", state()));
  assert.equal(orig.length, 1);
  assert.equal(next.length, 2);
});

test("remove drops by name, case-insensitively", () => {
  const list = [P.normalize("Prod", state()), P.normalize("lab", state())];
  const out = P.remove(list, "PROD");
  assert.equal(out.length, 1);
  assert.equal(out[0].name, "lab");
});

test("matches lights the active chip only on a full-state match", () => {
  const p = P.normalize("v", state({ stack: "s", filter: "f", minsev: "warn", group: true, view: "config" }));
  assert.equal(P.matches(p, state({ stack: "s", filter: "f", minsev: "warn", group: true, view: "config" })), true);
  assert.equal(P.matches(p, state({ stack: "s", filter: "f", minsev: "warn", group: false, view: "config" })), false);
  assert.equal(P.matches(null, state()), false);
});

test("serialize/parse round-trips and dedupes", () => {
  const list = [P.normalize("a", state({ filter: "x" })), P.normalize("b", state())];
  const round = P.parse(P.serialize(list));
  assert.equal(round.length, 2);
  assert.equal(round[0].filter, "x");
});

test("parse is tolerant: junk, envelope form, and duplicates", () => {
  assert.deepEqual(P.parse("not json"), []);
  assert.deepEqual(P.parse("null"), []);
  const env = P.parse(JSON.stringify({ presets: [{ name: "a" }, { name: "A" }, { name: "" }] }));
  assert.equal(env.length, 1); // "A" dedupes "a", the nameless one is dropped
  assert.equal(env[0].name, "a");
});

test("sanitize caps the set at MAX", () => {
  const many = Array.from({ length: P.MAX + 5 }, (_, i) => ({ name: "v" + i }));
  assert.equal(P.sanitize(many).length, P.MAX);
});
