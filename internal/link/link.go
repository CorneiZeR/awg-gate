// Package link drives the tunnel: the userspace amneziawg-go process, the
// `awg` tool that configures it and `ip` for addresses and routes.
//
// amneziawg-go runs as a child of awg-gate in the foreground, so the
// container lives exactly as long as the tunnel does. wg-quick and awg-quick
// are not used at all: they are shell wrappers whose name a base image can
// silently shadow with plain WireGuard's, and everything they would do is a
// handful of the calls below.
package link

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/CorneiZeR/awg-gate/internal/fsutil"
)

// SocketDir is where amneziawg-go puts its control socket.
const SocketDir = "/var/run/amneziawg"

// RunDir holds awg-gate's runtime files: rendered configurations and health.
var RunDir = "/run/awg-gate"

// Device is a running amneziawg-go.
type Device struct {
	Iface   string
	cmd     *exec.Cmd
	exited  chan struct{}
	exitErr error
}

// Start launches amneziawg-go for iface and waits for its control socket.
func Start(ctx context.Context, iface string) (*Device, error) {
	cmd := exec.Command("amneziawg-go", "-f", iface)
	cmd.Env = append(os.Environ(), "WG_PROCESS_FOREGROUND=1")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start amneziawg-go: %w", err)
	}
	d := watch(iface, cmd)

	sock := filepath.Join(SocketDir, iface+".sock")
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(sock); err == nil {
			return d, nil
		}
		select {
		case <-d.exited:
			return nil, fmt.Errorf("amneziawg-go exited before it was ready (is /dev/net/tun passed in and NET_ADMIN granted?): %w", d.exitErr)
		case <-deadline.C:
			return nil, errors.Join(fmt.Errorf("amneziawg-go did not open %s within 15s", sock), d.Stop())
		case <-ctx.Done():
			return nil, errors.Join(ctx.Err(), d.Stop())
		case <-tick.C:
		}
	}
}

// watch follows a started process: its exit closes Exited and is kept for Err,
// so any number of readers - the daemon loop, a deferred Stop - see it.
func watch(iface string, cmd *exec.Cmd) *Device {
	d := &Device{Iface: iface, cmd: cmd, exited: make(chan struct{})}
	go func() {
		d.exitErr = cmd.Wait()
		close(d.exited)
	}()
	return d
}

// Exited is closed when amneziawg-go has exited, for whatever reason.
func (d *Device) Exited() <-chan struct{} { return d.exited }

// Err is how amneziawg-go exited; it waits for the exit.
func (d *Device) Err() error {
	<-d.exited
	return d.exitErr
}

// Stop terminates amneziawg-go, which removes the interface with it. Stopping
// a device that has already exited does nothing.
func (d *Device) Stop() error {
	select {
	case <-d.exited:
		return nil
	default:
	}
	if err := d.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("stop amneziawg-go: %w", err)
	}
	select {
	case <-d.exited:
		return nil
	case <-time.After(5 * time.Second):
		if err := d.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill amneziawg-go: %w", err)
		}
		<-d.exited
		return errors.New("amneziawg-go ignored SIGTERM for 5s and was killed")
	}
}

// SetConf replaces the device configuration; with sync, peers whose settings
// did not change keep their sessions (awg syncconf).
func SetConf(ctx context.Context, iface, text string, sync bool) error {
	verb := "setconf"
	if sync {
		verb = "syncconf"
	}
	// The file holds a private key: it lives only as long as awg reads it.
	return fsutil.WithTempFile(RunDir, []byte(text), func(path string) error {
		_, err := run(ctx, "awg", verb, iface, path)
		return err
	})
}

// SetAddress gives the interface its address (prefix length included).
func SetAddress(ctx context.Context, iface, prefix string) error {
	_, err := run(ctx, "ip", "-4", "address", "replace", prefix, "dev", iface)
	return err
}

// Up sets the MTU and brings the interface up.
func Up(ctx context.Context, iface string, mtu int) error {
	_, err := run(ctx, "ip", "link", "set", "dev", iface, "mtu", strconv.Itoa(mtu), "up")
	return err
}

// Route sends a prefix into the tunnel.
func Route(ctx context.Context, iface, prefix string) error {
	_, err := run(ctx, "ip", "-4", "route", "replace", prefix, "dev", iface)
	return err
}

// SetEndpoint points a peer at a (re-resolved) endpoint.
func SetEndpoint(ctx context.Context, iface, publicKey, endpoint string) error {
	_, err := run(ctx, "awg", "set", iface, "peer", publicKey, "endpoint", endpoint)
	return err
}

// PeerStats is what the device reports about one peer.
type PeerStats struct {
	Endpoint      string
	LastHandshake time.Time
	RxBytes       int64
	TxBytes       int64
}

// Stats reads every peer's endpoint, last handshake and transfer counters.
func Stats(ctx context.Context, iface string) (map[string]*PeerStats, error) {
	out := map[string]*PeerStats{}
	get := func(pub string) *PeerStats {
		if out[pub] == nil {
			out[pub] = &PeerStats{}
		}
		return out[pub]
	}
	hs, err := run(ctx, "awg", "show", iface, "latest-handshakes")
	if err != nil {
		return nil, err
	}
	for _, f := range fields(hs) {
		if len(f) == 2 {
			if sec, err := strconv.ParseInt(f[1], 10, 64); err == nil && sec > 0 {
				get(f[0]).LastHandshake = time.Unix(sec, 0)
			} else {
				get(f[0])
			}
		}
	}
	tr, err := run(ctx, "awg", "show", iface, "transfer")
	if err != nil {
		return nil, err
	}
	for _, f := range fields(tr) {
		if len(f) == 3 {
			s := get(f[0])
			s.RxBytes, _ = strconv.ParseInt(f[1], 10, 64)
			s.TxBytes, _ = strconv.ParseInt(f[2], 10, 64)
		}
	}
	ep, err := run(ctx, "awg", "show", iface, "endpoints")
	if err != nil {
		return nil, err
	}
	for _, f := range fields(ep) {
		if len(f) == 2 && f[1] != "(none)" {
			get(f[0]).Endpoint = f[1]
		}
	}
	return out, nil
}

// IPForwarding reports whether the namespace forwards IPv4 between interfaces.
func IPForwarding() (bool, error) {
	raw, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(raw)) == "1", nil
}

// HasInterface reports whether the current network namespace has iface.
func HasInterface(iface string) bool {
	_, err := os.Stat(filepath.Join("/sys/class/net", iface))
	return err == nil
}

func fields(out []byte) [][]string {
	var rows [][]string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) > 0 {
			rows = append(rows, f)
		}
	}
	return rows
}

func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), msg)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return stdout.Bytes(), nil
}
