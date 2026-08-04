// Headless unit tests for the action log (CF-114).
// Pure functions, deterministic clock passed in — run from desktop/frontend with: node --test *.test.js
const test = require("node:test");
const assert = require("node:assert/strict");
const A = require("./dist/audit.js");

const T = Date.UTC(2026, 6, 26, 12, 0, 0); // fixed instant

test("add prepends newest-first and ignores an empty kind", () => {
  let log = [];
  log = A.add(log, "mute", "certs · a:443", "8h", T);
  log = A.add(log, "note", "http · svc", "Marco: on it", T + 1000);
  assert.equal(log.length, 2);
  assert.equal(log[0].kind, "note"); // newest first
  const same = A.add(log, "", "x", "y", T); // empty kind → no-op append
  assert.equal(same.length, 2);
});

test("add does not mutate its input and caps at MAX", () => {
  const base = [];
  const next = A.add(base, "issue", "t", "github", T);
  assert.equal(base.length, 0);
  assert.equal(next.length, 1);

  let big = [];
  for (let i = 0; i < A.MAX + 20; i++) big = A.add(big, "mute", "t" + i, "", T + i);
  assert.equal(big.length, A.MAX);
  // Newest kept, oldest dropped: the last-added target is at the head.
  assert.equal(big[0].target, "t" + (A.MAX + 19));
});

test("sanitize drops junk and coerces fields", () => {
  const dirty = [{ kind: "mute", target: 5, detail: null, at: "x" }, { target: "no kind" }, null];
  const clean = A.sanitize(dirty);
  assert.equal(clean.length, 1);
  assert.deepEqual(clean[0], { at: 0, kind: "mute", target: "", detail: "" });
});

test("toJSON round-trips the sanitized log", () => {
  const log = A.add([], "mute", "certs · a", "1h", T);
  const back = JSON.parse(A.toJSON(log));
  assert.equal(back[0].kind, "mute");
  assert.equal(back[0].at, T);
});

test("toMarkdown builds a table and escapes pipes/newlines", () => {
  const log = A.add([], "note", "http · svc", "line1|still\nline2", T);
  const md = A.toMarkdown(log);
  assert.match(md, /\| Time \(UTC\) \| Action \| Target \| Detail \|/);
  assert.match(md, /2026-07-26T12:00:00\.000Z/);
  assert.match(md, /line1\\\|still line2/); // pipe escaped, newline flattened
});
