package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CorneiZeR/awg-gate/internal/gate"
)

func TestInitCreatesOnceAndKeeps(t *testing.T) {
	dir := t.TempDir()
	h1, created, err := Init(dir, "v1")
	if err != nil || !created {
		t.Fatalf("first Init: created=%v err=%v", created, err)
	}
	for _, f := range []string{keyFile, paramsFile} {
		st, err := os.Stat(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %o, want 600", f, st.Mode().Perm())
		}
	}
	h2, created, err := Init(dir, "v1")
	if err != nil || created {
		t.Fatalf("second Init: created=%v err=%v", created, err)
	}
	if h1.PrivateKey != h2.PrivateKey || h1.Params != h2.Params {
		t.Fatal("a second start drew new values")
	}
}

func TestInitRefusesProfileChange(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := Init(dir, "v1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Init(dir, "v3"); err == nil || !strings.Contains(err.Error(), "reissuing") {
		t.Fatalf("profile change: %v", err)
	}
}

func TestInitKeepsAKeyWhenParamsAreMissing(t *testing.T) {
	dir := t.TempDir()
	h1, _, err := Init(dir, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, paramsFile)); err != nil {
		t.Fatal(err)
	}
	h2, created, err := Init(dir, "v3")
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if h2.PrivateKey != h1.PrivateKey || h2.Params.Profile != "v3" {
		t.Fatal("the key was redrawn or the new profile ignored")
	}
}

func TestInfoRoundTrip(t *testing.T) {
	dir := t.TempDir()
	h, _, err := Init(dir, "v3")
	if err != nil {
		t.Fatal(err)
	}
	c, err := gate.Parse([]byte("endpoint: vpn.example.com:443\n"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(NewInfo(c, h))
	if strings.Contains(string(raw), h.PrivateKey.String()) {
		t.Fatal("hub info carries the private key")
	}
	path := filepath.Join(dir, "hub.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	i, err := ReadInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if i.PublicKey != h.PrivateKey.Public().String() || i.ServersSubnet != "10.247.0.0/24" || i.Params != h.Params {
		t.Fatalf("info = %+v", i)
	}
}
