package dns

import (
	"encoding/binary"
	"testing"
)

// buildResponse crafts a minimal DNS response for one question and the given
// answer records, so the parser can be tested without a network.
type answer struct {
	typ  uint16
	ttl  uint32
	data []byte
}

func buildResponse(t *testing.T, qname string, qtype uint16, rcode int, answers []answer) []byte {
	t.Helper()
	var b []byte
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint16(hdr[0:], 0x1234)
	binary.BigEndian.PutUint16(hdr[2:], uint16(0x8000|rcode)) // QR=1 + rcode
	binary.BigEndian.PutUint16(hdr[4:], 1)                    // QDCOUNT
	binary.BigEndian.PutUint16(hdr[6:], uint16(len(answers))) // ANCOUNT
	b = append(b, hdr...)

	qn, err := encodeName(qname)
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, qn...)
	var tc [4]byte
	binary.BigEndian.PutUint16(tc[0:], qtype)
	binary.BigEndian.PutUint16(tc[2:], classIN)
	b = append(b, tc[:]...)

	for _, a := range answers {
		name, _ := encodeName(qname)
		b = append(b, name...)
		rr := make([]byte, 10)
		binary.BigEndian.PutUint16(rr[0:], a.typ)
		binary.BigEndian.PutUint16(rr[2:], classIN)
		binary.BigEndian.PutUint32(rr[4:], a.ttl)
		binary.BigEndian.PutUint16(rr[8:], uint16(len(a.data)))
		b = append(b, rr...)
		b = append(b, a.data...)
	}
	return b
}

func TestBuildQueryRoundTripsName(t *testing.T) {
	q, err := buildQuery("www.example.com", typeA, 0x1234)
	if err != nil {
		t.Fatal(err)
	}
	name, _, err := parseName(q, 12)
	if err != nil {
		t.Fatal(err)
	}
	if name != "www.example.com" {
		t.Errorf("nome query round-trip: %q", name)
	}
}

func TestParseARecords(t *testing.T) {
	resp := buildResponse(t, "example.com", typeA, 0, []answer{
		{typ: typeA, ttl: 300, data: []byte{93, 184, 216, 34}},
		{typ: typeA, ttl: 300, data: []byte{1, 2, 3, 4}},
	})
	rcode, recs, err := parseMessage(resp, typeA)
	if err != nil || rcode != 0 {
		t.Fatalf("parse: rcode=%d err=%v", rcode, err)
	}
	if len(recs) != 2 || recs[0].Value != "93.184.216.34" || recs[0].TTL != 300 {
		t.Errorf("record A inattesi: %+v", recs)
	}
}

func TestParseSOASerial(t *testing.T) {
	// SOA rdata: mname, rname, serial, refresh, retry, expire, minimum.
	mname, _ := encodeName("ns1.example.com")
	rname, _ := encodeName("hostmaster.example.com")
	rdata := append(append([]byte{}, mname...), rname...)
	nums := make([]byte, 20)
	binary.BigEndian.PutUint32(nums[0:], 2026072401) // serial
	rdata = append(rdata, nums...)

	resp := buildResponse(t, "example.com", typeSOA, 0, []answer{{typ: typeSOA, ttl: 3600, data: rdata}})
	_, recs, err := parseMessage(resp, typeSOA)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Value != "2026072401" {
		t.Errorf("serial SOA inatteso: %+v", recs)
	}
}

func TestParseRcodeNXDomain(t *testing.T) {
	resp := buildResponse(t, "nope.example.com", typeA, 3, nil)
	rcode, recs, err := parseMessage(resp, typeA)
	if err != nil {
		t.Fatal(err)
	}
	if rcode != 3 || len(recs) != 0 {
		t.Errorf("atteso NXDOMAIN senza record, avuto rcode=%d recs=%+v", rcode, recs)
	}
}
