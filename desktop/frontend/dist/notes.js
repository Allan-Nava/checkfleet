// notes.js — operator notes on findings (CF-112).
//
// A note pins context to a finding: who's on it and what's going on ("Marco —
// rotating this cert tomorrow"). Keyed by config#check#target like mutes, stored
// locally only — it's an operator annotation, never a config change, and holds
// no secrets (just the text you type).
//
// Pure data logic, no DOM, unit-testable headlessly (see notes.test.js). Same
// UMD wrapper as charts.js / presets.js / acks.js. Notes reuse CFAcks.key for
// their identity, so a finding's mute and note share one key.
(function (root, factory) {
  "use strict";
  const api = factory();
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else root.CFNotes = api;
})(typeof self !== "undefined" ? self : this, function () {
  "use strict";

  const str = (v) => (typeof v === "string" ? v : "");
  const num = (v) => (typeof v === "number" && isFinite(v) ? v : 0);

  // normalize trims a note; an empty owner AND empty text mean "no note" (null),
  // so clearing both fields deletes the annotation rather than storing a blank.
  function normalize(rec) {
    if (!rec || !str(rec.key)) return null;
    const owner = str(rec.owner).trim();
    const text = str(rec.text).trim();
    if (!owner && !text) return null;
    return { key: rec.key, owner, text, at: num(rec.at) };
  }

  // set upserts a note, or deletes it when the note is empty. Immutable.
  function set(notes, rec) {
    const out = Object.assign({}, notes);
    const r = normalize(rec);
    if (r) out[r.key] = r;
    else if (rec && str(rec.key)) delete out[rec.key];
    return out;
  }

  function remove(notes, key) {
    const out = Object.assign({}, notes);
    delete out[key];
    return out;
  }

  function get(notes, key) { return (notes && notes[key]) || null; }
  function has(notes, key) { return !!(notes && notes[key]); }
  function count(notes) { return Object.keys(notes || {}).length; }

  // describe is the chip tooltip: "owner: text", or just one of them.
  function describe(rec) {
    const r = normalize(rec);
    if (!r) return "";
    if (r.owner && r.text) return r.owner + ": " + r.text;
    return r.owner || r.text;
  }

  // sanitize keeps only well-formed notes — the gatekeeper for storage reads.
  function sanitize(notes) {
    const out = {};
    Object.keys(notes || {}).forEach((k) => {
      const r = normalize(notes[k]);
      if (r) out[k] = r;
    });
    return out;
  }

  return { normalize, set, remove, get, has, count, describe, sanitize };
});
