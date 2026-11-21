package keys

import (
	"bytes"
	"testing"
)

// RFC 7748 section 6.1: Alice's private key and the public key it yields.
const (
	rfcPrivateHex = "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a"
	rfcPublicHex  = "8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a"
)

func fromHex(t *testing.T, s string) Key {
	t.Helper()
	var k Key
	for i := range Size {
		var b byte
		for _, c := range s[2*i : 2*i+2] {
			b <<= 4
			switch {
			case c >= '0' && c <= '9':
				b |= byte(c - '0')
			default:
				b |= byte(c-'a') + 10
			}
		}
		k[i] = b
	}
	return k
}

func TestPublicMatchesRFC7748(t *testing.T) {
	priv := fromHex(t, rfcPrivateHex)
	if got, want := priv.Public(), fromHex(t, rfcPublicHex); got != want {
		t.Fatalf("public key = %x, want %x", got, want)
	}
}

func TestGenerateClamps(t *testing.T) {
	k, err := Generate(bytes.NewReader(bytes.Repeat([]byte{0xff}, Size)))
	if err != nil {
		t.Fatal(err)
	}
	if k[0]&7 != 0 || k[31]&128 != 0 || k[31]&64 == 0 {
		t.Fatalf("key not clamped: first=%08b last=%08b", k[0], k[31])
	}
}

func TestGenerateShortRandomness(t *testing.T) {
	if _, err := Generate(bytes.NewReader([]byte{1, 2, 3})); err == nil {
		t.Fatal("expected an error for short randomness")
	}
}

func TestParseRoundTrip(t *testing.T) {
	k, err := Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(k.String())
	if err != nil || back != k {
		t.Fatalf("round trip: %v, %x != %x", err, back, k)
	}
}

func TestParseRejects(t *testing.T) {
	for _, s := range []string{"", "not base64!", "AAAA", Key{}.String() + "AAAA"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) accepted", s)
		}
	}
}
