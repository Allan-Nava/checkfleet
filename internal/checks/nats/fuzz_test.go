package nats

import (
	"encoding/json"
	"testing"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// FuzzParseJSZ feeds arbitrary /jsz?meta=1 payloads through the JSON decode and
// the meta-cluster analysis. Both the body (from the monitoring endpoint) and
// the values in it are untrusted, so decode+analyze must never panic; a
// malformed body just fails to unmarshal and is skipped.
func FuzzParseJSZ(f *testing.F) {
	seeds := []string{
		`{}`,
		`{"meta_cluster":{}}`,
		`{"meta_cluster":{"name":"c","leader":"n1","cluster_size":3,"replicas":[{"name":"n2","current":true,"lag":5},{"name":"n3","offline":true,"lag":99999}]}}`,
		`{"meta_cluster":{"leader":"","cluster_size":-1,"replicas":[{"name":"","lag":18446744073709551615}]}}`,
		`{"meta_cluster":{"replicas":null}}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	c := New(engine.NATSConfig{
		LagWarn:          100,
		LagCrit:          1000,
		ExpectMetaLeader: "n1",
		ExpectPeers:      []string{"n1", "n2", "n3"},
	})
	f.Fuzz(func(t *testing.T, data []byte) {
		var j jsz
		if err := json.Unmarshal(data, &j); err != nil {
			return // malformed JSON is a normal, non-panicking outcome
		}
		_ = c.analyzeMeta([]metaView{{responder: "n1", mc: j.MetaCluster}})
	})
}
