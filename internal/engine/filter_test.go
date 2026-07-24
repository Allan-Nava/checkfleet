package engine

import "testing"

func sample() []Finding {
	return []Finding{
		{Check: "certs", Target: "a.example:443", Status: OK},
		{Check: "http", Target: "https://a.example/", Status: WARN},
		{Check: "http", Target: "https://b.example/health", Status: BAD},
		{Check: "nats", Target: "gw-sg", Status: ERROR},
	}
}

func checks(f []Finding) []string {
	var out []string
	for _, x := range f {
		out = append(out, x.Check)
	}
	return out
}

func TestFilterOnly(t *testing.T) {
	got := Filter(sample(), FilterOptions{Only: map[string]bool{"http": true}})
	if len(got) != 2 || got[0].Check != "http" || got[1].Check != "http" {
		t.Errorf("--only http: attesi 2 http, avuto %v", checks(got))
	}
}

func TestFilterMinSeverity(t *testing.T) {
	got := Filter(sample(), FilterOptions{MinSeverity: BAD})
	if len(got) != 2 { // BAD + ERROR
		t.Errorf("min-severity bad: attesi 2 (BAD,ERROR), avuto %v", got)
	}
	for _, f := range got {
		if severity[f.Status] < severity[BAD] {
			t.Errorf("finding sotto soglia non filtrato: %+v", f)
		}
	}
}

func TestFilterTargetGlob(t *testing.T) {
	got := Filter(sample(), FilterOptions{TargetGlob: "https://b.example/*"})
	if len(got) != 1 || got[0].Target != "https://b.example/health" {
		t.Errorf("glob target: atteso 1 match, avuto %v", got)
	}
}

func TestFilterCombined(t *testing.T) {
	got := Filter(sample(), FilterOptions{Only: map[string]bool{"http": true}, MinSeverity: BAD})
	if len(got) != 1 || got[0].Status != BAD {
		t.Errorf("only http + min bad: atteso 1 BAD http, avuto %v", got)
	}
}

func TestFilterEmptyKeepsAll(t *testing.T) {
	if got := Filter(sample(), FilterOptions{}); len(got) != 4 {
		t.Errorf("nessun filtro: attesi 4, avuto %d", len(got))
	}
}

func TestParseStatus(t *testing.T) {
	if s, ok := ParseStatus("warn"); !ok || s != WARN {
		t.Errorf("parse warn: %v %v", s, ok)
	}
	if _, ok := ParseStatus("boom"); ok {
		t.Error("status invalido deve fallire")
	}
	if s, ok := ParseStatus(""); !ok || s != "" {
		t.Errorf("vuoto = nessun filtro: %v %v", s, ok)
	}
}
