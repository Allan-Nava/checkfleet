package ldapcheck

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// fakeSession is an in-memory LDAP session.
type fakeSession struct {
	bindErr   error
	entries   int
	searchErr error
}

func (f *fakeSession) Bind(dn, pw string) error                { return f.bindErr }
func (f *fakeSession) Search(base, filter string) (int, error) { return f.entries, f.searchErr }
func (f *fakeSession) Close()                                  {}

func checkWith(t engine.LDAPTarget, sess *fakeSession, dialErr error) *Check {
	c := New(engine.LDAPConfig{Targets: []engine.LDAPTarget{t}})
	c.dial = func(context.Context, engine.LDAPTarget) (session, error) {
		if dialErr != nil {
			return nil, dialErr
		}
		return sess, nil
	}
	return c
}

func run(t *testing.T, c *Check) engine.Finding {
	t.Helper()
	f := c.Run(context.Background())
	if len(f) != 1 {
		t.Fatalf("want 1 finding, got %d", len(f))
	}
	return f[0]
}

func TestBindOKNoSearch(t *testing.T) {
	tg := engine.LDAPTarget{URL: "ldap://dir:389", BindDN: "cn=admin", PasswordEnv: "X"}
	if got := run(t, checkWith(tg, &fakeSession{}, nil)); got.Status != engine.OK {
		t.Errorf("bind ok without search: want OK, got %s (%s)", got.Status, got.Message)
	}
}

func TestBindFailIsBad(t *testing.T) {
	tg := engine.LDAPTarget{URL: "ldap://dir", BindDN: "cn=admin", PasswordEnv: "X"}
	if got := run(t, checkWith(tg, &fakeSession{bindErr: errors.New("invalid credentials")}, nil)); got.Status != engine.BAD {
		t.Errorf("bind failed: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestSearchTooFewIsBad(t *testing.T) {
	tg := engine.LDAPTarget{URL: "ldap://dir", BaseDN: "ou=people,dc=x", Filter: "(uid=*)", MinEntries: 5}
	if got := run(t, checkWith(tg, &fakeSession{entries: 2}, nil)); got.Status != engine.BAD || !strings.Contains(got.Message, "want") {
		t.Errorf("few results: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestSearchOK(t *testing.T) {
	tg := engine.LDAPTarget{URL: "ldap://dir", BaseDN: "ou=people,dc=x", Filter: "(uid=*)", MinEntries: 1}
	if got := run(t, checkWith(tg, &fakeSession{entries: 42}, nil)); got.Status != engine.OK {
		t.Errorf("search ok: want OK, got %s (%s)", got.Status, got.Message)
	}
}

func TestSearchErrorIsBad(t *testing.T) {
	tg := engine.LDAPTarget{URL: "ldap://dir", BaseDN: "dc=x", Filter: "(x)", MinEntries: 1}
	if got := run(t, checkWith(tg, &fakeSession{searchErr: errors.New("no such object")}, nil)); got.Status != engine.BAD {
		t.Errorf("search error: want BAD, got %s (%s)", got.Status, got.Message)
	}
}

func TestDialErrorIsError(t *testing.T) {
	tg := engine.LDAPTarget{URL: "ldap://down"}
	if got := run(t, checkWith(tg, nil, errors.New("connection refused"))); got.Status != engine.ERROR {
		t.Errorf("connection failed: want ERROR, got %s (%s)", got.Status, got.Message)
	}
}
