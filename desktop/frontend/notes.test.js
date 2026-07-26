// Headless unit tests for operator notes (CF-112).
// Pure functions, no DOM — run with: node --test desktop/frontend/
const test = require("node:test");
const assert = require("node:assert/strict");
const N = require("./dist/notes.js");

test("normalize trims and rejects an empty note", () => {
  assert.equal(N.normalize({ key: "k", owner: "  ", text: "   " }), null);
  assert.equal(N.normalize({ owner: "x", text: "y" }), null); // no key
  const r = N.normalize({ key: "k", owner: "  Marco ", text: "  rotating cert ", at: 5 });
  assert.deepEqual(r, { key: "k", owner: "Marco", text: "rotating cert", at: 5 });
});

test("set upserts, and an empty note deletes the key", () => {
  const base = {};
  const one = N.set(base, { key: "k", owner: "Marco", text: "on it", at: 1 });
  assert.deepEqual(base, {}, "input not mutated");
  assert.equal(N.count(one), 1);
  // Re-setting the same key with an empty note removes it.
  const gone = N.set(one, { key: "k", owner: "", text: "" });
  assert.equal(N.count(gone), 0);
  assert.equal(N.count(one), 1, "prior object not mutated");
});

test("remove/get/has behave", () => {
  const notes = N.set({}, { key: "k", owner: "Ada", text: "note" });
  assert.equal(N.has(notes, "k"), true);
  assert.equal(N.get(notes, "k").owner, "Ada");
  const out = N.remove(notes, "k");
  assert.equal(N.has(out, "k"), false);
  assert.equal(N.get(out, "missing"), null);
});

test("describe formats owner + text", () => {
  assert.equal(N.describe({ key: "k", owner: "Marco", text: "rotating" }), "Marco: rotating");
  assert.equal(N.describe({ key: "k", owner: "", text: "just text" }), "just text");
  assert.equal(N.describe({ key: "k", owner: "Ada", text: "" }), "Ada");
  assert.equal(N.describe(null), "");
});

test("sanitize drops junk records", () => {
  const dirty = { a: { key: "a", owner: "x", text: "y" }, b: { key: "b" }, c: null };
  const clean = N.sanitize(dirty);
  assert.deepEqual(Object.keys(clean), ["a"]);
});
