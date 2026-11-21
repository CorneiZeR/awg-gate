package nft

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/CorneiZeR/awg-gate/internal/gate"
)

func TestHub(t *testing.T) {
	rules := []gate.Rule{
		{From: netip.MustParseAddr("10.247.1.10"), To: netip.MustParseAddr("10.247.0.10"), FromName: "alice/laptop", ToName: "shop-dev"},
		{From: netip.MustParseAddr("10.247.1.10"), To: netip.MustParseAddr("10.247.0.11"), FromName: "alice/laptop", ToName: "blog-dev", Ports: []int{80, 443}},
	}
	got := Hub("awg0", rules)
	for _, want := range []string{
		`iifname "awg0" oifname "awg0" ip saddr 10.247.1.10 ip daddr 10.247.0.10 accept comment "alice/laptop -> shop-dev"`,
		`ip daddr 10.247.0.11 meta l4proto { tcp, udp } th dport { 80, 443 } accept`,
		`ip daddr 10.247.0.11 icmp type echo-request accept`,
		`iifname "awg0" counter drop comment "no rule lets this through"`,
		`policy accept;`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// The catch-all drop must come after every accept.
	if strings.Index(got, "no rule lets this through") < strings.LastIndex(got, "accept comment") {
		t.Error("the forward drop precedes an accept")
	}
	if strings.Contains(got, "policy drop") {
		t.Error("a drop policy would take the traffic of a host-network hub with it")
	}
}

func TestSpoke(t *testing.T) {
	if got := Spoke("awg0", SpokeOptions{}); got != "" {
		t.Errorf("nothing asked, got rules:\n%s", got)
	}
	got := Spoke("awg0", SpokeOptions{AllowPorts: []int{443}})
	for _, want := range []string{"th dport { 443 } accept", "ct original proto-dst { 443 } accept", "chain forward"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "lockdown") {
		t.Error("lockdown rules without lockdown")
	}
	got = Spoke("awg0", SpokeOptions{Lockdown: true})
	if !strings.Contains(got, `iifname != { "lo", "awg0" } counter drop`) || strings.Contains(got, "chain forward") {
		t.Errorf("lockdown only:\n%s", got)
	}
}

func TestScriptReplacesAtomically(t *testing.T) {
	s := Script("")
	if s != "table inet awg_gate\ndelete table inet awg_gate\n" {
		t.Errorf("Script(\"\") = %q", s)
	}
}

func TestQuoteStripsInjection(t *testing.T) {
	if got := quote(`a" accept; drop \`); got != `"a accept; drop "` {
		t.Errorf("quote = %s", got)
	}
}
