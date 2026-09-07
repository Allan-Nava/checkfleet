package discovery

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/checkfleet/internal/engine"
)

// srvRecord is one answer the in-test nameserver will hand back.
type srvRecord struct {
	priority, weight, port uint16
	target                 string
}

// fakeDNS serves SRV answers over UDP on 127.0.0.1 and returns its address.
//
// A real nameserver in-test, not a mock: it is the only way to prove that the
// resolver override actually sends the query somewhere else, which is the whole
// reason the option exists (Consul's DNS on :8600 is invisible to the system
// resolver).
func fakeDNS(t *testing.T, answers []srvRecord) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return // the listener was closed at cleanup
			}
			resp := srvResponse(buf[:n], answers)
			if resp != nil {
				_, _ = conn.WriteTo(resp, addr)
			}
		}
	}()
	return conn.LocalAddr().String()
}

// srvResponse echoes the query and appends the SRV answers. Only what a reply
// needs: same ID, response+recursion-available flags, the question copied back,
// then one SRV record per answer.
func srvResponse(query []byte, answers []srvRecord) []byte {
	if len(query) < 12 {
		return nil
	}
	// Walk the question's name to find where it ends, so it can be copied.
	i := 12
	for i < len(query) && query[i] != 0 {
		i += int(query[i]) + 1
	}
	i += 5 // the zero label plus QTYPE and QCLASS
	if i > len(query) {
		return nil
	}

	out := make([]byte, 0, 512)
	out = append(out, query[0], query[1]) // ID
	out = append(out, 0x81, 0x80)         // response, recursion desired+available
	out = binary.BigEndian.AppendUint16(out, 1)
	out = binary.BigEndian.AppendUint16(out, uint16(len(answers)))
	out = binary.BigEndian.AppendUint16(out, 0)
	out = binary.BigEndian.AppendUint16(out, 0)
	out = append(out, query[12:i]...) // the question, verbatim

	for _, a := range answers {
		out = append(out, 0xC0, 0x0C) // pointer to the question's name
		out = binary.BigEndian.AppendUint16(out, 33)
		out = binary.BigEndian.AppendUint16(out, 1)
		out = binary.BigEndian.AppendUint32(out, 60)
		rdata := make([]byte, 0, 32)
		rdata = binary.BigEndian.AppendUint16(rdata, a.priority)
		rdata = binary.BigEndian.AppendUint16(rdata, a.weight)
		rdata = binary.BigEndian.AppendUint16(rdata, a.port)
		rdata = append(rdata, encodeName(a.target)...)
		out = binary.BigEndian.AppendUint16(out, uint16(len(rdata)))
		out = append(out, rdata...)
	}
	return out
}

func encodeName(name string) []byte {
	var out []byte
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	return append(out, 0)
}

func TestResolveReadsSRVRecords(t *testing.T) {
	addr := fakeDNS(t, []srvRecord{
		{priority: 10, weight: 1, port: 4222, target: "nats1.service.consul."},
		{priority: 10, weight: 1, port: 4223, target: "nats2.service.consul."},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hosts, err := Resolve(ctx, engine.Discovery{DNSSRV: []engine.SRVLookup{
		{Name: "_nats._tcp.service.consul", Resolver: addr},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"nats1.service.consul:4222", "nats2.service.consul:4223"}
	got := Addresses(hosts)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("want %v, got %v", want, got)
	}
	if hosts[0].Source != "dns-srv" {
		t.Errorf("want the dns-srv source, got %q", hosts[0].Source)
	}
}

// certs discovered from an SRV pointing at 8443 still has to be probed on the
// port the module configures, so the record's port must be droppable.
func TestSRVCanDropThePort(t *testing.T) {
	addr := fakeDNS(t, []srvRecord{{port: 8443, target: "web1.example.com."}})

	no := false
	hosts, err := Resolve(context.Background(), engine.Discovery{DNSSRV: []engine.SRVLookup{
		{Name: "_https._tcp.example.com", Resolver: addr, KeepPort: &no},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := Addresses(hosts); len(got) != 1 || got[0] != "web1.example.com" {
		t.Fatalf("want the bare host, got %v", got)
	}
}

func TestSRVRequiresAName(t *testing.T) {
	_, err := Resolve(context.Background(), engine.Discovery{DNSSRV: []engine.SRVLookup{{}}})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("want a clear error for an SRV lookup with no name, got %v", err)
	}
}

// A nameserver that answers nothing must be an error, not an empty fleet.
func TestSRVFailureIsReported(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Resolve(ctx, engine.Discovery{DNSSRV: []engine.SRVLookup{
		{Name: "_nats._tcp.invalid.", Resolver: "127.0.0.1:1"},
	}})
	if err == nil || !strings.Contains(err.Error(), "dns srv") {
		t.Fatalf("an unreachable nameserver must surface, got %v", err)
	}
}
