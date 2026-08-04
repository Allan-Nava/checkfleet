// runbook.js — operator hints on a finding (CF-124).
//
// The engine attaches `runbook` (a procedure URL) and `remediation` (a short
// "what to do" note) to findings above OK, from the `runbooks:` rules in the
// config. This module turns them into the drawer's "What to do" block, so an
// operator reading a BAD does not have to go and find the procedure.
//
// The URL comes from the operator's own config, but it lands in an anchor: only
// http(s) becomes clickable, anything else renders as inert text. A finding
// carrying no hints produces "" and the block disappears entirely.
//
// Pure string logic, no DOM, unit-testable headlessly (see runbook.test.js).
// Same UMD wrapper as charts.js / presets.js / acks.js / notes.js.
(function (root, factory) {
  "use strict";
  const api = factory();
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else root.CFRunbook = api;
})(typeof self !== "undefined" ? self : this, function () {
  "use strict";

  const str = (v) => (typeof v === "string" ? v.trim() : "");

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
  }

  // linkable reports whether a runbook URL is safe to put in an href.
  function linkable(url) {
    return /^https?:\/\//i.test(url);
  }

  // has reports whether a finding carries any hint at all.
  function has(f) {
    return !!(f && (str(f.runbook) || str(f.remediation)));
  }

  // block renders the drawer's "What to do" section, or "" when there is
  // nothing to show. The caller inserts it verbatim.
  function block(f) {
    if (!has(f)) return "";
    const parts = [];
    const note = str(f && f.remediation);
    const url = str(f && f.runbook);
    if (note) parts.push(escapeHtml(note));
    if (url) {
      parts.push(linkable(url)
        ? `<a href="#" class="runbook-link" data-runbook="${escapeHtml(url)}">Open runbook</a>`
        : escapeHtml(url));
    }
    return `<div class="hint-block"><span class="mute-label">What to do</span>` +
      `<p class="hint-text">${parts.join(" — ")}</p></div>`;
  }

  return { has, block, linkable, escapeHtml };
});
