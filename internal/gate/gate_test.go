package gate

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/CorneiZeR/awg-gate/internal/keys"
)

func pub(t *testing.T) string {
	t.Helper()
	k, err := keys.Generate(nil)
	if err != nil {
		t.Fatal(err)
	}
	return k.Public().String()
}

func sampleYAML(t *testing.T) string {
	t.Helper()
	return `# the team's access network
endpoint: vpn.example.com:443
servers:
  shop-dev:
    address: 10.247.0.10
    public_key: ` + pub(t) + `
  blog-dev:
    address: 10.247.0.11
    public_key: ` + pub(t) + `
    ports: [443, 80]
people:
  alice:
    reach: [shop-dev]
    devices:
      laptop: {address: 10.247.1.10, public_key: ` + pub(t) + `}
      phone: {address: 10.247.1.11, public_key: ` + pub(t) + `}
  bob:
    reach: ["*"]
    expires: 2026-12-31
    devices:
      laptop: {address: 10.247.1.20, public_key: ` + pub(t) + `}
`
}

func TestParseDefaults(t *testing.T) {
	c, err := Parse([]byte(sampleYAML(t)))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenPort != 443 || c.MTU != 1280 || c.Profile != "v1" || c.Interface != "awg0" || c.Keepalive != 25 {
		t.Errorf("defaults not applied: %+v", c)
	}
	if c.HubAddr() != netip.MustParseAddr("10.247.0.1") {
		t.Errorf("hub address = %s", c.HubAddr())
	}
}

func TestPlan(t *testing.T) {
	c, err := Parse([]byte(sampleYAML(t)))
	if err != nil {
		t.Fatal(err)
	}
	p := c.Plan(time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC))
	if len(p.Peers) != 5 || len(p.Expired) != 0 {
		t.Fatalf("peers=%d expired=%v", len(p.Peers), p.Expired)
	}
	var got []string
	for _, r := range p.Rules {
		got = append(got, r.FromName+">"+r.ToName)
		if r.ToName == "blog-dev" && (len(r.Ports) != 2 || r.Ports[0] != 80) {
			t.Errorf("blog-dev ports = %v, want sorted [80 443]", r.Ports)
		}
	}
	want := "alice/laptop>shop-dev alice/phone>shop-dev bob/laptop>blog-dev bob/laptop>shop-dev"
	if strings.Join(got, " ") != want {
		t.Errorf("rules = %v\nwant    %s", got, want)
	}

	// The day after bob's last day his laptop is gone, with its rules.
	p = c.Plan(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(p.Peers) != 4 || len(p.Expired) != 1 || p.Expired[0] != "bob/laptop" {
		t.Fatalf("after expiry: peers=%d expired=%v", len(p.Peers), p.Expired)
	}
	for _, r := range p.Rules {
		if strings.HasPrefix(r.FromName, "bob/") {
			t.Errorf("expired device still has a rule: %+v", r)
		}
	}
	if next := c.NextExpiry(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)); !next.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("NextExpiry = %s", next)
	}
}

