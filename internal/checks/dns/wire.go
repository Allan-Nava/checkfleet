package dns

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Minimal DNS wire codec (RFC 1035): enough to build a query and parse the
// answers we care about (A, AAAA, CNAME, NS, TXT, SOA), including their TTL and
// name compression. No third-party dependency.

const (
	typeA     uint16 = 1
	typeNS    uint16 = 2
	typeCNAME uint16 = 5
	typeSOA   uint16 = 6
	typeTXT   uint16 = 16
	typeAAAA  uint16 = 28
	classIN   uint16 = 1
)

var typeNames = map[uint16]string{
	typeA: "A", typeNS: "NS", typeCNAME: "CNAME", typeSOA: "SOA", typeTXT: "TXT", typeAAAA: "AAAA",
}
var typeNumbers = map[string]uint16{
	"A": typeA, "NS": typeNS, "CNAME": typeCNAME, "SOA": typeSOA, "TXT": typeTXT, "AAAA": typeAAAA,
}

func typeNumber(s string) (uint16, bool) {
	n, ok := typeNumbers[strings.ToUpper(strings.TrimSpace(s))]
	return n, ok
}

func typeName(n uint16) string {
	if s, ok := typeNames[n]; ok {
		return s
	}
	return fmt.Sprintf("TYPE%d", n)
}

// record is one parsed resource record.
type record struct {
	Type  string
	Value string
	TTL   uint32
}

// buildQuery encodes a standard recursive query for name/qtype.
func buildQuery(name string, qtype, id uint16) ([]byte, error) {
	var b []byte
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint16(hdr[0:], id)
	binary.BigEndian.PutUint16(hdr[2:], 0x0100) // RD=1
	binary.BigEndian.PutUint16(hdr[4:], 1)      // QDCOUNT
	b = append(b, hdr...)

	qname, err := encodeName(name)
	if err != nil {
		return nil, err
	}
	b = append(b, qname...)
	var tc [4]byte
	binary.BigEndian.PutUint16(tc[0:], qtype)
	binary.BigEndian.PutUint16(tc[2:], classIN)
	return append(b, tc[:]...), nil
}

func encodeName(name string) ([]byte, error) {
	name = strings.TrimSuffix(name, ".")
	var b []byte
	if name != "" {
		for _, label := range strings.Split(name, ".") {
			if len(label) == 0 || len(label) > 63 {
				return nil, fmt.Errorf("invalid label in %q", name)
			}
			b = append(b, byte(len(label)))
			b = append(b, label...)
		}
	}
	return append(b, 0), nil
}

// parseMessage returns the DNS rcode and the answer records.
func parseMessage(msg []byte, want uint16) (rcode int, answers []record, err error) {
	if len(msg) < 12 {
		return 0, nil, fmt.Errorf("response too short")
	}
	rcode = int(msg[3] & 0x0F)
	qd := int(binary.BigEndian.Uint16(msg[4:]))
	an := int(binary.BigEndian.Uint16(msg[6:]))

	off := 12
	for i := 0; i < qd; i++ {
		if _, off, err = parseName(msg, off); err != nil {
			return rcode, nil, err
		}
		off += 4 // qtype + qclass
	}
	for i := 0; i < an; i++ {
		var name string
		if name, off, err = parseName(msg, off); err != nil {
			return rcode, nil, err
		}
		_ = name
		if off+10 > len(msg) {
			return rcode, nil, fmt.Errorf("RR troncato")
		}
		rrType := binary.BigEndian.Uint16(msg[off:])
		ttl := binary.BigEndian.Uint32(msg[off+4:])
		rdlen := int(binary.BigEndian.Uint16(msg[off+8:]))
		off += 10
		if off+rdlen > len(msg) {
			return rcode, nil, fmt.Errorf("RDATA troncato")
		}
		val, ok := decodeRData(msg, off, rdlen, rrType)
		if ok {
			answers = append(answers, record{Type: typeName(rrType), Value: val, TTL: ttl})
		}
		off += rdlen
	}
	_ = want
	return rcode, answers, nil
}

func decodeRData(msg []byte, off, rdlen int, rrType uint16) (string, bool) {
	switch rrType {
	case typeA:
		if rdlen != 4 {
			return "", false
		}
		return net.IP(msg[off : off+4]).String(), true
	case typeAAAA:
		if rdlen != 16 {
			return "", false
		}
		return net.IP(msg[off : off+16]).String(), true
	case typeCNAME, typeNS:
		name, _, err := parseName(msg, off)
		if err != nil {
			return "", false
		}
		return name, true
	case typeTXT:
		var parts []string
		p := off
		for p < off+rdlen {
			l := int(msg[p])
			p++
			if p+l > off+rdlen {
				break
			}
			parts = append(parts, string(msg[p:p+l]))
			p += l
		}
		return strings.Join(parts, ""), true
	case typeSOA:
		_, p, err := parseName(msg, off) // mname
		if err != nil {
			return "", false
		}
		if _, p, err = parseName(msg, p); err != nil { // rname
			return "", false
		}
		if p+4 > len(msg) {
			return "", false
		}
		serial := binary.BigEndian.Uint32(msg[p:])
		return strconv.FormatUint(uint64(serial), 10), true
	default:
		return "", false
	}
}

// parseName decodes a (possibly compressed) domain name at off, returning the
// name and the offset just past it in the top-level record.
func parseName(msg []byte, off int) (string, int, error) {
	var labels []string
	next := -1
	jumps := 0
	for {
		if off < 0 || off >= len(msg) {
			return "", 0, fmt.Errorf("offset nome fuori range")
		}
		b := msg[off]
		switch {
		case b == 0:
			off++
			if next < 0 {
				next = off
			}
			return strings.Join(labels, "."), next, nil
		case b&0xC0 == 0xC0:
			if off+1 >= len(msg) {
				return "", 0, fmt.Errorf("puntatore troncato")
			}
			ptr := int(b&0x3F)<<8 | int(msg[off+1])
			if next < 0 {
				next = off + 2
			}
			off = ptr
			if jumps++; jumps > 50 {
				return "", 0, fmt.Errorf("troppi puntatori (loop?)")
			}
		default:
			l := int(b)
			off++
			if off+l > len(msg) {
				return "", 0, fmt.Errorf("label troncata")
			}
			labels = append(labels, string(msg[off:off+l]))
			off += l
		}
	}
}
