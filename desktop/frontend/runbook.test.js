// Headless unit tests for the finding runbook hints (CF-124).
// Pure string logic — run with: node --test desktop/frontend/
const test = require("node:test");
const assert = require("node:assert/strict");
const R = require("./dist/runbook.js");

test("a finding with no hints produces nothing", () => {
  assert.equal(R.has({ check: "http", target: "a" }), false);
  assert.equal(R.block({ check: "http", target: "a" }), "");
  assert.equal(R.block({ runbook: "   ", remediation: "" }), "");
  assert.equal(R.block(null), "");
});

test("both hints render as note then runbook link", () => {
  const out = R.block({ remediation: "Renew and reload", runbook: "https://wiki/tls" });
  assert.match(out, /What to do/);
  assert.match(out, /Renew and reload — <a href="#" class="runbook-link" data-runbook="https:\/\/wiki\/tls">Open runbook<\/a>/);
});

test("either hint alone is enough", () => {
  assert.match(R.block({ remediation: "Escalate to ops" }), /Escalate to ops/);
  const only = R.block({ runbook: "https://wiki/x" });
  assert.match(only, /Open runbook/);
  assert.doesNotMatch(only, /—/); // no separator with a single part
});

test("a non-http runbook is inert text, never an anchor", () => {
  const out = R.block({ runbook: "javascript:alert(1)" });
  assert.doesNotMatch(out, /<a /);
  assert.match(out, /javascript:alert\(1\)/);
  assert.equal(R.linkable("javascript:alert(1)"), false);
  assert.equal(R.linkable("https://ok"), true);
  assert.equal(R.linkable("HTTP://ok"), true);
});

test("hint text is escaped", () => {
  const out = R.block({ remediation: '<img src=x onerror="alert(1)">' });
  assert.doesNotMatch(out, /<img/);
  assert.match(out, /&lt;img/);
});

test("a quote in the runbook cannot break out of the attribute", () => {
  const out = R.block({ runbook: 'https://wiki/x" onclick="alert(1)' });
  assert.doesNotMatch(out, /onclick="alert/);
  assert.match(out, /&quot;/);
});
