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
    AddEndpoint: async (yaml, kind, value, recordType, expectStatus) => {
      // Preview-only textual merge (the real backend edits the YAML node tree).
      const nl = "\n";
      let block;
      if (kind === "certs") block = "  certs:" + nl + "    targets: [" + value + "]" + nl;
      else if (kind === "tcp") block = "  tcp:" + nl + "    targets:" + nl + "      - address: " + value + nl;
      else if (kind === "dns") block = "  dns:" + nl + "    targets:" + nl + "      - name: " + value +
        (recordType && recordType !== "A" ? nl + "        type: " + recordType : "") + nl;
      else block = "  http:" + nl + "    targets:" + nl + "      - url: " + value +
        (expectStatus ? nl + "        expect_status: " + expectStatus : "") + nl;
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
    ScheduleSnippet: async (path, interval) =>
      "# cron — run every 5 min:\n*/5 * * * * checkfleet check all --config " +
      (path || "checkfleet.yml") + " --exit-on-bad\n\n" +
      "# or run continuously as a Prometheus exporter:\ncheckfleet serve --config " +
      (path || "checkfleet.yml") + " --interval " + (interval || "60s") + " --listen :9876",
  };

  /* ---------------- state ---------------- */
  let report = null;
  let timer = null;
  let visibleFindings = [];
  let editorOn = false;
  let dashboardOn = false;
  let moduleTrend = { modules: [], runs: [] };

  /* ---------------- rendering ---------------- */
  function severityAllowed(status, min) {
    return RANK[status] >= RANK[min.toUpperCase()];
  }

  function render() {
    const summary = $("summary");
    const empty = $("empty");
    const rows = $("rows");

    if (!report) {
      summary.hidden = true;
      empty.style.display = "flex";
      rows.innerHTML = "";
      return;
    }
    if (report.err) {
      summary.hidden = true;
      empty.style.display = "flex";
      $("emptyText").innerHTML = "⚠️ " + escapeHtml(report.err);
      rows.innerHTML = "";
      setStatus("configuration error");
      return;
    }

    // summary (kept hidden while the editor/dashboard views own the screen)
    summary.hidden = editorOn || dashboardOn;
    const worst = report.worst || "OK";
    const worstEl = $("worst");
    worstEl.className = "worst s-" + worst;
    $("worstLabel").textContent = worst;
    $("cOK").textContent = report.ok;
    $("cWARN").textContent = report.warn;
    $("cBAD").textContent = report.bad;
    $("cERROR").textContent = report.error;
    const findings = report.findings || [];
    $("mTotal").textContent = findings.length;
    $("mDur").textContent = report.durationMs != null ? report.durationMs + " ms" : "—";
    $("mStarted").textContent = report.started ? new Date(report.started).toLocaleTimeString() : "—";
    $("mModules").innerHTML = (report.modules || [])
      .map((m) => `<span class="chip" data-mod="${escapeHtml(m)}" title="Explain ${escapeHtml(m)}">${escapeHtml(m)}</span>`).join("");

    // table (preserve backend worst-first order)
    const q = $("filter").value.trim().toLowerCase();
    const min = $("minsev").value;
    const visible = findings.filter((f) => {
      if (!severityAllowed(f.status, min)) return false;
      if (!q) return true;
      return (f.check + " " + f.target + " " + f.message).toLowerCase().includes(q);
    });
    visibleFindings = visible;

    const findingRow = (f, i) => `
      <tr data-i="${i}"${$("groupBy").checked ? ` data-grp="${escapeHtml(f.check)}"` : ""}>
        <td><span class="badge ${f.status}">${f.status}</span></td>
        <td class="cell-check">${escapeHtml(f.check)}</td>
        <td class="cell-target">${escapeHtml(f.target)}</td>
        <td class="cell-msg">${escapeHtml(f.message)}</td>
      </tr>`;

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
            <td colspan="4">
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

    empty.style.display = visible.length ? "none" : "flex";
    if (!visible.length) $("emptyText").textContent = "No findings match these filters.";

    const can = findings.length > 0;
    $("export").disabled = !can;
    const changes = report.changes || [];
    $("changes").disabled = changes.length === 0;
    $("changes").textContent = changes.length ? `Changes (${changes.length})` : "Changes";
    setStatus(`${findings.length} findings · ${report.ok} OK / ${report.warn} WARN / ${report.bad} BAD / ${report.error} ERROR`);
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
    const btn = $("run");
    btn.disabled = true;
    setStatus("running checks…");
    try {
      report = await Backend.RunChecks($("configPath").value, $("stack").value);
    } catch (e) {
      report = { err: String(e) };
    }
    btn.disabled = false;
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
      }));
    } catch (_) {}
  }
  function loadSettings() {
    try { return JSON.parse(localStorage.getItem("cf-settings") || "{}"); } catch (_) { return {}; }
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

  function setAutoRefresh(on) {
    if (timer) { clearInterval(timer); timer = null; }
    if (on) {
      const secs = parseInt($("interval").value, 10) || 30;
      timer = setInterval(run, secs * 1000);
    }
  }

  async function save(fmt) {
    try {
      const path = await Backend.SaveReport(fmt);
      if (path) setStatus("saved: " + path);
    } catch (e) { setStatus("export error: " + e); }
  }

  function toggleTheme() {
    const root = document.documentElement;
    const next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
    root.setAttribute("data-theme", next);
    try { localStorage.setItem("cf-theme", next); } catch (_) {}
  }

  /* ---------------- config editor ---------------- */
  function setEditor(on) {
    editorOn = on;
    if (on && dashboardOn) setDashboard(false);
    $("editor").hidden = !on;
    $("findingsBar").hidden = on;
    $("tableWrap").hidden = on;
    $("summary").hidden = on || !report;
    $("cfgToggle").classList.toggle("active", on);
    if (on) loadConfigText();
  }

  /* ---------------- dashboard (CF-91) ---------------- */
  function setDashboard(on) {
    dashboardOn = on;
    if (on && editorOn) setEditor(false);
    $("dashboard").hidden = !on;
    $("findingsBar").hidden = on;
    $("tableWrap").hidden = on;
    $("summary").hidden = on || !report;
    $("dashToggle").classList.toggle("active", on);
    if (on) renderDashboard();
  }

  // renderDashboard pulls the persisted history (survives restarts) and draws the
  // stacked-area, donut and worst-status band. The donut shows the live run when
  // there is one, otherwise the newest history point.
  async function renderDashboard() {
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
    http:  { label: "URL",       ph: "https://example.com/health" },
    certs: { label: "Host:port", ph: "example.com:443" },
    tcp:   { label: "Host:port", ph: "db.internal:5432" },
    dns:   { label: "Name",      ph: "example.com" },
  };

  function syncAddForm() {
    const kind = $("epKind").value;
    const h = EP_HINTS[kind] || EP_HINTS.http;
    $("epValueLabel").textContent = h.label;
    $("epValue").placeholder = h.ph;
    $("epExtraStatus").hidden = kind !== "http";
    $("epExtraType").hidden = kind !== "dns";
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
        $("cfgText").value, kind, value, $("epType").value, Number($("epStatus").value) || 0);
      $("cfgText").value = updated;
      $("epValue").value = "";
      toggleAddForm(false);
      cfgMessage("Added " + kind + " endpoint — review and Save.", false);
    } catch (err) { cfgMessage("Add failed: " + err, true); }
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
      await refreshStacks();
    } catch (e) { cfgMessage("Save error: " + e, true); }
  }

  /* ---------------- drawer (detail / explain / validate) ---------------- */
  function openDrawer(title, bodyHTML) {
    $("drawerTitle").textContent = title;
    $("drawerBody").innerHTML = bodyHTML;
    $("drawer").hidden = false;
    $("drawerScrim").hidden = false;
  }
  function closeDrawer() {
    $("drawer").hidden = true;
    $("drawerScrim").hidden = true;
  }

  function showFindingDetail(f) {
    const text = `${f.status} ${f.check} ${f.target} — ${f.message}`;
    openDrawer("Finding", `
      <div class="kv"><span>Status</span><span class="badge ${f.status}">${f.status}</span></div>
      <div class="kv"><span>Check</span><b class="mono">${escapeHtml(f.check)}</b></div>
      <div class="kv"><span>Target</span><b class="mono">${escapeHtml(f.target)}</b></div>
      <p class="drawer-msg">${escapeHtml(f.message)}</p>
      <button class="btn copy-btn" data-copy="${escapeHtml(text)}">Copy</button>`);
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

  /* ---------------- wiring ---------------- */
  function bind() {
    $("run").addEventListener("click", run);
    $("pick").addEventListener("click", async () => {
      try {
        const p = await Backend.OpenConfigDialog();
        if (p) { $("configPath").value = p; updateHint(); await refreshStacks(); }
      } catch (_) {}
    });
    $("configPath").addEventListener("change", () => { updateHint(); refreshStacks(); saveSettings(); });
    $("filter").addEventListener("input", render);
    $("minsev").addEventListener("change", render);
    $("stack").addEventListener("change", saveSettings);
    $("notify").addEventListener("change", saveSettings);
    $("auto").addEventListener("change", (e) => { setAutoRefresh(e.target.checked); saveSettings(); });
    $("interval").addEventListener("change", () => { if ($("auto").checked) setAutoRefresh(true); saveSettings(); });
    $("export").addEventListener("click", () => save($("expfmt").value));
    $("theme").addEventListener("click", toggleTheme);
    $("validate").addEventListener("click", runValidate);
    $("changes").addEventListener("click", showChanges);
    $("trend").addEventListener("click", showTrend);
    $("groupBy").addEventListener("change", () => { render(); saveSettings(); });
    $("dashToggle").addEventListener("click", () => setDashboard(dashboardOn ? false : true));
    $("dashRefresh").addEventListener("click", renderDashboard);
    $("chartHeatmap").addEventListener("click", (e) => {
      const el = e.target.closest("[data-module]");
      if (el) showModuleDrill(el.getAttribute("data-module"));
    });
    $("cfgToggle").addEventListener("click", () => setEditor(editorOn ? false : true));
    $("cfgReload").addEventListener("click", loadConfigText);
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
    // Copy button inside the drawer.
    $("drawerBody").addEventListener("click", (e) => {
      const btn = e.target.closest(".copy-btn");
      if (btn) {
        try { navigator.clipboard.writeText(btn.dataset.copy); btn.textContent = "Copied"; } catch (_) {}
      }
    });
    $("drawerClose").addEventListener("click", closeDrawer);
    $("drawerScrim").addEventListener("click", closeDrawer);
    document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeDrawer(); });
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
    $("configPath").value = s.config || startup.path || "";
    if (s.interval) $("interval").value = s.interval;
    if (s.auto) $("auto").checked = true;
    if (s.notify) $("notify").checked = true;
    if (s.groupBy) $("groupBy").checked = true;
    updateHint();
    await refreshStacks();
    if (s.stack) $("stack").value = s.stack;
    if ($("auto").checked) setAutoRefresh(true);

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
