// presets.js — saved views for the fleet toolbar (CF-108).
//
// A "view" (preset) is a named bundle of the toolbar state you look at often:
// which stack, the text filter, the min-severity, whether findings are grouped,
// and which top-level view is open. Chips switch between them in one click; the
// whole set imports/exports as JSON. All state lives in localStorage — no files,
// no secrets (a preset stores UI knobs, never config contents or credentials).
//
// This module is pure data logic, no DOM, so it is unit-testable headlessly
// (see presets.test.js). Same UMD wrapper as charts.js: window.CFPresets in the
// browser, module.exports under Node.
(function (root, factory) {
  "use strict";
  const api = factory();
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else root.CFPresets = api;
})(typeof self !== "undefined" ? self : this, function () {
  "use strict";

  const SEVS = ["ok", "warn", "bad", "error"];
  const VIEWS = ["fleet", "dashboard", "config"];
  const MAX = 24; // a sensible cap so the chip row never runs away

  const str = (v) => (typeof v === "string" ? v : "");
  const oneOf = (v, allowed, fallback) => (allowed.indexOf(v) >= 0 ? v : fallback);

  // normalize turns loose UI state into a clean preset record. `name` is trimmed;
  // unknown minsev/view fall back to the permissive defaults, group is coerced to
  // a real boolean. Returns null when there is no usable name.
  function normalize(name, state) {
    const nm = str(name).trim();
    if (!nm) return null;
    state = state || {};
    return {
      name: nm,
      stack: str(state.stack),
      filter: str(state.filter),
      minsev: oneOf(state.minsev, SEVS, "ok"),
      group: !!state.group,
      view: oneOf(state.view, VIEWS, "fleet"),
    };
  }

  // key is the case-insensitive identity of a preset name, so "Prod" and "prod"
  // are the same view rather than two near-duplicate chips.
  const key = (name) => str(name).trim().toLowerCase();

  // upsert adds a preset or replaces the one with the same name (case-insensitive),
  // keeping list order stable (a replace updates in place, a new one appends).
  // Returns a new array; the input is never mutated. Caps the set at MAX.
  function upsert(list, preset) {
    const arr = Array.isArray(list) ? list.slice() : [];
    if (!preset || !preset.name) return arr;
    const i = arr.findIndex((p) => key(p.name) === key(preset.name));
    if (i >= 0) arr[i] = preset;
    else arr.push(preset);
    return arr.slice(0, MAX);
  }

  // remove drops the preset with the given name (case-insensitive). New array.
  function remove(list, name) {
    const arr = Array.isArray(list) ? list : [];
    return arr.filter((p) => key(p.name) !== key(name));
  }

  function find(list, name) {
    const arr = Array.isArray(list) ? list : [];
    return arr.find((p) => key(p.name) === key(name)) || null;
  }

  // matches reports whether the current UI state already equals a preset — used
  // to light up the active chip. Compares only the fields a preset owns.
  function matches(preset, state) {
    if (!preset) return false;
    const n = normalize(preset.name, preset);
    const s = normalize(preset.name, state); // borrow the name so normalize accepts it
    return !!n && !!s &&
      n.stack === s.stack && n.filter === s.filter &&
      n.minsev === s.minsev && n.group === s.group && n.view === s.view;
  }

  // sanitize keeps only well-formed presets, dropping junk and de-duplicating by
  // name — the gatekeeper for anything read from storage or an imported file.
  function sanitize(list) {
    const out = [];
    const seen = new Set();
    (Array.isArray(list) ? list : []).forEach((p) => {
      const n = p && normalize(p.name, p);
      if (!n || seen.has(key(n.name))) return;
      seen.add(key(n.name));
      out.push(n);
    });
    return out.slice(0, MAX);
  }

  function serialize(list) {
    return JSON.stringify(sanitize(list), null, 2);
  }

  // parse is tolerant: it accepts a bare array or a {presets:[…]} envelope, and
  // never throws — bad JSON yields an empty set so a broken import can't wedge
  // the app.
  function parse(json) {
    let data;
    try { data = JSON.parse(json); } catch (_) { return []; }
    if (Array.isArray(data)) return sanitize(data);
    if (data && Array.isArray(data.presets)) return sanitize(data.presets);
    return [];
  }

  return { SEVS, VIEWS, MAX, normalize, key, upsert, remove, find, matches, sanitize, serialize, parse };
});
