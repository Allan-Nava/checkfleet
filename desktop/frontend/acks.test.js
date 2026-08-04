// Headless unit tests for finding acks/mutes (CF-110).
// Pure functions, deterministic clock passed in — run from desktop/frontend with: node --test *.test.js
const test = require("node:test");
const assert = require("node:assert/strict");
const A = require("./dist/acks.js");

const NOW = 1_000_000_000_000; // a fixed "now" in ms
const HOUR = 3600 * 1000;

test("key joins config#check#target stably", () => {
  const k = A.key("/etc/checkfleet.yml", "certs", "example.com:443");
  assert.equal(k, A.key("/etc/checkfleet.yml", "certs", "example.com:443"));
  assert.notEqual(k, A.key("/etc/other.yml", "certs", "example.com:443"));
});

test("durationUntil maps snooze choices to absolute expiries", () => {
  assert.equal(A.durationUntil("1h", NOW), NOW + HOUR);
  assert.equal(A.durationUntil("8h", NOW), NOW + 8 * HOUR);
  assert.equal(A.durationUntil("24h", NOW), NOW + 24 * HOUR);
  assert.equal(A.durationUntil("recovery", NOW), A.UNTIL_RECOVERY);
  assert.equal(A.durationUntil("nonsense", NOW), NOW + HOUR); // safe default
});

test("isMuted respects timed and until-recovery mutes", () => {
  const k = A.key("c", "http", "t");
  let acks = A.mute({}, { key: k, until: NOW + HOUR, at: NOW });
  assert.equal(A.isMuted(acks, k, NOW), true);
  assert.equal(A.isMuted(acks, k, NOW + 2 * HOUR), false); // expired
  assert.equal(A.isMuted(acks, "other", NOW), false);

  const rec = A.mute({}, { key: k, until: A.UNTIL_RECOVERY, at: NOW });
  assert.equal(A.isMuted(rec, k, NOW + 999 * HOUR), true); // never expires by time
});

test("mute/unmute are immutable upserts", () => {
  const k = A.key("c", "http", "t");
  const base = {};
  const one = A.mute(base, { key: k, until: NOW + HOUR, at: NOW });
  assert.deepEqual(base, {}, "input not mutated");
  assert.equal(Object.keys(one).length, 1);
  const gone = A.unmute(one, k);
  assert.equal(Object.keys(gone).length, 0);
  assert.equal(Object.keys(one).length, 1, "unmute did not mutate its input");
});

test("prune drops expired timed mutes, keeps until-recovery", () => {
  const acks = {
    a: { key: "a", until: NOW - HOUR, at: NOW },       // expired
    b: { key: "b", until: NOW + HOUR, at: NOW },       // live
    c: { key: "c", until: A.UNTIL_RECOVERY, at: NOW }, // until recovery
  };
  const kept = A.prune(acks, NOW);
  assert.deepEqual(Object.keys(kept).sort(), ["b", "c"]);
});

test("activeCount counts only live mutes", () => {
  const acks = {
    a: { key: "a", until: NOW - HOUR, at: NOW },
    b: { key: "b", until: NOW + HOUR, at: NOW },
    c: { key: "c", until: A.UNTIL_RECOVERY, at: NOW },
  };
  assert.equal(A.activeCount(acks, NOW), 2);
});

test("describe renders a human label", () => {
  assert.equal(A.describe({ key: "k", until: A.UNTIL_RECOVERY }, NOW), "muted until recovery");
  assert.equal(A.describe({ key: "k", until: NOW + 30 * 60000 }, NOW), "muted · 30m left");
  assert.equal(A.describe({ key: "k", until: NOW + 5 * HOUR }, NOW), "muted · 5h left");
  assert.equal(A.describe({ key: "k", until: NOW - 60000 }, NOW), "mute expired");
  assert.equal(A.describe(null, NOW), "");
});

test("normalize rejects a keyless record and coerces fields", () => {
  assert.equal(A.normalize({ until: 5 }), null);
  const r = A.normalize({ key: "k", until: "x", reason: 7, at: NOW });
  assert.equal(r.until, 0);
  assert.equal(r.reason, "");
  assert.equal(r.at, NOW);
});
