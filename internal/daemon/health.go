// Package daemon runs the two long-lived roles: the hub, and `up` — the
// tunnel end a server (spoke) or a Linux client runs.
package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/CorneiZeR/awg-gate/internal/link"
)

// Health is what a daemon last found about itself; `awg-gate health` reads it.
type Health struct {
	Role   string    `json:"role"`
	OK     bool      `json:"ok"`
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at"`
	// ConfigError is a gate.yaml that failed to load on a reload; the hub
	// keeps serving the last good one meanwhile.
	ConfigError string `json:"config_error,omitempty"`
	Interface   string `json:"interface"`
	ConfigPath  string `json:"config_path"`
}

// healthMaxAge: a report older than this means the daemon loop is stuck.
const healthMaxAge = 2 * time.Minute

func healthPath() string { return filepath.Join(link.RunDir, "health.json") }

func writeHealth(h Health) error {
	h.At = time.Now().UTC()
	raw, _ := json.Marshal(h)
	if err := os.MkdirAll(link.RunDir, 0o700); err != nil {
		return err
	}
	tmp := healthPath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, healthPath())
}

// ReadHealth returns the last report.
func ReadHealth() (Health, error) {
	raw, err := os.ReadFile(healthPath())
	if err != nil {
		return Health{}, fmt.Errorf("no health report yet (%w)", err)
	}
	var h Health
	if err := json.Unmarshal(raw, &h); err != nil {
		return Health{}, err
	}
	return h, nil
}

// CheckHealth is the container health check: fresh and ok, or an error saying why not.
func CheckHealth(now time.Time) (Health, error) {
	h, err := ReadHealth()
	if err != nil {
		return h, err
	}
	if age := now.Sub(h.At); age > healthMaxAge {
		return h, fmt.Errorf("%s: last report is %s old", h.Role, age.Round(time.Second))
	}
	if !h.OK {
		return h, errors.New(h.Role + ": " + h.Reason)
	}
	return h, nil
}
