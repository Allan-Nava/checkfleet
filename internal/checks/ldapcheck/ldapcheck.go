// Package ldapcheck implements an LDAP health check: connect, bind (anonymous
// or with credentials), and an optional sanity search. The LDAP I/O is behind
// the session interface so the finding logic is unit-tested with a fake; the
// go-ldap-backed session lives in ldap.go.
package ldapcheck

import (
	"context"
	"fmt"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// session is the LDAP I/O this check needs.
type session interface {
	Bind(dn, password string) error
	Search(baseDN, filter string) (entries int, err error)
	Close()
}

type Check struct {
	cfg engine.LDAPConfig
	// dial is injectable for tests; defaults to the go-ldap session.
	dial func(ctx context.Context, t engine.LDAPTarget) (session, error)
}

func New(cfg engine.LDAPConfig) *Check {
	return &Check{cfg: cfg, dial: dialLDAP}
}

func (c *Check) Name() string { return "ldap" }

func (c *Check) Run(ctx context.Context) []engine.Finding {
	findings := make([]engine.Finding, len(c.cfg.Targets))
	sem := make(chan struct{}, 8)
	done := make(chan int)
	for i, t := range c.cfg.Targets {
		go func(i int, t engine.LDAPTarget) {
			sem <- struct{}{}
			findings[i] = c.probe(ctx, t)
			<-sem
			done <- i
		}(i, t)
	}
	for range c.cfg.Targets {
		<-done
	}
	return findings
}

func (c *Check) probe(ctx context.Context, t engine.LDAPTarget) engine.Finding {
	label := t.Name
	if label == "" {
		label = t.URL
	}
	f := engine.Finding{Check: c.Name(), Target: label}

	sess, err := c.dial(ctx, t)
	if err != nil {
		f.Status, f.Message = engine.ERROR, fmt.Sprintf("connessione fallita: %v", err)
		return f
	}
	defer sess.Close()

	if err := sess.Bind(t.BindDN, password(t)); err != nil {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("bind fallito: %v", err)
		return f
	}

	if t.BaseDN == "" {
		f.Status, f.Message = engine.OK, bindDesc(t)+", bind ok"
		return f
	}
	n, err := sess.Search(t.BaseDN, t.Filter)
	if err != nil {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("search fallita: %v", err)
		return f
	}
	if n < t.MinEntries {
		f.Status, f.Message = engine.BAD, fmt.Sprintf("search: %d risultati (attesi ≥ %d)", n, t.MinEntries)
		return f
	}
	f.Status, f.Message = engine.OK, fmt.Sprintf("%s, %d risultati", bindDesc(t), n)
	return f
}

func bindDesc(t engine.LDAPTarget) string {
	if t.BindDN == "" {
		return "bind anonimo"
	}
	return "bind " + t.BindDN
}
