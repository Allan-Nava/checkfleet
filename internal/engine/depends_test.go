package engine

import (
	"strings"
	"testing"
)

func deadHost() []Finding {
	return []Finding{
		{Check: "tcp", Target: "db-01:22", Status: BAD, Message: "connection refused"},
		{Check: "postgres", Target: "db-01:5432", Status: ERROR, Message: "connection failed"},
		{Check: "redis", Target: "db-01:6379", Status: ERROR, Message: "connection failed"},
		{Check: "http", Target: "https://web-01/", Status: BAD, Message: "500"},
	}
}

var sameHostRule = []DependsRule{{OnCheck: "tcp", SameHost: true}}

func TestDeadHostSuppressesItsConsequences(t *testing.T) {
	got := ApplyDependencies(deadHost(), sameHostRule)
	byKey := map[string]Finding{}
	for _, f := range got {
		byKey[f.Check] = f
	}
	for _, name := range []string{"postgres", "redis"} {
		f := byKey[name]
		if f.Status != Suppressed {
			t.Errorf("%s status = %s, want %s", name, f.Status, Suppressed)
		}
		if f.SuppressedBy != "tcp db-01:22" {
			t.Errorf("%s suppressed_by = %q", name, f.SuppressedBy)
		}
		if !strings.Contains(f.Message, "[suppressed by tcp db-01:22]") {
			t.Errorf("%s message not annotated: %q", name, f.Message)
		}
	}
	// The parent keeps its severity: it is the thing to page about.
	if byKey["tcp"].Status != BAD {
		t.Errorf("the parent was suppressed: %+v", byKey["tcp"])
	}
	// An unrelated host is untouched.
	if byKey["http"].Status != BAD || byKey["http"].SuppressedBy != "" {
		t.Errorf("an unrelated finding was suppressed: %+v", byKey["http"])
	}
}

// TestSuppressionNeverHidesAFinding is the trap this feature has to avoid. A
// finding that disappears is indistinguishable from a check that never ran, and
// "the fleet went quiet" is the worst way to learn about an outage.
func TestSuppressionNeverHidesAFinding(t *testing.T) {
	in := deadHost()
	got := ApplyDependencies(in, sameHostRule)
	if len(got) != len(in) {
		t.Fatalf("suppression dropped rows: %d → %d", len(in), len(got))
	}
	for _, f := range got {
		if f.Status == Suppressed && f.SuppressedBy == "" {
			t.Errorf("a downgraded finding with no explanation: %+v", f)
		}
	}
}

func TestAWarnParentSuppressesNothing(t *testing.T) {
	in := deadHost()
	in[0].Status = WARN // the tcp check is merely slow
	for _, f := range ApplyDependencies(in, sameHostRule) {
		if f.SuppressedBy != "" {
			t.Errorf("a WARN parent explained nothing away, yet %s was suppressed", f.Check)
		}
	}
}

func TestNoRulesIsInert(t *testing.T) {
	in := deadHost()
	got := ApplyDependencies(in, nil)
	for i := range got {
		if got[i] != in[i] {
			t.Errorf("finding %d changed with no rules: %+v", i, got[i])
		}
	}
}

func TestApplyDependenciesDoesNotMutateInput(t *testing.T) {
	in := deadHost()
	ApplyDependencies(in, sameHostRule)
	for _, f := range in {
		if f.SuppressedBy != "" {
			t.Errorf("the input was annotated in place: %+v", f)
		}
	}
}

func TestExplicitParentTarget(t *testing.T) {
	got := ApplyDependencies(deadHost(), []DependsRule{
		{Check: "postgres", OnCheck: "tcp", OnTarget: "db-01:*"},
	})
	var suppressed int
	for _, f := range got {
		if f.SuppressedBy != "" {
			suppressed++
			if f.Check != "postgres" {
				t.Errorf("the rule named postgres but suppressed %s", f.Check)
			}
		}
	}
	if suppressed != 1 {
		t.Errorf("suppressed %d findings, want 1", suppressed)
	}
}

