package issue

import (
	"bytes"
	"net/netip"
	"strings"
	"testing"

	"github.com/CorneiZeR/awg-gate/internal/conf"
	"github.com/CorneiZeR/awg-gate/internal/keys"
	"github.com/CorneiZeR/awg-gate/internal/obfs"
	"github.com/CorneiZeR/awg-gate/internal/state"
)

func info(t *testing.T, profile string) state.Info {
	t.Helper()
	p, err := obfs.Generate(profile, nil)
	if err != nil {
		t.Fatal(err)
	}
	hub, _ := keys.Generate(nil)
	return state.Info{
		Version: state.InfoVersion, Endpoint: "vpn.example.com:443", PublicKey: hub.Public().String(),
		MTU: 1280, Keepalive: 25, Network: "10.247.0.0/16", ServersSubnet: "10.247.0.0/24",
		PeopleSubnet: "10.247.1.0/24", HubAddress: "10.247.0.1", Params: p,
	}
}

func TestPersonRoutesOnlyServers(t *testing.T) {
	i := info(t, "v1")
	priv, _ := keys.Generate(nil)
	f := Person(i, priv, netip.MustParseAddr("10.247.1.10"))
	if got := f.Peers[0].List("AllowedIPs"); len(got) != 1 || got[0] != "10.247.0.0/24" {
		t.Errorf("AllowedIPs = %v", got)
	}
	if _, ok := f.Interface.Get("DNS"); ok {
		t.Error("a DNS line was issued")
	}
	if v, _ := f.Interface.Get("Address"); v != "10.247.1.10/32" {
		t.Errorf("Address = %s", v)
	}
	if v, _ := f.Interface.Get("H1"); v != i.Params.H1 {
		t.Errorf("H1 = %s, want the hub's %s", v, i.Params.H1)
	}
	// What is issued parses back as the same file.
	again, err := conf.Parse(strings.NewReader(Header("alice/laptop") + f.Render()))
	if err != nil || again.Render() != f.Render() {
		t.Fatalf("issued file does not parse back: %v", err)
	}
}

func TestServerRoutesOnlyPeople(t *testing.T) {
	priv, _ := keys.Generate(nil)
	f := Server(info(t, "v3"), priv, netip.MustParseAddr("10.247.0.10"))
	if got := f.Peers[0].List("AllowedIPs"); len(got) != 1 || got[0] != "10.247.1.0/24" {
		t.Errorf("AllowedIPs = %v", got)
	}
	if _, ok := f.Interface.Get("HeaderProtectionKey"); !ok {
		t.Error("v3 parameters missing")
	}
}

func TestQR(t *testing.T) {
	priv, _ := keys.Generate(nil)
	text := Person(info(t, "v3"), priv, netip.MustParseAddr("10.247.1.10")).Render()
	png, err := QRPNG(text)
	if err != nil || !bytes.HasPrefix(png, []byte("\x89PNG")) {
		t.Fatalf("PNG: %v", err)
	}
	term, err := QRTerminal(text)
	if err != nil || strings.Count(term, "\n") < 20 {
		t.Fatalf("terminal QR: %v", err)
	}
}
