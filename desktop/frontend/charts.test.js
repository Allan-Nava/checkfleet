// Headless smoke test for the dashboard chart geometry (CF-91).
// Pure functions, no DOM — run with: node --test desktop/frontend/
const test = require("node:test");
const assert = require("node:assert/strict");
const C = require("./dist/charts.js");

const P = (ok, warn, bad, error, worst) => ({ ok, warn, bad, error, worst });

test("niceMax rounds up to friendly gridlines", () => {
  assert.equal(C.niceMax(0), 1);
  assert.equal(C.niceMax(3), 3);
  assert.equal(C.niceMax(7), 10);
  assert.equal(C.niceMax(23), 25);
  assert.equal(C.niceMax(140), 200);
});

test("total sums the four counts", () => {
  assert.equal(C.total(P(14, 3, 2, 1)), 20);
  assert.equal(C.total({}), 0);
});

test("stackedPolys returns one polygon per layer, stacked to the full total", () => {
  const pts = [P(10, 0, 0, 0), P(6, 2, 1, 1)];
  const polys = C.stackedPolys(pts, 200, 100, 10, 10);
  assert.equal(polys.length, 4);
  assert.deepEqual(polys.map((l) => l.status), ["ok", "warn", "bad", "error"]);
  // Every polygon is a closed band: 2 points per input on top + 2 on the bottom.
  for (const l of polys) assert.equal(l.poly.length, pts.length * 2);
  // All coordinates are finite (no NaN leaking into the SVG).
  for (const l of polys) for (const [x, y] of l.poly) {
    assert.ok(Number.isFinite(x) && Number.isFinite(y), `finite ${l.status}`);
  }
});

test("donutArcs fractions sum to 1 and stay in order", () => {
  const arcs = C.donutArcs(P(14, 3, 2, 1), 84, 84, 78, 56);
  assert.equal(arcs.length, 4);
  const sum = arcs.reduce((a, x) => a + x.frac, 0);
  assert.ok(Math.abs(sum - 1) < 1e-9, `fractions sum to 1, got ${sum}`);
  for (const a of arcs) assert.match(a.d, /^M[\d.]+ [\d.-]+ A/);
});

test("donutArcs flags a 100% status as a full ring (arc can't span 360)", () => {
  const arcs = C.donutArcs(P(5, 0, 0, 0), 84, 84, 78, 56);
  assert.equal(arcs.length, 1);
  assert.equal(arcs[0].ring, true);
  assert.equal(arcs[0].status, "ok");
});

test("donutArcs on an all-zero distribution is empty", () => {
  assert.deepEqual(C.donutArcs(P(0, 0, 0, 0), 84, 84, 78, 56), []);
});

test("svg builders emit themed classes and no NaN", () => {
  const pts = [P(10, 0, 0, 0, "OK"), P(8, 2, 0, 0, "WARN"), P(6, 1, 2, 1, "ERROR")];
  const area = C.svgArea(pts, {});
  const donut = C.svgDonut(P(6, 1, 2, 1), {});
  const band = C.svgBand(pts, {});
  for (const svg of [area, donut, band]) {
    assert.match(svg, /^<svg class="chart"/);
    assert.doesNotMatch(svg, /NaN/);
  }
  assert.match(area, /area-error/);
  assert.match(donut, /arc-bad/);
  assert.match(band, /band-ERROR/);
});

test("svgArea degrades to a single stacked bar for one run", () => {
  const svg = C.svgArea([P(3, 1, 0, 0, "WARN")], {});
  assert.match(svg, /<rect class="area-ok"/);
  assert.doesNotMatch(svg, /NaN/);
});

test("svgArea on empty input is a no-op", () => {
  assert.equal(C.svgArea([], {}), "");
  assert.equal(C.svgBand([], {}), "");
});
