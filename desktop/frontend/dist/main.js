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
    DefaultConfigPath: async () => "/Users/allan/hiway/checkfleet.yml",
    ListStacks: async () => ["prod-cologno", "prod-stnx", "staging"],
    OpenConfigDialog: async () => "/Users/allan/hiway/checkfleet.yml",
    SaveReport: async (fmt) => "~/checkfleet-report." + (fmt === "json" ? "json" : "md"),
    RunChecks: async () => mockReport(),
  };

  /* ---------------- state ---------------- */
  let report = null;
  let timer = null;

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
      setStatus("errore di configurazione");
      return;
    }

    // summary
    summary.hidden = false;
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
      .map((m) => `<span class="chip">${escapeHtml(m)}</span>`).join("");

    // table (preserve backend worst-first order)
    const q = $("filter").value.trim().toLowerCase();
    const min = $("minsev").value;
    const visible = findings.filter((f) => {
      if (!severityAllowed(f.status, min)) return false;
      if (!q) return true;
      return (f.check + " " + f.target + " " + f.message).toLowerCase().includes(q);
    });

    rows.innerHTML = visible.map((f) => `
      <tr>
        <td><span class="badge ${f.status}">${f.status}</span></td>
        <td class="cell-check">${escapeHtml(f.check)}</td>
        <td class="cell-target">${escapeHtml(f.target)}</td>
        <td class="cell-msg">${escapeHtml(f.message)}</td>
      </tr>`).join("");

    empty.style.display = visible.length ? "none" : "flex";
    if (!visible.length) $("emptyText").textContent = "Nessun finding con questi filtri.";

    const can = findings.length > 0;
    $("expmd").disabled = !can;
    $("expjson").disabled = !can;
    setStatus(`${findings.length} finding · ${report.ok} OK / ${report.warn} WARN / ${report.bad} BAD / ${report.error} ERROR`);
  }

  function setStatus(t) { $("statusText").textContent = t; }

  function escapeHtml(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }

  /* ---------------- actions ---------------- */
  async function run() {
    const btn = $("run");
    btn.disabled = true;
    setStatus("esecuzione dei check…");
    try {
      report = await Backend.RunChecks($("configPath").value, $("stack").value);
    } catch (e) {
      report = { err: String(e) };
    }
    btn.disabled = false;
    render();
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
      if (path) setStatus("salvato: " + path);
    } catch (e) { setStatus("errore export: " + e); }
  }

  function toggleTheme() {
    const root = document.documentElement;
    const next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
    root.setAttribute("data-theme", next);
    try { localStorage.setItem("cf-theme", next); } catch (_) {}
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
    $("configPath").addEventListener("change", () => { updateHint(); refreshStacks(); });
    $("filter").addEventListener("input", render);
    $("minsev").addEventListener("change", render);
    $("auto").addEventListener("change", (e) => setAutoRefresh(e.target.checked));
    $("interval").addEventListener("change", () => { if ($("auto").checked) setAutoRefresh(true); });
    $("expmd").addEventListener("click", () => save("markdown"));
    $("expjson").addEventListener("click", () => save("json"));
    $("theme").addEventListener("click", toggleTheme);
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
    const def = await Backend.DefaultConfigPath();
    if (def) $("configPath").value = def;
    updateHint();
    await refreshStacks();

    // In preview mode, add fake window controls and auto-run with sample data.
    if (IS_MOCK) {
      const tl = document.createElement("div");
      tl.className = "fake-traffic";
      tl.innerHTML = "<i></i><i></i><i></i>";
      document.querySelector(".titlebar").appendChild(tl);
      run();
    }
  }

  /* ---------------- mock data (preview only) ---------------- */
  function mockReport() {
    const f = (check, target, status, message) => ({ check, target, status, message });
    const findings = [
      f("stream", "https://live.hiway.media/edge/master.m3u8", "BAD", "live-edge fermo da 47s (soglia 30s), ladder 3/4 varianti"),
      f("haproxy", "lb-cologno-01:8404/be_ingest", "BAD", "backend senza server disponibili (2 DOWN)"),
      f("postgres", "pg-cologno-01:5432", "ERROR", "connessione fallita: dial tcp 10.0.3.11:5432: i/o timeout"),
      f("certs", "api.hiway.media:443", "ERROR", "handshake TLS fallito: connection refused"),
      f("nats", "nats-stnx-02:8222", "WARN", "peer in lag di 1420 sul raft meta-group (soglia 1000)"),
      f("redis", "redis-cache-01:6379", "WARN", "used_memory 82% di maxmemory (soglia 80%)"),
      f("certs", "cdn.hiway.media:443", "WARN", "scade tra 12 giorni (2026-08-05, CN=*.hiway.media)"),
      f("dns", "hiway.media @ 1.1.1.1", "WARN", "TTL 30s sotto la soglia (60s)"),
      f("consul", "consul-cologno:8500", "OK", "leader presente, 5/5 peer, 0 check critical"),
      f("nats", "nats-stnx-01:8222", "OK", "meta-leader presente, 3/3 peer, versioni allineate"),
      f("certs", "www.hiway.media:443", "OK", "scade tra 68 giorni (2026-09-30, CN=*.hiway.media)"),
      f("http", "https://hiway.media/health", "OK", "HTTP 200, 142ms"),
      f("redis", "redis-session-01:6379", "OK", "role master, link up, ultimo RDB ok"),
      f("tcp", "smtp.hiway.media:587", "OK", "connesso in 38ms, banner atteso"),
      f("postgres", "pg-stnx-01:5432", "OK", "primary, 2 repliche in pari, 41% connessioni"),
    ];
    const count = (s) => findings.filter((x) => x.status === s).length;
    return {
      configPath: "/Users/allan/hiway/checkfleet.yml",
      stack: "prod-cologno",
      modules: ["certs", "http", "nats", "haproxy", "stream", "postgres", "consul", "redis", "dns", "tcp"],
      findings,
      ok: count("OK"), warn: count("WARN"), bad: count("BAD"), error: count("ERROR"),
      worst: "ERROR",
      durationMs: 486,
      started: new Date().toISOString(),
    };
  }

  document.addEventListener("DOMContentLoaded", init);
})();