func TestValidateRejects(t *testing.T) {
	k1, k2 := pub(t), pub(t)
	cases := map[string]struct{ yaml, want string }{
		"unknown key":         {"endpoint: h:1\nexpire: 2027-01-01\n", "field expire not found"},
		"bad endpoint":        {"endpoint: nohost\n", "endpoint"},
		"subnet outside":      {"endpoint: h:1\npeople_subnet: 10.248.1.0/24\n", "outside network"},
		"subnets overlap":     {"endpoint: h:1\npeople_subnet: 10.247.0.0/25\n", "overlap"},
		"host bits":           {"endpoint: h:1\nnetwork: 10.247.0.1/16\n", "host bits"},
		"mtu":                 {"endpoint: h:1\nmtu: 1500\n", "mtu"},
		"profile":             {"endpoint: h:1\nprofile: v2\n", "profile"},
		"server outside":      {"endpoint: h:1\nservers:\n  a: {address: 10.247.1.5, public_key: " + k1 + "}\n", "servers_subnet"},
		"hub address taken":   {"endpoint: h:1\nservers:\n  a: {address: 10.247.0.1, public_key: " + k1 + "}\n", "the hub"},
		"broadcast":           {"endpoint: h:1\nservers:\n  a: {address: 10.247.0.255, public_key: " + k1 + "}\n", "host address"},
		"duplicate key":       {"endpoint: h:1\nservers:\n  a: {address: 10.247.0.5, public_key: " + k1 + "}\n  b: {address: 10.247.0.6, public_key: " + k1 + "}\n", "already server a"},
		"duplicate address":   {"endpoint: h:1\nservers:\n  a: {address: 10.247.0.5, public_key: " + k1 + "}\n  b: {address: 10.247.0.5, public_key: " + k2 + "}\n", "already server a"},
		"bad name":            {"endpoint: h:1\nservers:\n  Shop_Dev: {address: 10.247.0.5, public_key: " + k1 + "}\n", "lowercase"},
		"bad key":             {"endpoint: h:1\nservers:\n  a: {address: 10.247.0.5, public_key: nope}\n", "public_key"},
		"bad port":            {"endpoint: h:1\nservers:\n  a: {address: 10.247.0.5, public_key: " + k1 + ", ports: [0]}\n", "port 0"},
		"reach unknown":       {"endpoint: h:1\npeople:\n  p:\n    reach: [nope]\n    devices: {d: {address: 10.247.1.5, public_key: " + k1 + "}}\n", "not a server"},
		"star not alone":      {"endpoint: h:1\nservers:\n  a: {address: 10.247.0.5, public_key: " + k2 + "}\npeople:\n  p:\n    reach: ['*', a]\n    devices: {d: {address: 10.247.1.5, public_key: " + k1 + "}}\n", "stands alone"},
		"bad date":            {"endpoint: h:1\npeople:\n  p:\n    reach: []\n    expires: 31.12.2026\n    devices: {d: {address: 10.247.1.5, public_key: " + k1 + "}}\n", "YYYY-MM-DD"},
		"no devices":          {"endpoint: h:1\npeople:\n  p:\n    reach: []\n", "no devices"},
		"device in servers":   {"endpoint: h:1\npeople:\n  p:\n    reach: []\n    devices: {d: {address: 10.247.0.5, public_key: " + k1 + "}}\n", "people_subnet"},
		"hub outside servers": {"endpoint: h:1\nhub_address: 10.247.1.1\n", "hub_address"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse() = %v, want an error mentioning %q", err, tc.want)
			}
		})
	}
}

func TestNextFree(t *testing.T) {
	c, err := Parse([]byte(sampleYAML(t)))
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := c.NextFree(c.ServersPrefix()); a != netip.MustParseAddr("10.247.0.2") {
		t.Errorf("next server address = %s", a)
	}
	if a, _ := c.NextFree(c.PeoplePrefix()); a != netip.MustParseAddr("10.247.1.1") {
		t.Errorf("next person address = %s", a)
	}
}

