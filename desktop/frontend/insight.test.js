// Headless unit tests for the M30 analyses in the GUI (M36).
// Pure string logic — run from desktop/frontend with: node --test *.test.js
const test = require("node:test");
const assert = require("node:assert/strict");
const I = require("./dist/insight.js");

const report = {
  runs: 30,
  score: { value: 82.5, findings: 12, unstable_targets: 1, modules: { http: 50, certs: 100 }, worst_modules: ["http"] },
  digest: {
    Runs: 30,
    New: [{ Check: "http", Target: "web", From: "OK", To: "BAD" }],
    Resolved: [], Degraded: [], Improved: [],
    Flapping: ["redis api"],
  },
  clusters: [{ dimension: "host", value: "db-01", size: 3, targets: ["postgres db-01:5432", "redis db-01:6379", "tcp db-01:22"] }],
  flapping: [{ check: "redis", target: "api", score: 41, recent: 60, changes: 12, runs: 30, level: "medium" }],
  recovery: [
    { check: "http", target: "web", outages: 0, down: true, ongoing_seconds: 2820, mttr_seconds: 0 },
    { check: "redis", target: "api", outages: 3, down: false, mttr_seconds: 480, p50_seconds: 300, p90_seconds: 900 },
  ],
  budgets: [{ check: "http", target: "web", budget_remaining: 0.2, fast_burn: 8, slow_burn: 0.8, exhausted: "2026-08-05T00:00:00Z" }],
  forecasts: [
    { check: "redis", target: "api", latest: 82, in_days: 2.5, r2: 0.98, eta: "2026-08-07T00:00:00Z" },
    { check: "redis", target: "cache", latest: 10, note: "not trending toward the threshold" },
  ],
  anomalies: [{ check: "redis", target: "api", latest: 95, baseline: 30, ratio: 3.15, z: 48, deviating: true, unit: "ms" }],
};

test("flap badge shows the level, not the number", () => {
  const idx = I.flapIndex(report);
  const html = I.flapBadge(idx["redis\x1fapi"]);
  assert.match(html, /chip-flap flap-medium/);
  assert.match(html, />flapping</);
  // The score belongs in the tooltip: a chip reading "41" invites comparing it
  // to a "44" next to it, a difference the data does not support.
  assert.doesNotMatch(html, />41</);
  assert.match(html, /flappiness 41\/100/);
  assert.match(html, /getting worse/); // recent 60 vs score 41
});

test("a steady target gets no badge", () => {
  assert.equal(I.flapBadge(undefined), "");
  assert.equal(I.flapBadge({ level: "" }), "");
});

test("score tile bands and worst-module tooltip", () => {
  const html = I.scoreTile(report);
  assert.match(html, /tile health warn/); // 82.5 → warn band
  assert.match(html, /82\.5/);
  assert.match(html, /worst: http/);
  assert.equal(I.scoreTile({}), "", "no score means no tile, not an empty box");
});

test("score trend needs at least two points", () => {
  assert.equal(I.scoreTrend([90]), "");
  const html = I.scoreTrend([95, 80, 60]);
  assert.equal((html.match(/hbar/g) || []).length, 3);
  assert.match(html, /s-ok/);
  assert.match(html, /s-bad/);
});

test("clusters render as clickable rows carrying their index", () => {
  const html = I.clusterRows(report);
  assert.match(html, /data-cluster="0"/);
  assert.match(html, /<b>3<\/b> failures share the same host/);
  assert.equal(I.clusterRows({}), "");
  assert.deepEqual(I.clusterTargets(report, 0).length, 3);
  assert.deepEqual(I.clusterTargets(report, 9), []);
});

test("recovery line compares the outage to the usual", () => {
  const down = I.recoveryLine(report, "http", "web");
  assert.match(down, /down for 47m/);
  const up = I.recoveryLine(report, "redis", "api");
  assert.match(up, /3 outage\(s\), MTTR ~8m/);
  assert.equal(I.recoveryLine(report, "certs", "none"), "");
});

test("humanDuration reads the way an operator says it", () => {
  assert.equal(I.humanDuration(45), "45s");
  assert.equal(I.humanDuration(2820), "47m");
  assert.equal(I.humanDuration(3600), "1h");
  assert.equal(I.humanDuration(5400), "1h30m");
  assert.equal(I.humanDuration(90000), "1d1h");
});

test("digest renders groups and forwards as text", () => {
  const html = I.digestHTML(report);
  assert.match(html, /New/);
  assert.match(html, /OK → BAD/);
  assert.match(html, /Flapping/);
  const text = I.digestText(report);
  assert.match(text, /New:/);
  assert.match(text, /- http web: OK → BAD/);
});

test("an unchanged fleet says so in one line", () => {
  const quiet = { digest: { Runs: 12, New: [], Resolved: [], Degraded: [], Improved: [], Flapping: [] } };
  assert.match(I.digestHTML(quiet), /Nothing changed across the last 12 run\(s\)/);
  assert.doesNotMatch(I.digestHTML(quiet), /<h4/);
  assert.match(I.digestText(quiet), /^Nothing changed/);
});

test("budget card bands on what is left", () => {
  const html = I.budgetCard(report, "http", "web");
  assert.match(html, /20% left/);
  assert.match(html, /s-warn/);
  assert.match(html, /fast burn 8\.0x/);
  const note = I.budgetCard({ budgets: [{ check: "a", target: "b", note: "needs at least 10 runs" }] }, "a", "b");
  assert.match(note, /needs at least 10 runs/);
});

test("forecast shows the ETA, and says why when suppressed", () => {
  assert.match(I.forecastNote(report, "redis", "api"), /crosses in ~2\.5 days/);
  // A blank line would read as "no risk", so the reason is shown.
  assert.match(I.forecastNote(report, "redis", "cache"), /not trending toward the threshold/);
  assert.equal(I.forecastNote(report, "nope", "nope"), "");
});

test("anomaly note prefers the ratio wording", () => {
  // 3.15 is 3.14999… as a float64, so toFixed(1) is "3.1" — the assertion, not
  // the code, was wrong the first time.
  assert.match(I.anomalyNote(report, "redis", "api"), /3\.1x its norm of 30\.00ms/);
  assert.equal(I.anomalyNote(report, "nope", "nope"), "");
});

test("everything escapes hostile values", () => {
  const evil = {
    clusters: [{ dimension: "host", value: '<img src=x onerror="alert(1)">', size: 3, targets: [] }],
    flapping: [{ check: "a", target: "b", score: 50, recent: 50, changes: 1, runs: 10, level: '"><script>' }],
    digest: { Runs: 1, New: [{ Check: "<b>", Target: "<i>", From: "OK", To: "BAD" }], Resolved: [], Degraded: [], Improved: [], Flapping: [] },
  };
  assert.doesNotMatch(I.clusterRows(evil), /<img/);
  assert.doesNotMatch(I.flapBadge(I.flapIndex(evil)["a\x1fb"]), /<script>/);
  assert.doesNotMatch(I.digestHTML(evil), /<b><\/code>/);
  assert.match(I.digestHTML(evil), /&lt;b&gt;/);
});
