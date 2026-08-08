// insight.js — rendering for the M30 analyses in the GUI (M36).
//
// The numbers arrive already computed from the Go binding (App.Insight): this
// module only turns them into markup. No statistics here on purpose — a second
// implementation in JavaScript would drift from internal/insight within a
// release, which is the same failure CF-163 closed for the post-run pipeline.
//
// Pure string logic, no DOM, unit-testable headlessly (see insight.test.js).
// Same UMD wrapper as charts.js / presets.js / acks.js / notes.js / runbook.js.
(function (root, factory) {
  "use strict";
  const api = factory();
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else root.CFInsight = api;
})(typeof self !== "undefined" ? self : this, function () {
  "use strict";

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
  }

  const key = (check, target) => check + "\x1f" + target;

  // ---------------------------------------------------------------- flapping

  // flapIndex maps check\x1ftarget → the flapping row, for O(1) lookup while
  // rendering a table of hundreds of rows.
  function flapIndex(report) {
    const out = {};
    ((report && report.flapping) || []).forEach((f) => { out[key(f.check, f.target)] = f; });
    return out;
  }

  // flapBadge renders the chip for one finding, or "" when it is steady.
  // The title carries the numbers; the chip itself carries only the level,
  // because a chip reading "41" invites comparing it to a "44" next to it.
  function flapBadge(row) {
    if (!row || !row.level) return "";
    const dir = row.recent > row.score + 10 ? " · getting worse"
      : row.recent < row.score - 10 ? " · settling" : "";
    const title = `flappiness ${Math.round(row.score)}/100${dir} — ` +
      `${row.changes} status changes in ${row.runs} runs`;
    return `<span class="chip chip-flap flap-${escapeHtml(row.level)}" ` +
      `title="${escapeHtml(title)}">flapping</span>`;
  }

  // ------------------------------------------------------------------- score

  // scoreTile renders the fleet health tile. Returns "" without a score, so a
  // fleet with no history shows the normal tiles and no empty box.
  function scoreTile(report) {
    const s = report && report.score;
    if (!s) return "";
    const band = s.value >= 90 ? "ok" : s.value >= 70 ? "warn" : "bad";
    const worst = (s.worst_modules || []).slice(0, 3).join(", ");
    const title = worst ? `worst: ${worst}` : "every module is clean";
    return `<div class="tile health ${band}" title="${escapeHtml(title)}">` +
      `<b>${s.value.toFixed(1)}</b><span>health</span></div>`;
  }

  // scoreTrend renders a sparkline-ish bar strip of the index over past runs.
  // values is a list of numbers, oldest first.
  function scoreTrend(values) {
    if (!values || values.length < 2) return "";
    const bars = values.map((v) => {
      const band = v >= 90 ? "ok" : v >= 70 ? "warn" : "bad";
      const h = Math.max(4, Math.round(v));
      return `<i class="hbar s-${band}" style="height:${h}%" title="${v.toFixed(1)}"></i>`;
    }).join("");
    return `<div class="health-trend" aria-label="fleet health over time">${bars}</div>`;
  }

  // ---------------------------------------------------------------- clusters

  // clusterBar renders the blast-radius groups as clickable rows above the
  // table. Clicking filters instead of expanding: the rows are already below,
  // and duplicating them is the wall of text the cluster exists to replace.
  function clusterRows(report) {
    const cs = (report && report.clusters) || [];
    if (!cs.length) return "";
    const rows = cs.map((c, i) =>
      `<button class="cluster-row" data-cluster="${i}">` +
      `<b>${c.size}</b> failures share the same ${escapeHtml(c.dimension)}: ` +
      `<code>${escapeHtml(c.value)}</code></button>`).join("");
    return `<div class="cluster-bar">${rows}</div>`;
  }

  // clusterTargets returns the "check target" labels of one cluster, for
  // filtering the table down to it.
  function clusterTargets(report, index) {
    const c = ((report && report.clusters) || [])[index];
    return c ? c.targets.slice() : [];
  }

  // ---------------------------------------------------------------- recovery

  // recoveryLine renders the MTTR/ongoing-outage line for a finding's drawer,
  // or "" when the target has no outage history.
  function recoveryLine(report, check, target) {
    const r = ((report && report.recovery) || [])
      .find((x) => x.check === check && x.target === target);
    if (!r) return "";
    if (r.down) {
      const since = r.ongoing_seconds ? humanDuration(r.ongoing_seconds) : null;
      let text = since
        ? (r.started_before_window ? `down for at least ${since}` : `down for ${since}`)
        : "down";
      if (r.started_before_window && !since) text += " (started before the window)";
      if (r.outages > 0) text += `, usually back in ~${humanDuration(r.mttr_seconds)}`;
      return `<div class="kv"><span>Recovery</span><b>${escapeHtml(text)}</b></div>`;
    }
    if (!r.outages) return "";
    const text = `${r.outages} outage(s), MTTR ~${humanDuration(r.mttr_seconds)} ` +
      `(p50 ${humanDuration(r.p50_seconds)}, p90 ${humanDuration(r.p90_seconds)})`;
    return `<div class="kv"><span>Recovery</span><b>${escapeHtml(text)}</b></div>`;
  }

  // humanDuration renders seconds the way an operator says them.
  function humanDuration(seconds) {
    const s = Math.max(0, Math.round(seconds || 0));
    if (s < 60) return s + "s";
    const m = Math.round(s / 60);
    if (m < 60) return m + "m";
    const h = Math.floor(m / 60), rem = m % 60;
    if (h < 24) return rem ? `${h}h${rem}m` : `${h}h`;
    const d = Math.floor(h / 24);
    return `${d}d${h % 24 ? (h % 24) + "h" : ""}`;
  }

  // ------------------------------------------------------------------ digest

  // digestHTML renders the "what changed" drawer body.
  function digestHTML(report) {
    const d = report && report.digest;
    if (!d) return `<p class="drawer-msg">No history yet.</p>`;
    const groups = [
      ["New", d.New], ["Worse", d.Degraded], ["Improved", d.Improved], ["Resolved", d.Resolved],
    ].filter(([, list]) => list && list.length);

    if (!groups.length && !(d.Flapping || []).length) {
      return `<p class="drawer-msg">Nothing changed across the last ${d.Runs || 0} run(s).</p>`;
    }
    let html = `<p class="drawer-msg">Across the last ${d.Runs || 0} run(s).</p>`;
    groups.forEach(([title, list]) => {
      html += `<h4 class="digest-h">${title}</h4><ul class="digest-list">` +
        list.map((c) => `<li><code>${escapeHtml(c.Check)}</code> ${escapeHtml(c.Target)}` +
          ` <span class="muted">${escapeHtml(c.From)} → ${escapeHtml(c.To)}</span></li>`).join("") +
        `</ul>`;
    });
    if ((d.Flapping || []).length) {
      html += `<h4 class="digest-h">Flapping</h4><ul class="digest-list">` +
        d.Flapping.map((f) => `<li>${escapeHtml(f)}</li>`).join("") + `</ul>`;
    }
    return html;
  }

  // digestText renders the same thing as plain text, for the forward button.
  function digestText(report) {
    const d = report && report.digest;
    if (!d) return "";
    const groups = [
      ["New", d.New], ["Worse", d.Degraded], ["Improved", d.Improved], ["Resolved", d.Resolved],
    ].filter(([, list]) => list && list.length);
    if (!groups.length && !(d.Flapping || []).length) {
      return `Nothing changed across the last ${d.Runs || 0} run(s).`;
    }
    let out = `Across the last ${d.Runs || 0} run(s):\n`;
    groups.forEach(([title, list]) => {
      out += `\n${title}:\n` + list.map((c) => `  - ${c.Check} ${c.Target}: ${c.From} → ${c.To}`).join("\n") + "\n";
    });
    if ((d.Flapping || []).length) {
      out += `\nFlapping:\n` + d.Flapping.map((f) => `  - ${f}`).join("\n") + "\n";
    }
    return out;
  }

  // ------------------------------------------------------------------ budget

  // budgetCard renders the SLO error budget for one target, or "" when the
  // history was too short to compute one.
  function budgetCard(report, check, target) {
    const b = ((report && report.budgets) || [])
      .find((x) => x.check === check && x.target === target);
    if (!b) return "";
    if (b.note) {
      return `<div class="kv"><span>Error budget</span><b class="muted">${escapeHtml(b.note)}</b></div>`;
    }
    const left = Math.round(b.budget_remaining * 100);
    const band = left <= 0 ? "bad" : left < 25 ? "warn" : "ok";
    let text = `${left}% left`;
    if (b.exhausted) text += ` · fast burn ${b.fast_burn.toFixed(1)}x`;
    else text += ` · burn ${b.slow_burn.toFixed(2)}x`;
    return `<div class="kv"><span>Error budget</span>` +
      `<b class="budget s-${band}">${escapeHtml(text)}</b></div>`;
  }

  // ---------------------------------------------------------------- forecast

  // forecastNote renders the ETA annotation for a metric drawer, or "" when
  // there is no projection worth showing. The suppressed cases carry their
  // reason: a blank line reads as "no risk".
  function forecastNote(report, check, target) {
    const f = ((report && report.forecasts) || [])
      .find((x) => x.check === check && x.target === target);
    if (!f) return "";
    if (f.eta) {
      const days = f.in_days.toFixed(1);
      return `<div class="kv"><span>Forecast</span>` +
        `<b>crosses in ~${escapeHtml(days)} days (R²=${f.r2.toFixed(2)})</b></div>`;
    }
    if (!f.note) return "";
    return `<div class="kv"><span>Forecast</span><b class="muted">${escapeHtml(f.note)}</b></div>`;
  }

  // anomalyNote renders how far the latest sample sits from its own baseline.
  function anomalyNote(report, check, target) {
    const a = ((report && report.anomalies) || [])
      .find((x) => x.check === check && x.target === target);
    if (!a) return "";
    if (a.note) {
      return `<div class="kv"><span>Baseline</span><b class="muted">${escapeHtml(a.note)}</b></div>`;
    }
    const unit = a.unit || "";
    const text = a.deviating
      ? (a.ratio ? `${a.ratio.toFixed(1)}x its norm of ${a.baseline.toFixed(2)}${unit}`
        : `off its norm of ${a.baseline.toFixed(2)}${unit}`)
      : `normal (baseline ${a.baseline.toFixed(2)}${unit})`;
    return `<div class="kv"><span>Baseline</span>` +
      `<b class="${a.deviating ? "s-warn" : ""}">${escapeHtml(text)}</b></div>`;
  }

  return {
    escapeHtml, humanDuration,
    flapIndex, flapBadge,
    scoreTile, scoreTrend,
    clusterRows, clusterTargets,
    recoveryLine, digestHTML, digestText,
    budgetCard, forecastNote, anomalyNote,
  };
});
