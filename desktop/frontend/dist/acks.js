// acks.js — acknowledge / snooze / mute for findings (CF-110).
//
// Muting a finding silences it for a while: it drops out of the "things to look
// at" and (from CF-111) stops the background monitor re-notifying, until the
// mute expires. A mute is keyed by config#check#target so it follows the exact
// target across runs, and stored locally only — no config changes, no secrets,
// just an operator's "I know, leave me alone until 6pm".
//
// Pure data logic, no DOM and no clock of its own (the caller passes `now`), so
// it is deterministic and unit-testable headlessly (see acks.test.js). Same UMD
// wrapper as charts.js / presets.js.
(function (root, factory) {
  "use strict";
  const api = factory();
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  else root.CFAcks = api;
})(typeof self !== "undefined" ? self : this, function () {
  "use strict";

  // UNTIL_RECOVERY is the sentinel for "mute until the finding goes green again"
  // (0 = no time expiry). CF-111 clears these when the target recovers; by time
  // alone they never expire.
  const UNTIL_RECOVERY = 0;

  // A record separator that can't appear in a config path, check name or target.
  const SEP = "\u001f";
  function key(config, check, target) {
    return [config || "", check || "", target || ""].join(SEP);
  }

  const num = (v) => (typeof v === "number" && isFinite(v) ? v : 0);
  const str = (v) => (typeof v === "string" ? v : "");

  // normalize cleans one mute record; returns null without a key.
  function normalize(rec) {
    if (!rec || !str(rec.key)) return null;
    return {
      key: rec.key,
      until: num(rec.until), // ms epoch, or UNTIL_RECOVERY (0)
      reason: str(rec.reason),
      at: num(rec.at),
    };
  }

  // isMuted reports whether `key` is actively muted at `now` (ms). An
  // until-recovery mute is always active until cleared; a timed one until `until`.
  function isMuted(acks, k, now) {
    const rec = acks && acks[k];
    if (!rec) return false;
    if (rec.until === UNTIL_RECOVERY) return true;
    return rec.until > now;
  }

  // prune drops timed mutes that have expired at `now`, keeping until-recovery
  // ones. Returns a new object; input is not mutated.
  function prune(acks, now) {
    const out = {};
    Object.keys(acks || {}).forEach((k) => {
      const r = normalize(acks[k]);
      if (!r) return;
      if (r.until === UNTIL_RECOVERY || r.until > now) out[k] = r;
    });
    return out;
  }

  // mute upserts a record. `until` is an absolute ms epoch (or UNTIL_RECOVERY).
  function mute(acks, rec) {
    const r = normalize(rec);
    const out = Object.assign({}, acks);
    if (r) out[r.key] = r;
    return out;
  }

  function unmute(acks, k) {
    const out = Object.assign({}, acks);
    delete out[k];
    return out;
  }

  // activeCount counts currently-muted keys at `now`.
  function activeCount(acks, now) {
    return Object.keys(acks || {}).filter((k) => isMuted(acks, k, now)).length;
  }

  // durationUntil turns a snooze choice into an absolute expiry (ms). Known
  // choices: "1h", "8h", "24h", "recovery". Unknown → 1h, a safe short default.
  const HOURS = { "1h": 1, "8h": 8, "24h": 24 };
  function durationUntil(choice, now) {
    if (choice === "recovery") return UNTIL_RECOVERY;
    const h = HOURS[choice] || 1;
    return now + h * 3600 * 1000;
  }

  // describe is the chip/tooltip label for a mute at `now`.
  function describe(rec, now) {
    const r = normalize(rec);
    if (!r) return "";
    if (r.until === UNTIL_RECOVERY) return "muted until recovery";
    const mins = Math.max(0, Math.round((r.until - now) / 60000));
    if (mins <= 0) return "mute expired";
    if (mins < 60) return "muted · " + mins + "m left";
    return "muted · " + Math.round(mins / 60) + "h left";
  }

  return { UNTIL_RECOVERY, SEP, key, normalize, isMuted, prune, mute, unmute, activeCount, durationUntil, describe };
});
