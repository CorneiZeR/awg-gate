package link

import (
	"os/exec"
	"testing"
	"time"
)

func started(t *testing.T, name string, args ...string) *Device {
	t.Helper()
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Skipf("%s: %v", name, err)
	}
	return watch("test0", cmd)
}

func stopWithin(t *testing.T, d *Device, limit time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- d.Stop() }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("Stop did not return within %s", limit)
		return nil
	}
}

// The daemon loop sees the exit first and then its deferred Stop runs: Stop must
// not wait for an exit that has already been reported.
func TestStopAfterTheExitWasSeen(t *testing.T) {
	d := started(t, "true")
	select {
	case <-d.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the process did not exit")
	}
	errc := make(chan error, 1)
	go func() { errc <- d.Err() }()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("Err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Err did not return for a process whose exit was already seen")
	}
	if err := stopWithin(t, d, 2*time.Second); err != nil {
		t.Fatalf("Stop = %v", err)
	}
}

func TestStopTerminatesARunningProcess(t *testing.T) {
	d := started(t, "sleep", "30")
	if err := stopWithin(t, d, 4*time.Second); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	select {
	case <-d.Exited():
	default:
		t.Fatal("Exited is not closed after Stop")
	}
}