func TestEditKeepsCommentsAndValidates(t *testing.T) {
	raw := []byte(sampleYAML(t))
	out, err := AddServer(raw, "api-dev", Server{Address: "10.247.0.12", PublicKey: pub(t)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "# the team's access network") {
		t.Error("comment lost")
	}
	if _, err := AddServer(out, "api-dev", Server{Address: "10.247.0.13", PublicKey: pub(t)}); err == nil {
		t.Error("a duplicate server was accepted")
	}
	if _, err := AddServer(out, "x", Server{Address: "10.247.0.12", PublicKey: pub(t)}); err == nil {
		t.Error("a taken address was accepted")
	}

	out, err = AddDevice(out, "carol", "laptop", Device{Address: "10.247.1.30", PublicKey: pub(t)}, []string{"api-dev"}, "2027-03-01")
	if err != nil {
		t.Fatal(err)
	}
	out, err = AddDevice(out, "carol", "phone", Device{Address: "10.247.1.31", PublicKey: pub(t)}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddDevice(out, "dave", "laptop", Device{Address: "10.247.1.40", PublicKey: pub(t)}, nil, ""); err == nil {
		t.Error("a new person without reach was accepted")
	}
	c, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if carol := c.People["carol"]; len(carol.Devices) != 2 || carol.Expires != "2027-03-01" {
		t.Fatalf("carol = %+v", carol)
	}

	// Removing a server drops it from every reach list, so the file stays valid.
	out, err = RemoveServer(out, "api-dev")
	if err != nil {
		t.Fatal(err)
	}
	out, err = RemovePerson(out, "carol", "laptop")
	if err != nil {
		t.Fatal(err)
	}
	out, err = RemovePerson(out, "carol", "phone")
	if err != nil {
		t.Fatal(err)
	}
	c, err = Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.People["carol"]; ok {
		t.Error("a person without devices was kept")
	}
	if _, err := RemovePerson(out, "carol", ""); err == nil {
		t.Error("removing a missing person succeeded")
	}
}

func TestEditEmptyFile(t *testing.T) {
	if _, err := AddServer(nil, "a", Server{Address: "10.247.0.5", PublicKey: pub(t)}); err == nil {
		t.Fatal("a file without an endpoint was accepted")
	}
	out, err := AddServer([]byte("endpoint: vpn.example.com:443\nservers:\n"), "a", Server{Address: "10.247.0.5", PublicKey: pub(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(out); err != nil {
		t.Fatal(err)
	}
}

func TestPlanLeavesDisabledOut(t *testing.T) {
	raw := strings.Replace(sampleYAML(t), "  bob:\n", "  bob:\n    disabled: true\n", 1)
	raw = strings.Replace(raw, "phone: {address: 10.247.1.11,", "phone: {disabled: true, address: 10.247.1.11,", 1)
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	p := c.Plan(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if strings.Join(p.Disabled, " ") != "alice/phone bob/laptop" {
		t.Errorf("disabled = %v", p.Disabled)
	}
	for _, peer := range p.Peers {
		if peer.Name == "alice/phone" || peer.Name == "bob/laptop" {
			t.Errorf("a disabled device is a peer: %s", peer.Name)
		}
	}
	for _, r := range p.Rules {
		if r.FromName != "alice/laptop" {
			t.Errorf("a disabled device has a rule: %+v", r)
		}
	}
	// A disabled device keeps its address: nobody else is given it.
	if a, err := c.NextFree(c.PeoplePrefix()); err != nil || a == netip.MustParseAddr("10.247.1.11") {
		t.Errorf("NextFree = %s, %v: the disabled phone's address was handed out", a, err)
	}
}

func TestSetDisabled(t *testing.T) {
	raw := []byte(sampleYAML(t))
	out, err := SetDisabled(raw, "bob", "", true)
	if err != nil {
		t.Fatal(err)
	}
	out, err = SetDisabled(out, "alice", "phone", true)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if !c.People["bob"].Disabled || !c.People["alice"].Devices["phone"].Disabled || c.People["alice"].Disabled {
		t.Fatalf("disabled flags wrong:\n%s", out)
	}
	if !strings.Contains(string(out), "# the team's access network") {
		t.Error("the comment was lost")
	}
	// Disabling twice changes nothing; enabling removes the key altogether.
	again, err := SetDisabled(out, "bob", "", true)
	if err != nil || string(again) != string(out) {
		t.Errorf("disabling twice changed the file (%v):\n%s", err, again)
	}
	for _, step := range []struct{ person, device string }{{"bob", ""}, {"alice", "phone"}} {
		if out, err = SetDisabled(out, step.person, step.device, false); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(string(out), "disabled") {
		t.Errorf("enabled again, the file still says disabled:\n%s", out)
	}
	if _, err := SetDisabled(raw, "nobody", "", true); err == nil {
		t.Error("an unknown person was accepted")
	}
	if _, err := SetDisabled(raw, "alice", "tablet", true); err == nil {
		t.Error("an unknown device was accepted")
	}
}
