package dns

import "testing"

// FuzzParseMessage feeds arbitrary bytes to the hand-rolled DNS wire parser.
// The message is raw UDP/TCP data from a resolver — fully untrusted — and the
// parser walks it by hand with offsets and compression pointers, so it must
// never panic (bad input may only return an error).
func FuzzParseMessage(f *testing.F) {
	// Header only (QD=0, AN=0).
	f.Add(make([]byte, 12))
	// Too short to hold a header.
	f.Add([]byte{0x00, 0x01, 0x02})
	// A well-formed response: 1 question ("a") + 1 A answer via a compression
	// pointer back to the question name.
	f.Add([]byte{
		0x00, 0x00, // ID
		0x81, 0x80, // flags (response, no error)
		0x00, 0x01, // QDCOUNT
		0x00, 0x01, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
		0x01, 'a', 0x00, // QNAME "a"
		0x00, 0x01, // QTYPE A
		0x00, 0x01, // QCLASS IN
		0xC0, 0x0C, // NAME pointer to offset 12
		0x00, 0x01, // TYPE A
		0x00, 0x01, // CLASS IN
		0x00, 0x00, 0x00, 0x3C, // TTL 60
		0x00, 0x04, // RDLENGTH 4
		0x5D, 0xB8, 0xD8, 0x22, // 93.184.216.34
	})
	// A self-referential compression pointer (loop guard must catch it).
	f.Add([]byte{
		0x00, 0x00, 0x81, 0x80, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
		0xC0, 0x0C, // NAME pointer to itself
	})
	f.Fuzz(func(t *testing.T, msg []byte) {
		// `want` is not consulted by parseMessage (the RR type is read from the
		// record itself); any value exercises the same code path.
		_, _, _ = parseMessage(msg, typeA)
	})
}
