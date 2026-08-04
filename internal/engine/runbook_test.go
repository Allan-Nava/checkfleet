package engine

import "testing"

func runbookFindings() []Finding {
	return []Finding{
		{Check: "certs", Target: "api.example.com:443", Status: BAD, Message: "expires in 2 days"},
		{Check: "postgres", Target: "db-01:5432", Status: WARN, Message: "replication lag 30s"},
		{Check: "http", Target: "https://ok.example", Status: OK, Message: "200 in 12ms"},
	}
}

func TestApplyRunbooksMatchesByCheckAndTarget(t *testing.T) {
	got := ApplyRunbooks(runbookFindings(), []RunbookRule{
		{Check: "certs", Runbook: "https://wiki/tls", Remediation: "Renew and reload"},
		{Check: "postgres", Target: "db-*", Remediation: "Check lag before failover"},
	})
	if got[0].Runbook != "https://wiki/tls" || got[0].Remediation != "Renew and reload" {
		t.Errorf("certs finding = %+v, want both hints", got[0])
	}
	if got[1].Remediation != "Check lag before failover" {
		t.Errorf("postgres remediation = %q, want the target-glob rule to match", got[1].Remediation)
	}
	if got[1].Runbook != "" {
		t.Errorf("postgres runbook = %q, want empty (the rule sets none)", got[1].Runbook)
	}
}

func TestApplyRunbooksSkipsOK(t *testing.T) {
	got := ApplyRunbooks(runbookFindings(), []RunbookRule{
		{Runbook: "https://wiki/all", Remediation: "Escalate to ops"},
	})
	if got[2].Runbook != "" || got[2].Remediation != "" {
		t.Errorf("OK finding = %+v, want no hints (nothing to do on green)", got[2])
	}
	if got[0].Runbook != "https://wiki/all" {
		t.Errorf("BAD finding = %+v, want the catch-all rule applied", got[0])
	}
}

func TestApplyRunbooksFirstNonEmptyWinsPerField(t *testing.T) {
	// The specific rule supplies only the runbook; the catch-all below it must
	// still fill in the remediation it leaves empty.
	got := ApplyRunbooks(runbookFindings(), []RunbookRule{
		{Check: "certs", Runbook: "https://wiki/tls"},
		{Runbook: "https://wiki/generic", Remediation: "Escalate to ops"},
	})
	if got[0].Runbook != "https://wiki/tls" {
		t.Errorf("runbook = %q, want the first rule to win", got[0].Runbook)
	}
	if got[0].Remediation != "Escalate to ops" {
		t.Errorf("remediation = %q, want the catch-all to fill the gap", got[0].Remediation)
	}
}

func TestApplyRunbooksNoRulesLeavesFindingsUntouched(t *testing.T) {
	in := runbookFindings()
	got := ApplyRunbooks(in, nil)
	if len(got) != len(in) {
		t.Fatalf("len = %d, want %d", len(got), len(in))
	}
	for i := range got {
		if got[i] != in[i] {
			t.Errorf("finding %d changed: %+v", i, got[i])
		}
	}
}

func TestApplyRunbooksDoesNotMutateInput(t *testing.T) {
	in := runbookFindings()
	ApplyRunbooks(in, []RunbookRule{{Runbook: "https://wiki/all"}})
	if in[0].Runbook != "" {
		t.Errorf("input finding was mutated: %+v", in[0])
	}
}

func TestApplyRunbooksNonMatchingRuleIsInert(t *testing.T) {
	got := ApplyRunbooks(runbookFindings(), []RunbookRule{
		{Check: "redis", Runbook: "https://wiki/redis"},
		{Check: "postgres", Target: "cache-*", Remediation: "wrong target"},
	})
	for _, f := range got {
		if f.Runbook != "" || f.Remediation != "" {
			t.Errorf("finding %s/%s got hints from a non-matching rule: %+v", f.Check, f.Target, f)
		}
	}
}
