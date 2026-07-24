package ldapcheck

import (
	"context"
	"crypto/tls"
	"net/url"
	"os"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
	ldap "github.com/go-ldap/ldap/v3"
)

// dialLDAP is the default session: a real go-ldap connection.
func dialLDAP(ctx context.Context, t engine.LDAPTarget) (session, error) {
	tlsCfg := &tls.Config{InsecureSkipVerify: t.InsecureSkipVerify, ServerName: hostFromURL(t.URL)}
	l, err := ldap.DialURL(t.URL, ldap.DialWithTLSConfig(tlsCfg))
	if err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		l.SetTimeout(time.Until(dl))
	}
	if t.StartTLS {
		if err := l.StartTLS(tlsCfg); err != nil {
			l.Close()
			return nil, err
		}
	}
	return &ldapSession{l: l}, nil
}

type ldapSession struct{ l *ldap.Conn }

func (s *ldapSession) Bind(dn, password string) error {
	if dn == "" {
		return nil // anonymous: the connection is already unauthenticated
	}
	return s.l.Bind(dn, password)
}

func (s *ldapSession) Search(baseDN, filter string) (int, error) {
	req := ldap.NewSearchRequest(baseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false, filter, []string{"dn"}, nil)
	res, err := s.l.Search(req)
	if err != nil {
		return 0, err
	}
	return len(res.Entries), nil
}

func (s *ldapSession) Close() { s.l.Close() }

func password(t engine.LDAPTarget) string {
	if t.PasswordEnv == "" {
		return ""
	}
	return os.Getenv(t.PasswordEnv)
}

func hostFromURL(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Hostname()
	}
	return ""
}