// TestAFindingNeverSuppressesItself — a catch-all rule with same_host would
// otherwise let the tcp finding explain itself away, silencing the one row that
// says what actually happened.
func TestAFindingNeverSuppressesItself(t *testing.T) {
	got := ApplyDependencies(deadHost(), []DependsRule{{OnCheck: "tcp", SameHost: true}})
	for _, f := range got {
		if f.Check == "tcp" && f.SuppressedBy != "" {
			t.Errorf("the parent suppressed itself: %+v", f)
		}
	}
}

func TestValidateRejectsIncompleteRules(t *testing.T) {
	problems := ValidateDependencies([]DependsRule{
		{Check: "postgres"},                            // no on_check
		{Check: "redis", OnCheck: "tcp"},               // neither same_host nor on_target
		{Check: "tcp", OnCheck: "tcp", SameHost: true}, // depends on itself
	})
	if len(problems) < 3 {
		t.Fatalf("want three problems, got %v", problems)
	}
	joined := strings.Join(problems, "\n")
	for _, want := range []string{"on_check is required", "same_host", "depends on itself"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing a problem about %q:\n%s", want, joined)
		}
	}
}

// TestValidateDetectsACycle — a cycle would leave the outcome depending on the
// order findings happened to arrive in, so it is refused rather than resolved
// arbitrarily at run time.
func TestValidateDetectsACycle(t *testing.T) {
	problems := ValidateDependencies([]DependsRule{
		{Check: "a", OnCheck: "b", SameHost: true},
		{Check: "b", OnCheck: "c", SameHost: true},
		{Check: "c", OnCheck: "a", SameHost: true},
	})
	var found string
	for _, p := range problems {
		if strings.Contains(p, "cycle") {
			found = p
		}
	}
	if found == "" {
		t.Fatalf("no cycle reported: %v", problems)
	}
	// The message must show the loop, not just assert one exists.
	for _, node := range []string{"a", "b", "c"} {
		if !strings.Contains(found, node) {
			t.Errorf("the cycle message omits %q: %s", node, found)
		}
	}
}

func TestValidateAcceptsAHealthyChain(t *testing.T) {
	problems := ValidateDependencies([]DependsRule{
		{Check: "postgres", OnCheck: "tcp", SameHost: true},
		{Check: "redis", OnCheck: "tcp", SameHost: true},
	})
	if len(problems) != 0 {
		t.Errorf("a plain chain should validate: %v", problems)
	}
}

func TestValidateIsDeterministic(t *testing.T) {
	rules := []DependsRule{
		{Check: "a", OnCheck: "b", SameHost: true},
		{Check: "b", OnCheck: "a", SameHost: true},
	}
	first := strings.Join(ValidateDependencies(rules), "|")
	for i := 0; i < 5; i++ {
		if got := strings.Join(ValidateDependencies(rules), "|"); got != first {
			t.Fatalf("message order is not stable:\n%s\n%s", first, got)
		}
	}
}

// TestSameHostNeedsAnAddressInTheTarget pins a limitation that would otherwise
// look like the feature being broken. same_host compares HostOf(target), and a
// module configured with a friendly name reports that name — "db-primary"
// shares no host with "10.0.0.5:5432" as far as any string comparison goes.
//
// The rule then matches nothing, silently. Named targets want an explicit
// on_target glob instead, which is what the docs say.
func TestSameHostNeedsAnAddressInTheTarget(t *testing.T) {
	named := []Finding{
		{Check: "tcp", Target: "db-primary", Status: BAD, Message: "refused"},
		{Check: "postgres", Target: "10.0.0.5:5432", Status: ERROR, Message: "failed"},
	}
	for _, f := range ApplyDependencies(named, sameHostRule) {
		if f.SuppressedBy != "" {
			t.Errorf("a friendly name must not be treated as a host match: %+v", f)
		}
	}

	// The documented workaround does match.
	got := ApplyDependencies(named, []DependsRule{
		{Check: "postgres", OnCheck: "tcp", OnTarget: "db-primary"},
	})
	var suppressed bool
	for _, f := range got {
		if f.Check == "postgres" && f.SuppressedBy == "tcp db-primary" {
			suppressed = true
		}
	}
	if !suppressed {
		t.Errorf("an explicit on_target should pair a named parent: %+v", got)
	}
}
