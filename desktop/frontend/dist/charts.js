// charts.js — hand-rolled SVG charts for the dashboard (CF-91).
//
// Zero-dep and theme-aware: every mark is filled through a CSS class
// (.area-ok, .arc-warn, .band-BAD, …) that resolves to the same status
// variables the rest of the app uses, so light/dark switching is automatic and
// there are no hard-coded colors here. Pure functions build SVG strings — no
// DOM — so the geometry is unit-testable headlessly (see charts.test.js).
//
// UMD-ish: attaches to window.CFCharts in the browser and exports the same API
// under Node for the smoke test. Both contexts, one file.
(function (root, factory) {
  "use strict";
  const api = factory();
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else root.CFCharts = api;
})(typeof self !== "undefined" ? self : this, function () {
  "use strict";

  // Stacking order, bottom → top: healthy at the base, worst on top.
  const LAYERS = ["ok", "warn", "bad", "error"];

  function total(p) {
    return (p.ok || 0) + (p.warn || 0) + (p.bad || 0) + (p.error || 0);
  }

  // niceMax rounds a raw maximum up to a friendly gridline value (1, 2, 5, 10…).
  function niceMax(n) {
    if (n <= 5) return Math.max(1, Math.ceil(n));
    const pow = Math.pow(10, Math.floor(Math.log10(n)));
    for (const m of [1, 2, 2.5, 5, 10]) {
      if (m * pow >= n) return m * pow;
    }
    return 10 * pow;
  }

  // stackedPolys returns one filled polygon ([[x,y]…]) per status layer, stacked
  // from the baseline up. Single-point inputs collapse to a centered column.
  function stackedPolys(points, w, h, pad, maxN) {
    const n = points.length;
    const x = (i) => (n === 1 ? w / 2 : pad + (i * (w - 2 * pad)) / (n - 1));
    const y = (v) => h - pad - (v / maxN) * (h - 2 * pad);
    const polys = [];
    let below = points.map(() => 0);
    for (const status of LAYERS) {
      const upper = points.map((p, i) => below[i] + (p[status] || 0));
      const top = points.map((_, i) => [x(i), y(upper[i])]);
      const bottom = points.map((_, i) => [x(i), y(below[i])]).reverse();
      polys.push({ status, poly: top.concat(bottom) });
      below = upper;
    }
    return polys;
  }

  function fmt(v) { return (Math.round(v * 100) / 100).toString(); }
  function polyStr(poly) { return poly.map(([a, b]) => fmt(a) + "," + fmt(b)).join(" "); }

  // svgArea draws the stacked-area timeline of per-run status counts.
  function svgArea(points, opts) {
    opts = opts || {};
    const w = opts.w || 640, h = opts.h || 200, pad = opts.pad || 26;
    if (!points.length) return "";
    const maxN = niceMax(Math.max(1, ...points.map(total)));
    const y = (v) => h - pad - (v / maxN) * (h - 2 * pad);

    // Horizontal gridlines at 0, mid and max, with left-side labels.
    const ticks = [0, maxN / 2, maxN];
    const grid = ticks.map((t) => {
      const yy = fmt(y(t));
      return `<line class="grid" x1="${pad}" y1="${yy}" x2="${w - pad}" y2="${yy}"/>` +
        `<text class="axis" x="${pad - 6}" y="${fmt(y(t) + 3)}" text-anchor="end">${fmt(t)}</text>`;
    }).join("");

    const layers = points.length >= 2
      ? stackedPolys(points, w, h, pad, maxN)
        .map((l) => `<polygon class="area-${l.status}" points="${polyStr(l.poly)}"/>`).join("")
      : barColumn(points[0], w, h, pad, maxN);

    return svgWrap(w, h, "Status counts per run", grid + layers);
  }

  // barColumn renders a single run as one stacked bar (area needs ≥2 points).
  function barColumn(p, w, h, pad, maxN) {
    const y = (v) => h - pad - (v / maxN) * (h - 2 * pad);
    const bw = 46, cx = w / 2 - bw / 2;
    let acc = 0, out = "";
    for (const status of LAYERS) {
      const v = p[status] || 0;
      if (v <= 0) continue;
      const y1 = y(acc + v), y0 = y(acc);
      out += `<rect class="area-${status}" x="${fmt(cx)}" y="${fmt(y1)}" width="${bw}" height="${fmt(y0 - y1)}"/>`;
      acc += v;
    }
    return out;
  }

  function polar(cx, cy, r, deg) {
    const a = ((deg - 90) * Math.PI) / 180;
    return [cx + r * Math.cos(a), cy + r * Math.sin(a)];
  }

  // donutArcs returns one segment per non-zero status. A status at 100% is
  // flagged {ring:true} because a single SVG arc cannot span a full circle.
  function donutArcs(dist, cx, cy, r, ri) {
    const t = total(dist);
    const arcs = [];
    if (t <= 0) return arcs;
    let ang = 0;
    for (const status of LAYERS) {
      const v = dist[status] || 0;
      if (v <= 0) continue;
      const frac = v / t;
      if (frac >= 0.999999) { arcs.push({ status, frac, ring: true }); continue; }
      const sweep = frac * 360, a1 = ang + sweep;
      const [ox0, oy0] = polar(cx, cy, r, ang), [ox1, oy1] = polar(cx, cy, r, a1);
      const [ix1, iy1] = polar(cx, cy, ri, a1), [ix0, iy0] = polar(cx, cy, ri, ang);
      const large = sweep > 180 ? 1 : 0;
      const d = `M${fmt(ox0)} ${fmt(oy0)} A${r} ${r} 0 ${large} 1 ${fmt(ox1)} ${fmt(oy1)} ` +
        `L${fmt(ix1)} ${fmt(iy1)} A${ri} ${ri} 0 ${large} 0 ${fmt(ix0)} ${fmt(iy0)} Z`;
      arcs.push({ status, frac, d });
      ang = a1;
    }
    return arcs;
  }

  // svgDonut draws the current status distribution as a ring with a total in the
  // middle. `dist` uses ok/warn/bad/error counts.
  function svgDonut(dist, opts) {
    opts = opts || {};
    const s = opts.size || 168, cx = s / 2, cy = s / 2;
    const r = opts.r || s / 2 - 6, ri = opts.ri || r - 22;
    const t = total(dist);
    const arcs = donutArcs(dist, cx, cy, r, ri);
    const body = t === 0
      ? `<circle class="arc-empty" cx="${cx}" cy="${cy}" r="${(r + ri) / 2}" fill="none" stroke-width="${r - ri}"/>`
      : arcs.map((a) => a.ring
        ? `<circle class="arc-${a.status}" cx="${cx}" cy="${cy}" r="${(r + ri) / 2}" fill="none" stroke-width="${r - ri}"/>`
        : `<path class="arc-${a.status}" d="${a.d}"/>`).join("");
    const center =
      `<text class="donut-total" x="${cx}" y="${cy - 2}" text-anchor="middle">${t}</text>` +
      `<text class="donut-label" x="${cx}" y="${cy + 14}" text-anchor="middle">findings</text>`;
    return svgWrap(s, s, "Status distribution", body + center, "xMidYMid meet");
  }

  // svgBand draws the worst status of each run as a strip of segments, oldest to
  // newest — a compact timeline of overall fleet health.
  function svgBand(points, opts) {
    opts = opts || {};
    const w = opts.w || 640, h = opts.h || 26, gap = 2;
    const n = points.length;
    if (!n) return "";
    const seg = (w - (n - 1) * gap) / n;
    const rects = points.map((p, i) => {
      const worst = (p.worst || "OK").toUpperCase();
      const x = i * (seg + gap);
      return `<rect class="band-${worst}" x="${fmt(x)}" y="0" width="${fmt(seg)}" height="${h}" rx="2">` +
        `<title>${worst}</title></rect>`;
    }).join("");
    return svgWrap(w, h, "Worst status per run", rects);
  }

  function svgWrap(w, h, title, inner, par) {
    return `<svg class="chart" viewBox="0 0 ${w} ${h}" width="100%" preserveAspectRatio="${par || "none"}" ` +
      `role="img" aria-label="${title}"><title>${title}</title>${inner}</svg>`;
  }

  return { LAYERS, total, niceMax, stackedPolys, donutArcs, svgArea, svgDonut, svgBand };
});
