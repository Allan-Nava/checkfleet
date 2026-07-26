// audit.js — a local action log for the incident workflow (CF-114).
//
// Every mute/unmute/note/issue action drops a line here, so there's a timeline
// of "what did we do about the fleet" you can glance at or export (JSON /
// Markdown) for a handover or a postmortem. Local-only, newest-first, bounded —
// operator history, no secrets.
//
// Pure data logic, no DOM, no clock of its own (the caller passes `at`), so it
// is deterministic and unit-testable headlessly (see audit.test.js). Same UMD
// wrapper as the other frontend modules.
(function (root, factory) {
  "use strict";
  const api = factory();
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else root.CFAudit = api;
})(typeof self !== "undefined" ? self : this, function () {
  "use strict";

  const MAX = 200; // keep the log bounded; the tail ages out
  const str = (v) => (typeof v === "string" ? v : "");
  const num = (v) => (typeof v === "number" && isFinite(v) ? v : 0);

  // add prepends an entry (newest first) and caps the log. An empty kind is
  // ignored. Returns a new array; the input is never mutated.
  function add(log, kind, target, detail, at) {
    const arr = Array.isArray(log) ? log.slice() : [];
    if (!str(kind)) return arr;
    arr.unshift({ at: num(at), kind: str(kind), target: str(target), detail: str(detail) });
    return arr.slice(0, MAX);
  }

  function sanitize(log) {
    return (Array.isArray(log) ? log : [])
      .filter((e) => e && str(e.kind))
      .map((e) => ({ at: num(e.at), kind: str(e.kind), target: str(e.target), detail: str(e.detail) }))
      .slice(0, MAX);
  }

  function count(log) { return sanitize(log).length; }

  const iso = (at) => (at ? new Date(at).toISOString() : "");

  function toJSON(log) { return JSON.stringify(sanitize(log), null, 2); }

  // toMarkdown renders a table for a handover/postmortem. Pipes in free text are
  // escaped so a note can't break the table.
  function toMarkdown(log) {
    const esc = (s) => str(s).replace(/\|/g, "\\|").replace(/\n/g, " ");
    const rows = sanitize(log).map((e) =>
      `| ${iso(e.at)} | ${esc(e.kind)} | ${esc(e.target)} | ${esc(e.detail)} |`);
    return "| Time (UTC) | Action | Target | Detail |\n|---|---|---|---|\n" + rows.join("\n");
  }

  return { MAX, add, sanitize, count, iso, toJSON, toMarkdown };
});
