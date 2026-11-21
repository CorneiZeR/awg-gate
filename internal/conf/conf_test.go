package conf

import (
	"reflect"
	"strings"
	"testing"
)

const sample = `# issued by awg-gate
[Interface]
PrivateKey = cHJpdmF0ZQ==
Address = 10.247.1.10/32
MTU = 1280
Jc = 5

[Peer]
PublicKey = cHVibGlj
Endpoint = vpn.example.com:443   # the hub
AllowedIPs = 10.247.0.0/24, 10.247.2.0/24
allowedips = 10.247.3.0/24
PersistentKeepalive = 25
`

func TestParse(t *testing.T) {
	f, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := f.Interface.Get("mtu"); v != "1280" {
		t.Errorf("MTU = %q", v)
	}
	if len(f.Peers) != 1 {
		t.Fatalf("peers = %d", len(f.Peers))
	}
	if v, _ := f.Peers[0].Get("Endpoint"); v != "vpn.example.com:443" {
		t.Errorf("Endpoint = %q, the trailing comment must be dropped", v)
	}
	want := []string{"10.247.0.0/24", "10.247.2.0/24", "10.247.3.0/24"}
	if got := f.Peers[0].List("AllowedIPs"); !reflect.DeepEqual(got, want) {
		t.Errorf("AllowedIPs = %v, want %v", got, want)
	}
}

func TestForSetconfStripsQuickKeys(t *testing.T) {
	f, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	out := f.ForSetconf().Render()
	for _, gone := range []string{"Address", "MTU"} {
		if strings.Contains(out, gone) {
			t.Errorf("%s survived:\n%s", gone, out)
		}
	}
	for _, kept := range []string{"PrivateKey", "Jc = 5", "PersistentKeepalive"} {
		if !strings.Contains(out, kept) {
			t.Errorf("%s lost:\n%s", kept, out)
		}
	}
}

func TestRenderRoundTrip(t *testing.T) {
	f, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(strings.NewReader(f.Render()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f, again) {
		t.Fatalf("round trip changed the file:\n%#v\n%#v", f, again)
	}
}

func TestParseErrors(t *testing.T) {
	for name, in := range map[string]string{
		"no interface":      "[Peer]\nPublicKey = x\n",
		"two interfaces":    "[Interface]\n[Interface]\n",
		"unknown section":   "[Interface]\n[Device]\n",
		"key before header": "PrivateKey = x\n[Interface]\n",
		"no equals":         "[Interface]\nPrivateKey\n",
	} {
		if _, err := Parse(strings.NewReader(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
