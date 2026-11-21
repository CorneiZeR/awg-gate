package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CorneiZeR/awg-gate/internal/gate"
	"github.com/CorneiZeR/awg-gate/internal/keys"
)

// hubFixture sets a hub up in a temporary directory and returns the paths of
// its gate.yaml and hub.json.
func hubFixture(t *testing.T) (gatePath, infoPath string) {
	t.Helper()
	dir := t.TempDir()
	gatePath = filepath.Join(dir, "gate.yaml")
	if err := os.WriteFile(gatePath, []byte("endpoint: vpn.example.com:443\nservers:\npeople:\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var info bytes.Buffer
	if err := cmdHubInfo([]string{"--init", "--config", gatePath, "--data", filepath.Join(dir, "data")}, &info); err != nil {
		t.Fatal(err)
	}
	infoPath = filepath.Join(dir, "hub.json")
	if err := os.WriteFile(infoPath, info.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return gatePath, infoPath
}

func TestIssueJSON(t *testing.T) {
	gatePath, infoPath := hubFixture(t)
	out := t.TempDir()
	var buf bytes.Buffer
	err := cmdIssue([]string{"person", "alice", "phone", "--hub-info", infoPath, "--address", "10.247.1.40", "--out", out, "--json"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	var e issuedEntry
	if err := json.Unmarshal(buf.Bytes(), &e); err != nil {
		t.Fatalf("not one JSON object: %v\n%s", err, buf.String())
	}
	if e.Kind != "person" || e.Name != "alice" || e.Device != "phone" || e.Address != "10.247.1.40" || e.Gate != "" {
		t.Errorf("entry = %+v", e)
	}
	if _, err := keys.Parse(e.PublicKey); err != nil {
		t.Errorf("public_key: %v", err)
	}
	if len(e.Files) != 2 || !strings.HasSuffix(e.Files[0], "alice-phone.conf") || !strings.HasSuffix(e.Files[1], "alice-phone.png") {
		t.Errorf("files = %v", e.Files)
	}
	// The printed key is the one the configuration's private key belongs to:
	// filed in gate.yaml it admits exactly this configuration.
	conf, err := os.ReadFile(e.Files[0])
	if err != nil {
		t.Fatal(err)
	}
	priv := ""
	for line := range strings.SplitSeq(string(conf), "\n") {
		if v, ok := strings.CutPrefix(line, "PrivateKey = "); ok {
			priv = v
		}
	}
	k, err := keys.Parse(priv)
	if err != nil || k.Public().String() != e.PublicKey {
		t.Errorf("printed public key does not match the configuration's private key (%v)", err)
	}

	// With --gate the entry is filed too, and the JSON says where.
	buf.Reset()
	err = cmdIssue([]string{"server", "shop-dev", "--hub-info", infoPath, "--gate", gatePath, "--ports", "443,80", "--out", out, "--json"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	e = issuedEntry{}
	if err := json.Unmarshal(buf.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Kind != "server" || e.Device != "" || e.Gate != gatePath || len(e.Ports) != 2 || len(e.Files) != 1 {
		t.Errorf("entry = %+v", e)
	}
	c, err := gate.Load(gatePath)
	if err != nil {
		t.Fatal(err)
	}
	if c.Servers["shop-dev"].PublicKey != e.PublicKey {
		t.Error("the filed key differs from the printed one")
	}

	if err := cmdIssue([]string{"person", "bob", "laptop", "--hub-info", infoPath, "--address", "10.247.1.41", "--out", out, "--json", "--qr"}, &buf); err == nil {
		t.Error("--json with --qr was accepted")
	}
}

func TestDisableEnable(t *testing.T) {
	gatePath, infoPath := hubFixture(t)
	var buf bytes.Buffer
	if err := cmdIssue([]string{"server", "a", "--hub-info", infoPath, "--gate", gatePath, "--out", t.TempDir()}, &buf); err != nil {
		t.Fatal(err)
	}
	if err := cmdIssue([]string{"person", "bob", "laptop", "--hub-info", infoPath, "--gate", gatePath, "--reach", "a", "--out", t.TempDir()}, &buf); err != nil {
		t.Fatal(err)
	}
	disabled := func() bool {
		t.Helper()
		c, err := gate.Load(gatePath)
		if err != nil {
			t.Fatal(err)
		}
		return len(c.Plan(time.Now()).Disabled) == 1
	}
	if err := cmdSetDisabled(true, []string{"person", "bob", "--gate", gatePath}, &buf); err != nil {
		t.Fatal(err)
	}
	if !disabled() {
		t.Fatal("bob is not disabled")
	}
	if err := cmdSetDisabled(false, []string{"person", "bob", "--gate", gatePath}, &buf); err != nil {
		t.Fatal(err)
	}
	if disabled() {
		t.Fatal("bob is still disabled")
	}
	if err := cmdSetDisabled(true, []string{"server", "a", "--gate", gatePath}, &buf); err == nil {
		t.Error("disabling a server was accepted")
	}
}
