package haproxy

import (
	"strings"
	"testing"
)

// FuzzParseCSV feeds arbitrary bodies to the HAProxy stats CSV parser. The CSV
// is fetched from the stats endpoint (untrusted from the checker's view), so
// the parser must never panic — only return rows or an error.
func FuzzParseCSV(f *testing.F) {
	seeds := []string{
		"",
		"# pxname,svname,status\n",
		"# pxname,svname,status\nbe_app,srv1,UP\nbe_app,BACKEND,UP\n",
		"# pxname,svname,scur,slim\nbe,srv,10,100\n",
		"no header at all\njust,some,rows\n",
		"#\n,\n,,,\n",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data string) {
		_, _ = parseCSV(strings.NewReader(data))
	})
}
