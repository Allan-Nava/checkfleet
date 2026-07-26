// checkfleet desktop frontend.
//
// It talks to the Go backend through window.go.main.App (injected by Wails).
// When those bindings are absent — e.g. opened directly in a browser for a
// design preview — it falls back to a realistic mock so the UI is fully
// navigable. Same file, both contexts.
(function () {
  "use strict";

  const RANK = { OK: 0, WARN: 1, BAD: 2, ERROR: 3 };
  const $ = (id) => document.getElementById(id);

  /* ---------------- backend ---------------- */
  const wails = window.go && window.go.main && window.go.main.App;
  const IS_MOCK = !wails;

  const Backend = wails || {
    Version: async () => "preview",
    DefaultConfigPath: async () => "/home/ops/checkfleet.yml",
    StartupConfig: async () => ({ path: "/home/ops/checkfleet.yml", autoRun: false }),
    ListStacks: async () => ["prod", "edge", "staging"],
    OpenConfigDialog: async () => "/home/ops/checkfleet.yml",
    SaveReport: async (fmt) => "~/checkfleet-report." + (fmt === "json" ? "json" : "md"),
    RunChecks: async () => mockReport(),
    Validate: async () => [],
    Explain: async (m) => "Sample explanation for the " + m + " module (preview).",
    Notify: async () => {},
    ReadConfig: async () => "timeout_seconds: 30\nchecks:\n  http:\n    targets:\n      - url: https://example.com/health\n        expect_status: 200\n        max_latency_ms: 2000\n  certs:\n    warn_days: 30\n    crit_days: 7\n    targets: [example.com:443]\n",
    SaveConfig: async () => {},
    ValidateText: async () => [],
    AddEndpoint: async (yaml, kind, value, recordType, extra, expectStatus) => {
      // Preview-only textual merge (the real backend edits the YAML node tree).
      const nl = "\n";
      const scalar = (k) => "  " + k + ":" + nl + "    targets: [" + value + "]" + nl;
      const map = (k, kv) => "  " + k + ":" + nl + "    targets:" + nl + "      - " + kv + nl;
      let block;
      switch (kind) {
        case "certs": case "tls": case "redis": case "nats": block = scalar(kind); break;
        case "tcp": case "smtp": block = map(kind, "address: " + value); break;
        case "dns": block = map("dns", "name: " + value + (recordType && recordType !== "A" ? nl + "        type: " + recordType : "")); break;
        case "grpc": block = map("grpc", "address: " + value + (extra ? nl + "        service: " + extra : "")); break;
        case "postgres": block = map("postgres", "dsn: " + value + (extra ? nl + "        password_env: " + extra : "")); break;
        default: block = map("http", "url: " + value + (expectStatus ? nl + "        expect_status: " + expectStatus : "")); break;
      }
      const base = yaml && yaml.trim() ? yaml.replace(/\s*$/, "\n") : "checks:\n";
      return (base.includes("checks:") ? base : base + "checks:\n") + block;
    },
    Trend: async () => {
      const kinds = ["OK", "OK", "WARN", "OK", "WARN", "BAD", "WARN", "OK", "OK", "ERROR", "WARN", "OK"];
      const base = Math.floor(Date.now() / 1000) - kinds.length * 3600;
      return kinds.map((w, i) => ({
        unix: base + i * 3600, worst: w,
        ok: w === "OK" ? 14 : 11, warn: w === "WARN" ? 3 : 0,
        bad: w === "BAD" ? 2 : 0, error: w === "ERROR" ? 1 : 0,
      }));
    },
    TrendByModule: async () => {
      const kinds = ["OK", "OK", "WARN", "OK", "WARN", "BAD", "WARN", "OK", "OK", "ERROR", "WARN", "OK"];
      const base = Math.floor(Date.now() / 1000) - kinds.length * 3600;
      const mods = ["certs", "http", "nats", "redis"];
      return {
        modules: mods,
        runs: kinds.map((w, i) => ({
          unix: base + i * 3600,
          worst: {
            certs: "OK",
            http: i % 3 === 0 ? w : "OK",
            nats: w === "ERROR" ? "ERROR" : "OK",
            redis: w === "WARN" ? "WARN" : "OK",
          },
        })),
      };
    },
    Availability: async () => {
      const now = Math.floor(Date.now() / 1000);
      return {
        runs: 12, fromUnix: now - 12 * 3600, toUnix: now, okRuns: 7, uptime: 58.3,
        currentWorst: "OK", currentSinceUnix: now - 2 * 3600,
        targets: [
          { check: "postgres", target: "pg-01:5432", runs: 12, okRuns: 9, uptime: 75, last: "ERROR" },
          { check: "redis", target: "redis-cache-01:6379", runs: 12, okRuns: 10, uptime: 83.3, last: "WARN" },
          { check: "stream", target: "live.example.com", runs: 12, okRuns: 11, uptime: 91.7, last: "OK" },
          { check: "certs", target: "api.example.com:443", runs: 12, okRuns: 12, uptime: 100, last: "OK" },
        ],
      };
    },
    Metrics: async () => {
      const now = Math.floor(Date.now() / 1000);
      const mk = (base, amp) => Array.from({ length: 12 }, (_, i) =>
        ({ unix: now - (12 - i) * 3600, value: Math.round((base + Math.sin(i / 1.7) * amp) * 10) / 10 }));
      return [
        { check: "http", target: "https://example.com/health", unit: "ms", points: mk(140, 45) },
        { check: "certs", target: "api.example.com:443", unit: "days", points: mk(30, 4) },
        { check: "tcp", target: "db.internal:5432", unit: "ms", points: mk(12, 7) },
      ];
    },
    HistoryRuns: async () => {
      const kinds = ["ERROR", "WARN", "BAD", "OK", "WARN", "OK", "OK"];
      const now = Math.floor(Date.now() / 1000);
      return kinds.map((w, i) => ({
        unix: now - i * 3600, worst: w, total: 15,
        ok: w === "OK" ? 15 : 11, warn: w === "WARN" ? 3 : 0,
        bad: w === "BAD" ? 2 : 0, error: w === "ERROR" ? 1 : 0,
      }));
    },
    RunAt: async () => ([
      { check: "postgres", target: "pg-01:5432", status: "ERROR", value: null, unit: "" },
      { check: "certs", target: "api.example.com:443", status: "WARN", value: 12, unit: "days" },
      { check: "http", target: "https://example.com/health", status: "OK", value: 142, unit: "ms" },
    ]),
    DiffRuns: async () => ([
      { check: "postgres", target: "pg-01:5432", from: "OK", to: "ERROR", kind: "new" },
      { check: "redis", target: "redis-cache-01:6379", from: "WARN", to: "OK", kind: "resolved" },
    ]),
    Send: async (target) => ({ ok: true, target, message: "sent to " + target }),
    WorkspaceStatus: async (paths) => (paths || []).map((p, i) => {
      const w = ["OK", "WARN", "BAD", "OK", "ERROR"][i % 5];
      return { path: p, worst: w, ok: 12, warn: w === "WARN" ? 3 : 0, bad: w === "BAD" ? 2 : 0, error: w === "ERROR" ? 1 : 0, err: "" };
    }),
    ScheduleSnippet: async (path, interval) =>
      "# cron — run every 5 min:\n*/5 * * * * checkfleet check all --config " +
      (path || "checkfleet.yml") + " --exit-on-bad\n\n" +
      "# or run continuously as a Prometheus exporter:\ncheckfleet serve --config " +
      (path || "checkfleet.yml") + " --interval " + (interval || "60s") + " --listen :9876",
    StartMonitor: async () => {},
    StopMonitor: async () => {},
    MonitorRunning: async () => false,
  };

  // Wails event bus (CF-109). In the browser preview there is no runtime, so the
  // subscription is a no-op and the monitor falls back to a JS interval.
  const Events = (window.runtime && window.runtime.EventsOn)
    ? window.runtime
    : { EventsOn: () => {} };

  /* ---------------- state ---------------- */
  let report = null;
  let timer = null;
  let visibleFindings = [];
  let editorOn = false;
  let dashboardOn = false;
  let view = "fleet";
  let moduleTrend = { modules: [], runs: [] };
  let metricSeries = [];
  let running = false;
  let historyRuns = [];
  let workspacePaths = [];
  let wsStatuses = {};

  /* ---------------- rendering ---------------- */
  function severityAllowed(status, min) {
    return RANK[status] >= RANK[min.toUpperCase()];
  }

  // sparkFor returns a tiny inline trend for a finding's target when history has
  // a numeric series for it (from the last Metrics read); otherwise "".
  function sparkFor(f) {
    const s = metricSeries.find((x) => x.check === f.check && x.target === f.target);
    if (!s || !s.points || s.points.length < 2) return "";
    return `<span class="spark-wrap" title="${escapeHtml(f.check + " " + f.target + (s.unit ? " (" + s.unit + ")" : ""))}">` +
      CFCharts.svgSparkline(s.points, {}) + `</span>`;
  }

  // setBusy toggles the top progress bar + the Run button spinner during work.
  function setBusy(on) {
    $("progress").hidden = !on;
    const btn = $("run");
    btn.disabled = on;
    btn.classList.toggle("loading", on);
    const label = btn.querySelector(".run-label");
    if (label) label.textContent = on ? "Running…" : "Run";
  }

  // emptyState paints the table's empty area for each situation (loading / first
  // run / no config error / no filter match). Buttons are wired via delegation.
  function emptyState(kind, msg) {
    const el = $("empty");
    el.style.display = "flex";
    if (kind === "loading") {
      el.innerHTML = `<div class="spinner spinner-lg"></div><p>Running checks…</p>`;
    } else if (kind === "press-run") {
      el.innerHTML = `<img src="assets/logo.svg" alt="" width="64" height="64">
        <p>Press <b>Run</b> to check the fleet.</p>
        <p class="empty-hint"><kbd>⌘↵</kbd> run · <kbd>⌘K</kbd> commands · <kbd>/</kbd> filter</p>`;
    } else if (kind === "no-match") {
      el.innerHTML = `<p class="empty-quiet">No findings match these filters.</p>
        <button class="btn" data-empty-act="clear">Clear filters</button>`;
    } else if (kind === "error") {
      el.innerHTML = `<div class="err-card">
        <div class="err-ico">!</div>
        <div class="err-body">
          <b>Couldn't run the checks</b>
          <p class="err-msg">${escapeHtml(msg)}</p>
          <div class="err-actions">
            <button class="btn btn-primary" data-empty-act="retry">Retry</button>
            <button class="btn" data-empty-act="edit">Open config editor</button>
          </div>
        </div></div>`;
    }
  }

  function render() {
    const summary = $("summary");
    const rows = $("rows");

    // Running with no usable data yet → loading state. (On an auto-refresh with a
    // valid previous report we keep showing it, with the top progress bar instead.)
    if (running && (!report || report.err)) {
      summary.hidden = true; rows.innerHTML = ""; emptyState("loading"); return;
    }
    if (!report) {
      summary.hidden = true; rows.innerHTML = ""; emptyState("press-run"); return;
    }
    if (report.err) {
      summary.hidden = true; rows.innerHTML = ""; emptyState("error", report.err);
      setStatus("configuration error"); return;
    }

    const findings = report.findings || [];

    // Auto-clear "until recovery" mutes whose target is green again (CF-111):
    // the snooze has served its purpose, so the finding counts once more.
    let cleared = false;
    findings.forEach((f) => {
      if (f.status !== "OK") return;
      const rec = acks[ackKey(f)];
      if (rec && rec.until === CFAcks.UNTIL_RECOVERY) { acks = CFAcks.unmute(acks, ackKey(f)); cleared = true; }
    });
    if (cleared) { saveAcks(); syncMutedKeys(); }

    // summary (kept hidden while the editor/dashboard views own the screen)
    summary.hidden = editorOn || dashboardOn;
    // The worst pill respects mutes: a snoozed problem doesn't dominate the
    // headline. Raw counts below stay raw; the status bar carries "N muted".
    const worst = worstOf(findings.filter((f) => !findingMuted(f)).map((f) => f.status));
    const worstEl = $("worst");
    worstEl.className = "worst s-" + worst;
    $("worstLabel").textContent = worst;
    $("cOK").textContent = report.ok;
    $("cWARN").textContent = report.warn;
    $("cBAD").textContent = report.bad;
    $("cERROR").textContent = report.error;
    $("mTotal").textContent = findings.length;
    $("mDur").textContent = report.durationMs != null ? report.durationMs + " ms" : "—";
    $("mStarted").textContent = report.started ? new Date(report.started).toLocaleTimeString() : "—";
    $("mModules").innerHTML = (report.modules || [])
      .map((m) => `<span class="chip" role="button" tabindex="0" data-mod="${escapeHtml(m)}" title="Explain ${escapeHtml(m)}">${escapeHtml(m)}</span>`).join("");

    // table (preserve backend worst-first order)
    const q = $("filter").value.trim().toLowerCase();
    const min = $("minsev").value;
    const hideMuted = $("hideMuted") && $("hideMuted").checked;
    const visible = findings.filter((f) => {
      if (!severityAllowed(f.status, min)) return false;
      if (hideMuted && findingMuted(f)) return false;
      if (!q) return true;
      return (f.check + " " + f.target + " " + f.message).toLowerCase().includes(q);
    });
    visibleFindings = visible;

    const findingRow = (f, i) => {
      const muted = findingMuted(f);
      const note = findingNote(f);
      const chip = muted
        ? `<span class="chip chip-muted" title="${escapeHtml(CFAcks.describe(acks[ackKey(f)], Date.now()))}">muted</span>`
        : "";
      const noteChip = note
        ? `<span class="chip chip-note" title="${escapeHtml(CFNotes.describe(note))}">note</span>`
        : "";
      return `
      <tr data-i="${i}"${$("groupBy").checked ? ` data-grp="${escapeHtml(f.check)}"` : ""}${muted ? ` class="row-muted"` : ""}>
        <td><span class="badge ${f.status}">${f.status}</span>${chip}${noteChip}</td>
        <td class="cell-check">${escapeHtml(f.check)}</td>
        <td class="cell-target">${escapeHtml(f.target)}</td>
        <td class="cell-trend">${sparkFor(f)}</td>
        <td class="cell-msg">${escapeHtml(f.message)}</td>
      </tr>`;
    };

    if ($("groupBy").checked) {
      const order = [];
      const groups = {};
      visible.forEach((f, i) => {
        if (!groups[f.check]) { groups[f.check] = []; order.push(f.check); }
        groups[f.check].push([f, i]);
      });
      rows.innerHTML = order.map((mod) => {
        const items = groups[mod];
        const worst = worstOf(items.map(([f]) => f.status));
        const header = `
          <tr class="group-row" data-group="${escapeHtml(mod)}">
            <td colspan="5">
              <span class="group-caret">▾</span>
              <span class="badge ${worst}">${worst}</span>
              <b>${escapeHtml(mod)}</b>
              <span class="group-count">${items.length}</span>
            </td>
          </tr>`;
        return header + items.map(([f, i]) => findingRow(f, i)).join("");
      }).join("");
    } else {
      rows.innerHTML = visible.map(findingRow).join("");
    }

    if (visible.length) $("empty").style.display = "none";
    else emptyState("no-match");

    const can = findings.length > 0;
    $("export").disabled = !can;
    $("send").disabled = !can;
    const changes = report.changes || [];
    $("changes").disabled = changes.length === 0;
    $("changes").textContent = changes.length ? `Changes (${changes.length})` : "Changes";
    const muted = findings.filter(findingMuted).length;
    setStatus(`${findings.length} findings · ${report.ok} OK / ${report.warn} WARN / ${report.bad} BAD / ${report.error} ERROR` +
      (muted ? ` · ${muted} muted` : ""));
  }

  function setStatus(t) { $("statusText").textContent = t; }

  const SEVERITY = { OK: 0, WARN: 1, BAD: 2, ERROR: 3 };
  function worstOf(statuses) {
    let worst = "OK";
    for (const s of statuses) if ((SEVERITY[s] || 0) > (SEVERITY[worst] || 0)) worst = s;
    return worst;
  }

  function escapeHtml(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }

  function cssEscape(s) {
    return window.CSS && CSS.escape ? CSS.escape(s) : String(s).replace(/["\\]/g, "\\$&");
  }

  /* ---------------- actions ---------------- */
  async function run() {
    running = true;
    setBusy(true);
    addToWorkspace($("configPath").value); // the fleets you run become your workspace
    setStatus("running checks…");
    render(); // show the loading state right away (unless a prior run is on screen)
    try {
      report = await Backend.RunChecks($("configPath").value, $("stack").value);
    } catch (e) {
      report = { err: String(e) };
    }
    // pull the metric series so the table can draw inline per-target sparklines
    if (report && !report.err) {
      try { metricSeries = (await Backend.Metrics($("configPath").value, 60)) || []; } catch (_) {}
    }
    running = false;
    setBusy(false);
    render();
    maybeNotify();
    if (dashboardOn) renderDashboard();
  }

  // maybeNotify fires a native notification when the run is bad and Notify is on.
  function maybeNotify() {
    if (IS_MOCK || !$("notify").checked || !report || report.err) return;
    if (report.worst === "BAD" || report.worst === "ERROR") {
      const n = report.bad + report.error;
      try { Backend.Notify("checkfleet: " + report.worst, n + " problem(s) — " + (report.configPath || "")); } catch (_) {}
    }
  }

  /* ---------------- settings persistence ---------------- */
  function saveSettings() {
    try {
      localStorage.setItem("cf-settings", JSON.stringify({
        config: $("configPath").value,
        stack: $("stack").value,
        interval: $("interval").value,
        auto: $("auto").checked,
        notify: $("notify").checked,
        groupBy: $("groupBy").checked,
        hideMuted: $("hideMuted").checked,
        view: view,
      }));
    } catch (_) {}
  }
  function loadSettings() {
    try { return JSON.parse(localStorage.getItem("cf-settings") || "{}"); } catch (_) { return {}; }
  }

  /* ---------------- saved views / presets (CF-108) ---------------- */
  // A view bundles the toolbar knobs you look at often. Logic lives in the
  // testable CFPresets module; here we bind it to the DOM and localStorage.
  let presets = [];
  function loadPresets() {
    try { presets = CFPresets.parse(localStorage.getItem("cf-views") || "[]"); } catch (_) { presets = []; }
  }
  function savePresets() {
    try { localStorage.setItem("cf-views", CFPresets.serialize(presets)); } catch (_) {}
  }
  // currentState reads the exact toolbar knobs a preset owns.
  function currentState() {
    return { stack: $("stack").value, filter: $("filter").value,
      minsev: $("minsev").value, group: $("groupBy").checked, view: view };
  }
  function applyPreset(p) {
    if (!p) return;
    $("stack").value = p.stack || "";
    $("filter").value = p.filter || "";
    $("minsev").value = p.minsev || "ok";
    $("groupBy").checked = !!p.group;
    setView(p.view || "fleet");
    render();
    saveSettings();
    toast("View “" + p.name + "”", { timeout: 1600 });
  }
  // A one-line human summary of a preset, for the chip's tooltip.
  function chipTitle(p) {
    const bits = [p.view || "fleet", p.stack ? "stack " + p.stack : "base stack"];
    if (p.filter) bits.push("filter “" + p.filter + "”");
    if (p.minsev && p.minsev !== "ok") bits.push("≥ " + p.minsev.toUpperCase());
    if (p.group) bits.push("grouped");
    return bits.join(" · ");
  }
  function renderViewChips() {
    const host = $("viewChips");
    if (!host) return;
    if (!presets.length) {
      host.innerHTML = `<span class="views-empty">No saved views yet — set the toolbar how you like it, then <b>Save view</b>.</span>`;
      return;
    }
    const st = currentState();
    host.innerHTML = presets.map((p) => {
      const active = CFPresets.matches(p, st);
      return `<span class="chip view-chip${active ? " active" : ""}" title="${escapeHtml(chipTitle(p))}">` +
        `<button class="chip-apply" data-view-apply="${escapeHtml(p.name)}">${escapeHtml(p.name)}</button>` +
        `<button class="chip-x" data-view-del="${escapeHtml(p.name)}" aria-label="Delete view ${escapeHtml(p.name)}">✕</button></span>`;
    }).join("");
  }
  function saveCurrentView() {
    openDrawer("Save view", `
      <p class="drawer-msg">Name this view — it captures the current stack, filter, min-severity, group toggle and open view. Reusing a name overwrites it.</p>
      <div class="view-form">
        <input id="viewName" type="text" placeholder="e.g. Prod errors" autocomplete="off" maxlength="40" aria-label="View name">
        <button class="btn btn-primary" data-view-save-confirm>Save</button>
      </div>`);
    const inp = $("viewName");
    if (inp) { inp.focus(); inp.addEventListener("keydown", (e) => { if (e.key === "Enter") { e.preventDefault(); confirmSaveView(); } }); }
  }
  function confirmSaveView() {
    const name = ($("viewName") ? $("viewName").value : "");
    const p = CFPresets.normalize(name, currentState());
    if (!p) { toast("Enter a name for the view", { kind: "warn" }); return; }
    presets = CFPresets.upsert(presets, p);
    savePresets();
    renderViewChips();
    closeDrawer();
    toast("Saved view “" + p.name + "”", { kind: "success", timeout: 2500 });
  }
  async function exportViews() {
    if (!presets.length) { toast("No saved views to export", { kind: "warn" }); return; }
    const json = CFPresets.serialize(presets);
    try {
      await navigator.clipboard.writeText(json);
      toast("Copied " + presets.length + " view" + (presets.length === 1 ? "" : "s") + " to the clipboard (JSON)", { kind: "success" });
    } catch (_) {
      openDrawer("Export views", `<p class="drawer-msg">Copy this JSON to share or back up your views.</p><pre class="view-json">${escapeHtml(json)}</pre>`);
    }
  }
  async function importViews() {
    let text = "";
    try { text = await navigator.clipboard.readText(); } catch (_) {}
    openDrawer("Import views", `
      <p class="drawer-msg">Paste a views JSON export below (pre-filled from your clipboard when possible). Views with a matching name are overwritten.</p>
      <textarea id="importJson" class="view-json-input" spellcheck="false" placeholder='[ { "name": "Prod errors", "minsev": "error" } ]'>${escapeHtml(text || "")}</textarea>
      <button class="btn btn-primary" data-view-import-confirm>Import</button>`);
    const ta = $("importJson"); if (ta) ta.focus();
  }
  function confirmImportViews() {
    const text = $("importJson") ? $("importJson").value : "";
    const incoming = CFPresets.parse(text);
    if (!incoming.length) { toast("No valid views found in that JSON", { kind: "warn" }); return; }
    incoming.forEach((p) => { presets = CFPresets.upsert(presets, p); });
    savePresets();
    renderViewChips();
    closeDrawer();
    toast("Imported " + incoming.length + " view" + (incoming.length === 1 ? "" : "s"), { kind: "success" });
  }

  /* ---------------- acknowledge / mute findings (CF-110) ---------------- */
  // A mute silences one finding (keyed config#check#target) for a while. Logic
  // lives in the testable CFAcks module; here we bind it to storage and the UI.
  let acks = {};
  function loadAcks() {
    try { acks = CFAcks.prune(JSON.parse(localStorage.getItem("cf-acks") || "{}"), Date.now()); }
    catch (_) { acks = {}; }
  }
  function saveAcks() { try { localStorage.setItem("cf-acks", JSON.stringify(acks)); } catch (_) {} }
  function ackKey(f) { return CFAcks.key($("configPath").value, f.check, f.target); }
  function findingMuted(f) { return CFAcks.isMuted(acks, ackKey(f), Date.now()); }
  // syncMutedKeys pushes the currently-active mute keys to Go so the background
  // monitor honours them (CF-111). No-op in the browser preview.
  function activeMutedKeys() {
    const now = Date.now();
    return Object.keys(acks).filter((k) => CFAcks.isMuted(acks, k, now));
  }
  function syncMutedKeys() {
    try { if (Backend.SetMutedKeys && !IS_MOCK) Backend.SetMutedKeys(activeMutedKeys()); } catch (_) {}
  }
  function muteFinding(f, choice) {
    const now = Date.now();
    acks = CFAcks.mute(acks, { key: ackKey(f), until: CFAcks.durationUntil(choice, now), at: now });
    saveAcks();
    syncMutedKeys();
    render();
    toast("Muted " + f.check + " · " + f.target, { timeout: 2200 });
  }
  function unmuteFinding(f) {
    acks = CFAcks.unmute(acks, ackKey(f));
    saveAcks();
    syncMutedKeys();
    render();
    toast("Unmuted " + f.check + " · " + f.target, { timeout: 2000 });
  }

  /* ---------------- operator notes (CF-112) ---------------- */
  // A note pins "who's on it / what's going on" to a finding, keyed like a mute.
  let notes = {};
  function loadNotes() {
    try { notes = CFNotes.sanitize(JSON.parse(localStorage.getItem("cf-notes") || "{}")); }
    catch (_) { notes = {}; }
  }
  function saveNotes() { try { localStorage.setItem("cf-notes", JSON.stringify(notes)); } catch (_) {} }
  function findingNote(f) { return CFNotes.get(notes, ackKey(f)); }
  function setFindingNote(f, owner, text) {
    const before = CFNotes.has(notes, ackKey(f));
    notes = CFNotes.set(notes, { key: ackKey(f), owner, text, at: Date.now() });
    saveNotes();
    render();
    const after = CFNotes.has(notes, ackKey(f));
    toast(after ? "Note saved" : (before ? "Note cleared" : "Note empty"), { timeout: 2000, kind: after || before ? "success" : "warn" });
  }

  async function refreshStacks() {
    try {
      const stacks = (await Backend.ListStacks($("configPath").value)) || [];
      const sel = $("stack");
      const cur = sel.value;
      sel.innerHTML = '<option value="">(base)</option>' +
        stacks.map((s) => `<option value="${escapeHtml(s)}">${escapeHtml(s)}</option>`).join("");
      sel.value = cur;
    } catch (_) {}
  }

  // Auto-refresh is a Go-driven background monitor (CF-109): the ticker lives in
  // Go, emits samples we render off-thread, and fires a native notification only
  // when the worst status changes. In mock/browser preview (no Wails backend) we
  // fall back to a plain JS interval so the toggle still does something.
  function setAutoRefresh(on) {
    if (timer) { clearInterval(timer); timer = null; }
    const secs = parseInt($("interval").value, 10) || 30;
    if (Backend.StartMonitor && !IS_MOCK) {
      if (on) Backend.StartMonitor($("configPath").value, $("stack").value, secs);
      else Backend.StopMonitor();
      setMonBadge(on);
      return;
    }
    if (on) timer = setInterval(run, secs * 1000);
    setMonBadge(on);
  }

  // setMonBadge shows/updates the "● monitoring" chip in the status bar, colored
  // by the latest worst status.
  function setMonBadge(on, worst) {
    const b = $("monBadge");
    if (!b) return;
    b.hidden = !on;
    if (worst) {
      b.className = "mon-badge s-" + worst;
      $("monText").textContent = "monitoring · " + worst;
    } else if (on) {
      b.className = "mon-badge";
      $("monText").textContent = "monitoring";
    }
  }

  // onMonitorSample renders a background sample and, when the worst status
  // changed, surfaces a toast (Go already fired the native notification).
  function onMonitorSample(s) {
    if (!s || !s.report) return;
    report = s.report;
    render();
    setMonBadge(true, s.worst);
    if (s.changed) {
      const kind = s.worst === "OK" ? "success" : (s.worst === "WARN" ? "warn" : "error");
      toast("Monitor: fleet is now " + s.worst, { kind });
    }
  }

  async function save(fmt) {
    try {
      const path = await Backend.SaveReport(fmt);
      if (path) { setStatus("saved: " + path); toast("Exported " + fmt.toUpperCase() + " → " + path, { kind: "success" }); }
    } catch (e) { setStatus("export error: " + e); toast("Export failed: " + e, { kind: "error" }); }
  }

  // sendReport posts the current run to a chat/webhook target (CF-106). The URL
  // lives only in an env var — the app never asks for it; the toast reports back.
  async function sendReport() {
    const target = $("sendfmt").value;
    setStatus("sending to " + target + "…");
    try {
      const r = await Backend.Send(target);
      const kind = r.ok ? "success" : (/not configured/.test(r.message) ? "warn" : "error");
      toast(r.message, { kind });
      setStatus(r.message);
    } catch (e) { toast("Send failed: " + e, { kind: "error" }); }
  }

  function toggleTheme() {
    const root = document.documentElement;
    const next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
    root.setAttribute("data-theme", next);
    try { localStorage.setItem("cf-theme", next); } catch (_) {}
  }

  /* ---------------- view navigation (CF-96) ---------------- */
  // The app has three first-class views — fleet, dashboard, config — driven by
  // one function so the titlebar tabs, shortcuts and command palette stay in sync.
  function setView(name) {
    if (name !== "fleet" && name !== "dashboard" && name !== "config") name = "fleet";
    view = name;
    editorOn = name === "config";
    dashboardOn = name === "dashboard";
    $("dashboard").hidden = !dashboardOn;
    $("editor").hidden = !editorOn;
    $("findingsBar").hidden = name !== "fleet";
    $("viewsBar").hidden = name !== "fleet";
    $("tableWrap").hidden = name !== "fleet";
    $("summary").hidden = name !== "fleet" || !report;
    const tabs = document.querySelectorAll(".viewtab");
    tabs.forEach((t) => {
      const on = t.dataset.view === name;
      t.classList.toggle("active", on);
      t.setAttribute("aria-selected", on ? "true" : "false");
    });
    if (dashboardOn) renderDashboard();
    if (editorOn) loadConfigText();
    renderViewChips(); // keep the active-view highlight in sync
    saveSettings();
  }

  // renderDashboard pulls the persisted history (survives restarts) and draws the
  // dashSkeleton drops shimmer placeholders into the chart hosts while the
  // history reads are in flight, so the dashboard never flashes empty.
  function dashSkeleton() {
    const bar = `<div class="skeleton skeleton-chart"></div>`;
    ["chartArea", "chartBand", "chartHeatmap", "chartLine", "sloBox"].forEach((id) => {
      const el = $(id); if (el) el.innerHTML = bar;
    });
    const d = $("chartDonut"); if (d) d.innerHTML = `<div class="skeleton skeleton-donut"></div>`;
  }

  // stacked-area, donut and worst-status band. The donut shows the live run when
  // there is one, otherwise the newest history point.
  async function renderDashboard() {
    dashSkeleton(); // placeholders while the history reads resolve
    let points = [];
    try { points = (await Backend.Trend($("configPath").value, 60)) || []; }
    catch (_) { points = []; }

    const has = points.length > 0;
    $("dashGrid").hidden = !has;
    $("dashEmpty").hidden = has;
    $("dashSub").textContent = has ? points.length + " run(s) · saved beside the config" : "";
    if (!has) return;

    const last = points[points.length - 1];
    const dist = report && !report.err
      ? { ok: report.ok, warn: report.warn, bad: report.bad, error: report.error }
      : { ok: last.ok, warn: last.warn, bad: last.bad, error: last.error };

    $("chartArea").innerHTML = CFCharts.svgArea(points, { w: 680, h: 210 });
    $("chartDonut").innerHTML = CFCharts.svgDonut(dist, { size: 168 });
    $("chartBand").innerHTML = CFCharts.svgBand(points, { w: 680, h: 26 });
    $("chartLegend").innerHTML = [["ok", "OK"], ["warn", "WARN"], ["bad", "BAD"], ["error", "ERROR"]]
      .map(([k, label]) => `<div class="lg"><span class="sw ${k}"></span>${label}<b>${dist[k] || 0}</b></div>`).join("");
    const when = (u) => new Date(u * 1000).toLocaleString();
    $("bandAxis").innerHTML =
      `<span>${escapeHtml(when(points[0].unix))}</span><span>${escapeHtml(when(last.unix))}</span>`;

    // Per-module heatmap (CF-93) — a second history read, collapsed per module.
    let mt = { modules: [], runs: [] };
    try { mt = (await Backend.TrendByModule($("configPath").value, 60)) || mt; }
    catch (_) { mt = { modules: [], runs: [] }; }
    moduleTrend = mt;
    $("chartHeatmap").innerHTML = mt.modules && mt.modules.length
      ? CFCharts.svgHeatmap(mt.modules, mt.runs, {})
      : `<p class="drawer-msg">Only one module in history — nothing to compare yet.</p>`;

    // Availability / SLO (CF-95) — a third history read, rolled up per target.
    let av = null;
    try { av = await Backend.Availability($("configPath").value, 60); }
    catch (_) { av = null; }
    renderAvailability(av);

    // Metric-over-time series (CF-94) — from the numeric Finding.Value in history.
    try { metricSeries = (await Backend.Metrics($("configPath").value, 60)) || []; }
    catch (_) { metricSeries = []; }
    populateMetricSel();
    drawMetric();
  }

  // populateMetricSel fills the series picker, keeping the current pick if valid.
  function populateMetricSel() {
    const sel = $("metricSel");
    const cur = sel.value;
    sel.innerHTML = metricSeries.map((s, i) =>
      `<option value="${i}">${escapeHtml(s.check)} ${escapeHtml(s.target)} (${escapeHtml(s.unit || "")})</option>`).join("");
    if (cur && metricSeries[cur]) sel.value = cur;
  }

  // drawMetric renders the selected series as a line chart (or an empty note).
  function drawMetric() {
    const host = $("chartLine");
    if (!metricSeries.length) {
      host.innerHTML = `<p class="chart-empty">No numeric metrics in history yet — run modules that measure a value (http/tcp latency, ntp offset, certs expiry).</p>`;
      return;
    }
    const idx = Math.min(metricSeries.length - 1, Math.max(0, parseInt($("metricSel").value, 10) || 0));
    host.innerHTML = CFCharts.svgLine(metricSeries[idx].points, { w: 680, h: 180, unit: metricSeries[idx].unit });
  }

  // renderAvailability paints the uptime hero + the least-available targets.
  function renderAvailability(av) {
    const box = $("sloBox");
    if (!av || !av.runs) { box.innerHTML = `<p class="drawer-msg">No history yet.</p>`; return; }
    const when = (u) => new Date(u * 1000).toLocaleString();
    const worst = av.currentWorst || "OK";
    const hero = `
      <div class="slo-hero s-${worst}">
        <b>${(av.uptime || 0).toFixed(1)}%</b><span>uptime</span>
        <div class="slo-meta">${av.runs} runs · now
          <span class="badge ${worst}">${worst}</span> since ${escapeHtml(when(av.currentSinceUnix))}</div>
      </div>`;
    const targets = (av.targets || []).slice(0, 6).map((t) => `
      <div class="slo-row">
        <span class="slo-name mono" title="${escapeHtml(t.check + " " + t.target)}">${escapeHtml(t.check)} ${escapeHtml(t.target)}</span>
        ${CFCharts.svgMeter(t.uptime)}
        <span class="slo-pct">${(t.uptime || 0).toFixed(0)}%</span>
      </div>`).join("");
    box.innerHTML = hero + `<div class="slo-targets">${targets}</div>`;
  }

  // showModuleDrill opens a drawer with one module's worst-status band over time.
  function showModuleDrill(m) {
    const runs = (moduleTrend.runs || []).filter((r) => r.worst && r.worst[m]);
    if (!runs.length) {
      openDrawer("Module: " + m, `<p class="drawer-msg">No history for ${escapeHtml(m)}.</p>`);
      return;
    }
    const points = runs.map((r) => ({ worst: r.worst[m], unix: r.unix }));
    const last = points[points.length - 1];
    const when = (u) => new Date(u * 1000).toLocaleString();
    openDrawer("Module: " + m, `
      <p class="drawer-msg">Worst status of <b>${escapeHtml(m)}</b> across ${points.length} run(s), oldest → newest.</p>
      <div class="chart-host band-host">${CFCharts.svgBand(points, { w: 680, h: 26 })}</div>
      <div class="band-axis"><span>${escapeHtml(when(points[0].unix))}</span><span>${escapeHtml(when(last.unix))}</span></div>
      <div class="kv"><span>Latest</span><span class="badge ${last.worst}">${last.worst}</span></div>`);
  }

  async function loadConfigText() {
    $("cfgPath").textContent = $("configPath").value || "(no file)";
    $("cfgMsg").hidden = true;
    $("addForm").hidden = true;
    $("schedBox").hidden = true;
    try { $("cfgText").value = await Backend.ReadConfig($("configPath").value); }
    catch (e) { cfgMessage(String(e), true); }
    liveValidate();
  }

  // liveValidate (CF-105) — debounced validity badge in the editor bar while you
  // type, using the same engine.Validate as the Validate button (no disk write).
  let _cfgValTimer = null;
  function liveValidate() {
    clearTimeout(_cfgValTimer);
    _cfgValTimer = setTimeout(async () => {
      const badge = $("cfgStatus");
      if (!badge) return;
      let problems = [];
      try { problems = (await Backend.ValidateText($("cfgText").value)) || []; }
      catch (e) { problems = [String(e)]; }
      if (problems.length === 0) {
        badge.className = "cfg-status ok"; badge.textContent = "✓ valid"; badge.removeAttribute("title");
      } else {
        badge.className = "cfg-status bad";
        badge.textContent = "✕ " + problems.length + " problem" + (problems.length > 1 ? "s" : "");
        badge.title = problems.join("\n");
      }
    }, 400);
  }

  function cfgMessage(text, bad) {
    const el = $("cfgMsg");
    el.textContent = text;
    el.className = "editor-msg" + (bad ? " bad" : " ok");
    el.hidden = false;
  }

  async function cfgValidate() {
    let problems = [];
    try { problems = (await Backend.ValidateText($("cfgText").value)) || []; }
    catch (e) { problems = [String(e)]; }
    if (problems.length === 0) cfgMessage("Config is valid ✅", false);
    else cfgMessage("Problems:\n- " + problems.join("\n- "), true);
  }

  // Placeholders + which extra field shows, per endpoint kind.
  const EP_HINTS = {
    http:     { label: "URL",       ph: "https://example.com/health" },
    certs:    { label: "Host:port", ph: "example.com:443" },
    tls:      { label: "Host:port", ph: "example.com:443" },
    tcp:      { label: "Host:port", ph: "db.internal:5432" },
    dns:      { label: "Name",      ph: "example.com" },
    redis:    { label: "Host:port", ph: "cache-01:6379" },
    nats:     { label: "Monitor",   ph: "nats-01:8222" },
    smtp:     { label: "Host:port", ph: "relay.example.com:587" },
    grpc:     { label: "Host:port", ph: "svc.internal:443" },
    postgres: { label: "DSN",       ph: "postgres://ops@pg-01:5432/app" },
  };

  function syncAddForm() {
    const kind = $("epKind").value;
    const h = EP_HINTS[kind] || EP_HINTS.http;
    $("epValueLabel").textContent = h.label;
    $("epValue").placeholder = h.ph;
    $("epExtraStatus").hidden = kind !== "http";
    $("epExtraType").hidden = kind !== "dns";
    const hasExtra = kind === "grpc" || kind === "postgres";
    $("epExtraExtra").hidden = !hasExtra;
    if (hasExtra) {
      $("epExtraLabel").textContent = kind === "grpc" ? "Service (opt.)" : "Password env (opt.)";
      $("epExtra").placeholder = kind === "grpc" ? "grpc.health.v1.Health" : "PGPASSWORD";
    }
  }

  function toggleAddForm(on) {
    const show = on === undefined ? $("addForm").hidden : on;
    $("addForm").hidden = !show;
    if (show) { $("schedBox").hidden = true; syncAddForm(); $("epValue").focus(); }
  }

  async function addEndpoint(e) {
    e.preventDefault();
    const kind = $("epKind").value;
    const value = $("epValue").value.trim();
    if (!value) { $("epValue").focus(); return; }
    try {
      const updated = await Backend.AddEndpoint(
        $("cfgText").value, kind, value, $("epType").value, $("epExtra").value.trim(), Number($("epStatus").value) || 0);
      $("cfgText").value = updated;
      $("epValue").value = "";
      $("epExtra").value = "";
      toggleAddForm(false);
      cfgMessage("Added " + kind + " endpoint — review and Save.", false);
      toast("Added " + kind + " endpoint — review and Save", { kind: "success" });
      liveValidate();
    } catch (err) { cfgMessage("Add failed: " + err, true); toast("Add failed: " + err, { kind: "error" }); }
  }

  async function toggleSchedule() {
    if (!$("schedBox").hidden) { $("schedBox").hidden = true; return; }
    $("addForm").hidden = true;
    try {
      $("schedBox").textContent = await Backend.ScheduleSnippet($("configPath").value, $("interval").value);
      $("schedBox").hidden = false;
    } catch (err) { cfgMessage("Schedule failed: " + err, true); }
  }

  async function cfgSave() {
    try {
      await Backend.SaveConfig($("configPath").value, $("cfgText").value);
      cfgMessage("Saved " + $("configPath").value, false);
      toast("Config saved", { kind: "success" });
      await refreshStacks();
    } catch (e) { cfgMessage("Save error: " + e, true); toast("Save failed: " + e, { kind: "error" }); }
  }

  /* ---------------- drawer (detail / explain / validate) ---------------- */
  function openDrawer(title, bodyHTML) {
    $("drawerTitle").textContent = title;
    $("drawerBody").innerHTML = bodyHTML;
    $("drawer").hidden = false;
    $("drawerScrim").hidden = false;
    trapFocus($("drawer"));
  }
  function closeDrawer() {
    if ($("drawer").hidden) return;
    $("drawer").hidden = true;
    $("drawerScrim").hidden = true;
    releaseFocus();
  }

  /* ---------------- toasts (CF-101) ---------------- */
  // toast(msg, {kind, timeout, action:{label, fn}}) — a non-blocking notification.
  function toast(msg, opts) {
    opts = opts || {};
    const host = $("toasts");
    if (!host) return;
    const el = document.createElement("div");
    el.className = "toast toast-" + (opts.kind || "info");
    el.setAttribute("role", opts.kind === "error" ? "alert" : "status");
    let html = `<span class="toast-dot"></span><span class="toast-msg">${escapeHtml(msg)}</span>`;
    if (opts.action) html += `<button class="toast-action">${escapeHtml(opts.action.label)}</button>`;
    html += `<button class="toast-x" aria-label="Dismiss">✕</button>`;
    el.innerHTML = html;
    host.appendChild(el);
    while (host.children.length > 4) host.firstChild.remove();
    requestAnimationFrame(() => el.classList.add("in"));

    let timer = setTimeout(() => dismissToast(el), opts.timeout || 3600);
    el.addEventListener("mouseenter", () => clearTimeout(timer));
    el.addEventListener("mouseleave", () => { timer = setTimeout(() => dismissToast(el), 1500); });
    el.querySelector(".toast-x").addEventListener("click", () => dismissToast(el));
    if (opts.action) {
      el.querySelector(".toast-action").addEventListener("click", () => {
        try { opts.action.fn(); } catch (_) {}
        dismissToast(el);
      });
    }
  }
  function dismissToast(el) {
    if (!el || el.classList.contains("out")) return;
    el.classList.add("out");
    el.addEventListener("transitionend", () => el.remove(), { once: true });
    setTimeout(() => el.remove(), 320); // fallback if transitionend doesn't fire
  }

  /* ---------------- focus management (CF-103) ---------------- */
  // Trap Tab within an open dialog and restore focus to the trigger on close.
  let _trapOff = null, _lastFocus = null;
  function trapFocus(container) {
    _lastFocus = document.activeElement;
    const q = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';
    const items = () => Array.from(container.querySelectorAll(q)).filter((el) => !el.disabled && el.offsetParent !== null);
    const first = items()[0];
    if (first) first.focus();
    const onKey = (e) => {
      if (e.key !== "Tab") return;
      const f = items();
      if (!f.length) return;
      const a = f[0], b = f[f.length - 1];
      if (e.shiftKey && document.activeElement === a) { e.preventDefault(); b.focus(); }
      else if (!e.shiftKey && document.activeElement === b) { e.preventDefault(); a.focus(); }
    };
    container.addEventListener("keydown", onKey);
    _trapOff = () => container.removeEventListener("keydown", onKey);
  }
  function releaseFocus() {
    if (_trapOff) { _trapOff(); _trapOff = null; }
    if (_lastFocus && _lastFocus.focus) { try { _lastFocus.focus(); } catch (_) {} }
    _lastFocus = null;
  }

  /* ---------------- command palette (CF-96) ---------------- */
  let palItems = [];
  let palSel = 0;

  // The command set — each entry reuses an existing action, so the palette,
  // titlebar tabs and shortcuts all drive the same code.
  function commandList() {
    return [
      { label: "Run checks", hint: "⌘↵", act: run },
      { label: "Go to Fleet", hint: "1", act: () => setView("fleet") },
      { label: "Go to Dashboard", hint: "2", act: () => setView("dashboard") },
      { label: "Go to Config", hint: "3", act: () => setView("config") },
      { label: "Focus filter", hint: "/", act: () => { setView("fleet"); $("filter").focus(); } },
      { label: "Validate config", act: runValidate },
      { label: "Save current view", act: () => { setView("fleet"); saveCurrentView(); } },
      { label: "Export saved views", act: exportViews },
      { label: "Import saved views", act: () => { setView("fleet"); importViews(); } },
      ...presets.map((p) => ({ label: "View: " + p.name, act: () => applyPreset(p) })),
      { label: "Show trend", act: showTrend },
      { label: "Export as Markdown", act: () => save("markdown") },
      { label: "Export as JSON", act: () => save("json") },
      { label: "Export as HTML", act: () => save("html") },
      { label: "Toggle theme", act: toggleTheme },
    ];
  }

  function paletteOpen() { return !$("paletteBox").hidden; }
  function openPalette() {
    $("paletteScrim").hidden = false;
    $("paletteBox").hidden = false;
    $("paletteInput").value = "";
    filterPalette("");
    trapFocus($("paletteBox")); // captures the trigger, focuses the input (first focusable)
  }
  function closePalette() {
    if ($("paletteBox").hidden) return;
    $("paletteBox").hidden = true;
    $("paletteScrim").hidden = true;
    releaseFocus();
  }

  function filterPalette(q) {
    q = q.trim().toLowerCase();
    palItems = commandList().filter((c) => !q || c.label.toLowerCase().includes(q));
    palSel = 0;
    renderPalette();
  }
  function renderPalette() {
    const ul = $("paletteList");
    if (!palItems.length) {
      ul.innerHTML = `<li class="palette-empty">No matching command</li>`;
      $("paletteInput").removeAttribute("aria-activedescendant");
      return;
    }
    ul.innerHTML = palItems.map((c, i) =>
      `<li id="pl-opt-${i}" role="option" aria-selected="${i === palSel}" data-i="${i}" class="${i === palSel ? "sel" : ""}">` +
      `<span class="pl-label">${escapeHtml(c.label)}</span>` +
      `${c.hint ? `<span class="pl-hint">${escapeHtml(c.hint)}</span>` : ""}</li>`).join("");
    $("paletteInput").setAttribute("aria-activedescendant", "pl-opt-" + palSel);
  }
  function movePalette(d) {
    if (!palItems.length) return;
    palSel = (palSel + d + palItems.length) % palItems.length;
    renderPalette();
    const sel = $("paletteList").querySelector("li.sel");
    if (sel) sel.scrollIntoView({ block: "nearest" });
  }
  function runPalette(i) {
    const c = palItems[i];
    closePalette();
    if (c) { try { c.act(); } catch (_) {} }
  }

  let drawerFinding = null; // the finding shown in the detail drawer, for mute actions

  async function showFindingDetail(f) {
    drawerFinding = f;
    const text = `${f.status} ${f.check} ${f.target} — ${f.message}`;
    const hasMetric = f.value != null;
    const metricRow = hasMetric
      ? `<div class="kv"><span>Metric</span><b class="mono">${escapeHtml(String(f.value))} ${escapeHtml(f.unit || "")}</b></div>`
      : "";
    const muted = findingMuted(f);
    const muteBlock = muted
      ? `<div class="mute-block"><span class="chip chip-muted">${escapeHtml(CFAcks.describe(acks[ackKey(f)], Date.now()))}</span>` +
        `<button class="btn" data-unmute="1">Unmute</button></div>`
      : `<div class="mute-block"><span class="mute-label">Snooze</span>` +
        `<button class="btn" data-mute="1h">1h</button>` +
        `<button class="btn" data-mute="8h">8h</button>` +
        `<button class="btn" data-mute="24h">24h</button>` +
        `<button class="btn" data-mute="recovery">until recovery</button></div>`;
    const note = findingNote(f);
    const noteBlock = `<div class="note-block">` +
      `<span class="mute-label">Note</span>` +
      `<input id="noteOwner" type="text" placeholder="owner (optional)" maxlength="40" autocomplete="off" value="${escapeHtml(note ? note.owner : "")}">` +
      `<textarea id="noteText" placeholder="what's going on with this target…" maxlength="500">${escapeHtml(note ? note.text : "")}</textarea>` +
      `<button class="btn btn-primary" data-note-save="1">Save note</button></div>`;
    openDrawer("Finding", `
      <div class="kv"><span>Status</span><span class="badge ${f.status}">${f.status}</span></div>
      <div class="kv"><span>Check</span><b class="mono">${escapeHtml(f.check)}</b></div>
      <div class="kv"><span>Target</span><b class="mono">${escapeHtml(f.target)}</b></div>
      ${metricRow}
      <p class="drawer-msg">${escapeHtml(f.message)}</p>
      ${muteBlock}
      ${noteBlock}
      ${hasMetric ? `<div class="finding-trend" id="findingTrend"><p class="chart-empty">loading trend…</p></div>` : ""}
      <button class="btn copy-btn" data-copy="${escapeHtml(text)}">Copy</button>`);

    // Metric-carrying findings get their history line right in the drawer (CF-94).
    if (hasMetric) {
      let series = [];
      try { series = (await Backend.Metrics($("configPath").value, 60)) || []; } catch (_) {}
      const s = series.find((x) => x.check === f.check && x.target === f.target);
      const el = $("findingTrend");
      if (el) {
        el.innerHTML = s && s.points && s.points.length
          ? `<div class="kv"><span>${escapeHtml(f.check)} trend</span><span>${escapeHtml(s.unit || "")}</span></div>` +
            CFCharts.svgLine(s.points, { w: 340, h: 120, unit: s.unit })
          : `<p class="chart-empty">No history series yet for this target.</p>`;
      }
    }
  }

  async function showExplain(module) {
    let doc = "";
    try { doc = await Backend.Explain(module); } catch (_) {}
    openDrawer("Explain: " + module, doc
      ? `<p class="drawer-msg">${escapeHtml(doc)}</p>`
      : `<p class="drawer-msg">No description for “${escapeHtml(module)}”.</p>`);
  }

  async function runValidate() {
    let problems = [];
    try { problems = (await Backend.Validate($("configPath").value, $("stack").value)) || []; }
    catch (e) { problems = [String(e)]; }
    const body = problems.length === 0
      ? `<p class="ok-note">Config is valid ✅</p>`
      : `<ul class="problems">${problems.map((p) => `<li>${escapeHtml(p)}</li>`).join("")}</ul>`;
    openDrawer("Validate", body);
    if (problems.length === 0) toast("Config is valid", { kind: "success" });
    else toast(problems.length + " problem" + (problems.length > 1 ? "s" : "") + " found", { kind: "warn" });
  }

  const CHANGE_SYMBOL = { new: "+", resolved: "-", worsened: "!", improved: "~" };
  function showChanges() {
    const changes = (report && report.changes) || [];
    if (!changes.length) return;
    const rows = changes.map((c) => `
      <div class="change change-${c.kind}">
        <span class="change-sym">${CHANGE_SYMBOL[c.kind] || "•"}</span>
        <span class="change-kind">${c.kind}</span>
        <span class="mono">${escapeHtml(c.check)} ${escapeHtml(c.target)}</span>
        <span class="change-transition">${c.from}→${c.to}</span>
      </div>`).join("");
    openDrawer("Changes since last run", rows);
  }

  async function showTrend() {
    let points = [];
    try { points = (await Backend.Trend($("configPath").value, 40)) || []; }
    catch (e) { openDrawer("Trend", `<p class="drawer-msg">${escapeHtml(String(e))}</p>`); return; }
    if (!points.length) {
      openDrawer("Trend", `<p class="drawer-msg">No history yet. Run the fleet a few times — each run is saved next to the config and shown here across restarts.</p>`);
      return;
    }
    const bars = points.map((p) => {
      const total = p.ok + p.warn + p.bad + p.error;
      const when = new Date(p.unix * 1000).toLocaleString();
      const tip = `${when} — worst ${p.worst} (${p.ok} OK, ${p.warn} WARN, ${p.bad} BAD, ${p.error} ERROR)`;
      return `<span class="spark-bar s-${p.worst}" title="${escapeHtml(tip)}"><i>${total}</i></span>`;
    }).join("");
    const last = points[points.length - 1];
    openDrawer("Trend — last " + points.length + " runs", `
      <p class="drawer-msg">Worst status per run (oldest → newest). Persisted next to the config, survives restarts.</p>
      <div class="spark">${bars}</div>
      <div class="kv"><span>Latest</span><span class="badge ${last.worst}">${last.worst}</span></div>`);
  }

  /* ---------------- history browser (CF-104) ---------------- */
  const histWhen = (u) => new Date(u * 1000).toLocaleString();

  async function showHistory() {
    let runs = [];
    try { runs = (await Backend.HistoryRuns($("configPath").value, 60)) || []; } catch (_) {}
    if (!runs.length) {
      openDrawer("History", `<p class="drawer-msg">No history yet. Each Run is appended next to the config and shown here across restarts.</p>`);
      return;
    }
    historyRuns = runs; // cached (newest-first) so "compare with previous" can find the older run
    const rows = runs.map((r) => `
      <button class="hist-run" data-run="${r.unix}">
        <span class="badge ${r.worst}">${r.worst}</span>
        <span class="hist-when">${escapeHtml(histWhen(r.unix))}</span>
        <span class="hist-counts">${r.ok}·${r.warn}·${r.bad}·${r.error}<span class="hist-total">/${r.total}</span></span>
      </button>`).join("");
    openDrawer("History — " + runs.length + " runs", `
      <p class="drawer-msg">Persisted runs, newest first. Open one to see its findings, or compare it with the previous run.</p>
      <div class="hist-list">${rows}</div>`);
  }

  async function showRunDetail(unix) {
    let fs = [];
    try { fs = (await Backend.RunAt($("configPath").value, unix)) || []; } catch (_) {}
    const idx = historyRuns.findIndex((r) => r.unix === unix);
    const prev = idx >= 0 && idx + 1 < historyRuns.length ? historyRuns[idx + 1] : null; // older run
    const rows = fs.map((f) => `
      <div class="hist-finding">
        <span class="badge ${f.status}">${f.status}</span>
        <span class="cell-check mono">${escapeHtml(f.check)}</span>
        <span class="mono hist-target">${escapeHtml(f.target)}</span>
        ${f.value != null ? `<span class="hist-val">${escapeHtml(String(f.value))} ${escapeHtml(f.unit || "")}</span>` : ""}
      </div>`).join("");
    openDrawer("Run · " + histWhen(unix), `
      <div class="hist-actions">
        <button class="btn" data-hist-back>← Runs</button>
        ${prev ? `<button class="btn" data-hist-diff="${prev.unix}" data-hist-to="${unix}">Compare with previous</button>` : ""}
      </div>
      <p class="drawer-msg">${fs.length} findings · messages aren't stored in history (status/value only).</p>
      <div class="hist-findings">${rows}</div>`);
  }

  async function showRunDiff(from, to) {
    let ch = [];
    try { ch = (await Backend.DiffRuns($("configPath").value, from, to)) || []; } catch (_) {}
    const body = ch.length
      ? ch.map((c) => `
        <div class="change change-${c.kind}">
          <span class="change-sym">${CHANGE_SYMBOL[c.kind] || "•"}</span>
          <span class="change-kind">${c.kind}</span>
          <span class="mono">${escapeHtml(c.check)} ${escapeHtml(c.target)}</span>
          <span class="change-transition">${c.from}→${c.to}</span>
        </div>`).join("")
      : `<p class="drawer-msg">No changes between these two runs.</p>`;
    openDrawer("Changes " + new Date(from * 1000).toLocaleTimeString() + " → " + new Date(to * 1000).toLocaleTimeString(), `
      <div class="hist-actions"><button class="btn" data-hist-back>← Runs</button></div>${body}`);
  }

  /* ---------------- workspace: your fleets (CF-107) ---------------- */
  function loadWorkspace() {
    try { workspacePaths = JSON.parse(localStorage.getItem("cf-workspace") || "[]"); } catch (_) { workspacePaths = []; }
    if (!Array.isArray(workspacePaths)) workspacePaths = [];
  }
  function saveWorkspace() {
    try { localStorage.setItem("cf-workspace", JSON.stringify(workspacePaths.slice(0, 20))); } catch (_) {}
  }
  function addToWorkspace(path) {
    path = (path || "").trim();
    if (!path) return;
    workspacePaths = [path, ...workspacePaths.filter((p) => p !== path)].slice(0, 20);
    saveWorkspace();
  }
  const wsBasename = (p) => p.split("/").pop() || p;

  function openWorkspace() {
    $("wsScrim").hidden = false;
    $("workspace").hidden = false;
    renderWorkspace();
    trapFocus($("workspace"));
  }
  function closeWorkspace() {
    if ($("workspace").hidden) return;
    $("workspace").hidden = true;
    $("wsScrim").hidden = true;
    releaseFocus();
  }

  function renderWorkspace() {
    const list = $("wsList");
    if (!workspacePaths.length) {
      list.innerHTML = `<p class="drawer-msg">No fleets yet. Add a config, or open one from the toolbar — the ones you use appear here.</p>`;
      $("wsWorst").hidden = true;
      return;
    }
    const cur = $("configPath").value;
    list.innerHTML = workspacePaths.map((p) => {
      const s = wsStatuses[p];
      const dot = s ? `<span class="badge ${s.err ? "ERROR" : s.worst}">${s.err ? "ERR" : s.worst}</span>` : `<span class="ws-dot"></span>`;
      const counts = s && !s.err ? `<span class="ws-counts mono">${s.ok}·${s.warn}·${s.bad}·${s.error}</span>` : "";
      return `<button class="ws-item${p === cur ? " active" : ""}" data-ws="${escapeHtml(p)}" title="${escapeHtml(p)}">` +
        `${dot}<span class="ws-name">${escapeHtml(wsBasename(p))}</span>${counts}</button>`;
    }).join("");
    const worsts = workspacePaths.map((p) => wsStatuses[p]).filter(Boolean).map((s) => s.err ? "ERROR" : s.worst);
    if (worsts.length) {
      const agg = worstOf(worsts);
      $("wsWorst").className = "badge " + agg; $("wsWorst").textContent = agg; $("wsWorst").hidden = false;
    } else { $("wsWorst").hidden = true; }
  }

  async function runWorkspace() {
    if (!workspacePaths.length) return;
    setStatus("running workspace…");
    let sts = [];
    try { sts = (await Backend.WorkspaceStatus(workspacePaths)) || []; } catch (_) {}
    wsStatuses = {};
    sts.forEach((s) => { wsStatuses[s.path] = s; });
    renderWorkspace();
    const agg = worstOf(sts.map((s) => (s.err ? "ERROR" : s.worst))) || "OK";
    toast("Workspace: worst " + agg + " across " + sts.length + " fleets", { kind: agg === "OK" ? "success" : "warn" });
    setStatus("workspace: " + sts.length + " fleets · worst " + agg);
  }

  function switchConfig(path) {
    $("configPath").value = path;
    addToWorkspace(path);
    updateHint();
    refreshStacks();
    saveSettings();
    closeWorkspace();
    setView("fleet");
    run();
  }

  /* ---------------- wiring ---------------- */
  function bind() {
    $("run").addEventListener("click", run);
    $("pick").addEventListener("click", async () => {
      try {
        const p = await Backend.OpenConfigDialog();
        if (p) { $("configPath").value = p; updateHint(); await refreshStacks(); }
      } catch (_) {}
    });
    $("configPath").addEventListener("change", () => { updateHint(); refreshStacks(); saveSettings(); if ($("auto").checked) setAutoRefresh(true); });
    $("filter").addEventListener("input", () => { render(); renderViewChips(); });
    $("minsev").addEventListener("change", () => { render(); renderViewChips(); });
    $("stack").addEventListener("change", () => { saveSettings(); renderViewChips(); if ($("auto").checked) setAutoRefresh(true); });
    $("notify").addEventListener("change", saveSettings);
    $("auto").addEventListener("change", (e) => { setAutoRefresh(e.target.checked); saveSettings(); });
    // background-monitor events (CF-109)
    Events.EventsOn("monitor:sample", onMonitorSample);
    Events.EventsOn("monitor:stopped", () => setMonBadge(false));
    $("interval").addEventListener("change", () => { if ($("auto").checked) setAutoRefresh(true); saveSettings(); });
    $("export").addEventListener("click", () => save($("expfmt").value));
    $("send").addEventListener("click", sendReport);
    $("theme").addEventListener("click", toggleTheme);
    $("validate").addEventListener("click", runValidate);
    $("changes").addEventListener("click", showChanges);
    $("trend").addEventListener("click", showTrend);
    $("history").addEventListener("click", showHistory);
    $("groupBy").addEventListener("change", () => { render(); saveSettings(); renderViewChips(); });
    $("hideMuted").addEventListener("change", () => { render(); saveSettings(); });
    document.querySelectorAll(".viewtab").forEach((t) =>
      t.addEventListener("click", () => setView(t.dataset.view)));
    $("palette").addEventListener("click", () => openPalette());
    // saved views (CF-108)
    $("viewSave").addEventListener("click", saveCurrentView);
    $("viewExport").addEventListener("click", exportViews);
    $("viewImport").addEventListener("click", importViews);
    $("viewChips").addEventListener("click", (e) => {
      const ap = e.target.closest("[data-view-apply]");
      if (ap) { applyPreset(CFPresets.find(presets, ap.dataset.viewApply)); return; }
      const del = e.target.closest("[data-view-del]");
      if (del) {
        presets = CFPresets.remove(presets, del.dataset.viewDel);
        savePresets(); renderViewChips();
        toast("Deleted view “" + del.dataset.viewDel + "”", { timeout: 2000 });
      }
    });
    $("workspaceBtn").addEventListener("click", openWorkspace);
    $("wsClose").addEventListener("click", closeWorkspace);
    $("wsScrim").addEventListener("click", closeWorkspace);
    $("wsRunAll").addEventListener("click", runWorkspace);
    $("wsAdd").addEventListener("click", async () => {
      try { const p = await Backend.OpenConfigDialog(); if (p) { addToWorkspace(p); renderWorkspace(); } } catch (_) {}
    });
    $("wsList").addEventListener("click", (e) => {
      const it = e.target.closest("[data-ws]");
      if (it) switchConfig(it.dataset.ws);
    });
    $("dashRefresh").addEventListener("click", renderDashboard);
    $("metricSel").addEventListener("change", drawMetric);
    // empty-state actions (retry / open editor / clear filters)
    $("empty").addEventListener("click", (e) => {
      const act = e.target.closest("[data-empty-act]");
      if (!act) return;
      if (act.dataset.emptyAct === "retry") run();
      else if (act.dataset.emptyAct === "edit") setView("config");
      else if (act.dataset.emptyAct === "clear") { $("filter").value = ""; $("minsev").value = "ok"; render(); }
    });
    $("chartHeatmap").addEventListener("click", (e) => {
      const el = e.target.closest("[data-module]");
      if (el) showModuleDrill(el.getAttribute("data-module"));
    });
    $("cfgReload").addEventListener("click", loadConfigText);
    $("cfgText").addEventListener("input", liveValidate);
    $("cfgValidate").addEventListener("click", cfgValidate);
    $("cfgSave").addEventListener("click", cfgSave);
    $("addEndpoint").addEventListener("click", () => toggleAddForm());
    $("epCancel").addEventListener("click", () => toggleAddForm(false));
    $("epKind").addEventListener("change", syncAddForm);
    $("addForm").addEventListener("submit", addEndpoint);
    $("schedule").addEventListener("click", toggleSchedule);

    // Row click -> finding detail.
    $("rows").addEventListener("click", (e) => {
      const head = e.target.closest("tr.group-row");
      if (head) {
        const mod = head.dataset.group;
        const collapsed = head.classList.toggle("collapsed");
        head.querySelector(".group-caret").textContent = collapsed ? "▸" : "▾";
        $("rows").querySelectorAll(`tr[data-grp="${cssEscape(mod)}"]`)
          .forEach((r) => { r.hidden = collapsed; });
        return;
      }
      const tr = e.target.closest("tr[data-i]");
      if (tr) showFindingDetail(visibleFindings[+tr.dataset.i]);
    });
    // Module chip click -> explain.
    $("mModules").addEventListener("click", (e) => {
      const chip = e.target.closest("[data-mod]");
      if (chip) showExplain(chip.dataset.mod);
    });
    $("mModules").addEventListener("keydown", (e) => {
      if (e.key !== "Enter" && e.key !== " ") return;
      const chip = e.target.closest("[data-mod]");
      if (chip) { e.preventDefault(); showExplain(chip.dataset.mod); }
    });
    // Copy button inside the drawer.
    $("drawerBody").addEventListener("click", (e) => {
      const btn = e.target.closest(".copy-btn");
      if (btn) {
        try { navigator.clipboard.writeText(btn.dataset.copy); btn.textContent = "Copied"; toast("Copied to clipboard", { kind: "success", timeout: 2000 }); } catch (_) {}
        return;
      }
      // history browser navigation (CF-104)
      const back = e.target.closest("[data-hist-back]");
      if (back) { showHistory(); return; }
      const diff = e.target.closest("[data-hist-diff]");
      if (diff) { showRunDiff(+diff.dataset.histDiff, +diff.dataset.histTo); return; }
      const runEl = e.target.closest("[data-run]");
      if (runEl) { showRunDetail(+runEl.dataset.run); return; }
      // mute / unmute a finding (CF-110)
      const mb = e.target.closest("[data-mute]");
      if (mb && drawerFinding) { muteFinding(drawerFinding, mb.dataset.mute); closeDrawer(); return; }
      if (e.target.closest("[data-unmute]") && drawerFinding) { unmuteFinding(drawerFinding); closeDrawer(); return; }
      // save a note (CF-112)
      if (e.target.closest("[data-note-save]") && drawerFinding) {
        setFindingNote(drawerFinding, $("noteOwner") ? $("noteOwner").value : "", $("noteText") ? $("noteText").value : "");
        closeDrawer();
        return;
      }
      // saved views (CF-108)
      if (e.target.closest("[data-view-save-confirm]")) { confirmSaveView(); return; }
      if (e.target.closest("[data-view-import-confirm]")) { confirmImportViews(); }
    });
    $("drawerClose").addEventListener("click", closeDrawer);
    $("drawerScrim").addEventListener("click", closeDrawer);

    // command palette wiring
    $("paletteInput").addEventListener("input", (e) => filterPalette(e.target.value));
    $("paletteInput").addEventListener("keydown", (e) => {
      if (e.key === "ArrowDown") { e.preventDefault(); movePalette(1); }
      else if (e.key === "ArrowUp") { e.preventDefault(); movePalette(-1); }
      else if (e.key === "Enter") { e.preventDefault(); runPalette(palSel); }
      else if (e.key === "Escape") { e.preventDefault(); closePalette(); }
    });
    $("paletteList").addEventListener("click", (e) => {
      const li = e.target.closest("li[data-i]");
      if (li) runPalette(+li.dataset.i);
    });
    $("paletteScrim").addEventListener("click", closePalette);

    // global keyboard shortcuts
    document.addEventListener("keydown", (e) => {
      const mod = e.metaKey || e.ctrlKey;
      if (mod && (e.key === "k" || e.key === "K")) { e.preventDefault(); paletteOpen() ? closePalette() : openPalette(); return; }
      if (mod && e.key === "Enter") { e.preventDefault(); run(); return; }
      if (e.key === "Escape") { closePalette(); closeDrawer(); closeWorkspace(); return; }
      // bare keys only when not typing and no modifier
      const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName || "");
      if (typing || mod || e.altKey) return;
      if (e.key === "1") setView("fleet");
      else if (e.key === "2") setView("dashboard");
      else if (e.key === "3") setView("config");
      else if (e.key === "/") { e.preventDefault(); setView("fleet"); $("filter").focus(); }
      else if (e.key === "r" || e.key === "R") run();
    });
  }

  function updateHint() {
    $("configHint").textContent = $("configPath").value || "";
  }

  async function init() {
    try {
      const t = localStorage.getItem("cf-theme");
      if (t) document.documentElement.setAttribute("data-theme", t);
    } catch (_) {}

    bind();
    $("version").textContent = await Backend.Version();

    // Startup config: env-chosen path (CHECKFLEET_CONFIG) or ./checkfleet.yml,
    // plus an optional auto-run (CHECKFLEET_AUTORUN=1) so the app can open
    // straight into a fleet.
    let startup = { path: "", autoRun: false };
    try { startup = (await Backend.StartupConfig()) || startup; } catch (_) {}

    // Persisted settings (config/stack/interval/auto/notify) win over the
    // startup defaults, so the app reopens where you left it.
    const s = loadSettings();
    loadWorkspace();
    loadPresets();
    loadAcks();
    loadNotes();
    syncMutedKeys();
    $("configPath").value = s.config || startup.path || "";
    if ($("configPath").value) addToWorkspace($("configPath").value);
    if (s.interval) $("interval").value = s.interval;
    if (s.auto) $("auto").checked = true;
    if (s.notify) $("notify").checked = true;
    if (s.groupBy) $("groupBy").checked = true;
    if (s.hideMuted) $("hideMuted").checked = true;
    updateHint();
    await refreshStacks();
    if (s.stack) $("stack").value = s.stack;
    if ($("auto").checked) setAutoRefresh(true);
    setView(s.view || "fleet"); // reopen on the last-used view

    if (IS_MOCK) {
      // Preview: add fake window controls and auto-run with sample data.
      const tl = document.createElement("div");
      tl.className = "fake-traffic";
      tl.innerHTML = "<i></i><i></i><i></i>";
      document.querySelector(".titlebar").appendChild(tl);
      run();
    } else if (startup.autoRun && startup.path) {
      run();
    }
  }

  /* ---------------- mock data (preview only) ---------------- */
  function mockReport() {
    const f = (check, target, status, message) => ({ check, target, status, message });
    const findings = [
      f("stream", "https://live.example.com/edge/master.m3u8", "BAD", "live-edge stalled for 47s (threshold 30s), ladder 3/4 variants"),
      f("haproxy", "lb-01:8404/be_ingest", "BAD", "backend has no available server (2 DOWN)"),
      f("postgres", "pg-01:5432", "ERROR", "connection failed: dial tcp 10.0.3.11:5432: i/o timeout"),
      f("certs", "api.example.com:443", "ERROR", "TLS handshake failed: connection refused"),
      f("nats", "nats-02:8222", "WARN", "peer lagging by 1420 on the raft meta-group (threshold 1000)"),
      f("redis", "redis-cache-01:6379", "WARN", "used_memory 82% of maxmemory (threshold 80%)"),
      f("certs", "cdn.example.com:443", "WARN", "expires in 12 days (2026-08-05, CN=*.example.com)"),
      f("dns", "example.com @ 1.1.1.1", "WARN", "TTL 30s below the threshold (60s)"),
      f("consul", "consul-01:8500", "OK", "leader present, 5/5 peers, 0 critical checks"),
      f("nats", "nats-01:8222", "OK", "meta-leader present, 3/3 peers, versions aligned"),
      f("certs", "www.example.com:443", "OK", "expires in 68 days (2026-09-30, CN=*.example.com)"),
      f("http", "https://example.com/health", "OK", "HTTP 200, 142ms"),
      f("redis", "redis-session-01:6379", "OK", "role master, link up, last RDB ok"),
      f("tcp", "smtp.example.com:587", "OK", "connected in 38ms, expected banner"),
      f("postgres", "pg-02:5432", "OK", "primary, 2 replicas in sync, 41% connections"),
    ];
    const count = (s) => findings.filter((x) => x.status === s).length;
    return {
      configPath: "/home/ops/checkfleet.yml",
      stack: "prod",
      modules: ["certs", "http", "nats", "haproxy", "stream", "postgres", "consul", "redis", "dns", "tcp"],
      findings,
      ok: count("OK"), warn: count("WARN"), bad: count("BAD"), error: count("ERROR"),
      worst: "ERROR",
      durationMs: 486,
      started: new Date().toISOString(),
      changes: [
        { check: "postgres", target: "pg-01:5432", from: "OK", to: "ERROR", kind: "new" },
        { check: "redis", target: "redis-cache-01:6379", from: "OK", to: "WARN", kind: "new" },
        { check: "certs", target: "old.example.com:443", from: "BAD", to: "OK", kind: "resolved" },
      ],
    };
  }

  document.addEventListener("DOMContentLoaded", init);
})();
