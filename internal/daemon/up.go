package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/CorneiZeR/awg-gate/internal/conf"
	"github.com/CorneiZeR/awg-gate/internal/fsutil"
	"github.com/CorneiZeR/awg-gate/internal/gate"
	"github.com/CorneiZeR/awg-gate/internal/link"
	"github.com/CorneiZeR/awg-gate/internal/nft"
)

// UpOptions configure a spoke or a Linux client.
type UpOptions struct {
	ConfigPath string
	Interface  string
	Firewall   nft.SpokeOptions
	Interval   time.Duration
	// StaleAfter is how old the newest handshake may get before the tunnel
	// counts as down. With a 25 s keepalive a live tunnel re-handshakes about
	// every two minutes.
	StaleAfter time.Duration
	Log        *slog.Logger
}

// tunnel is an `up` configuration, read and checked.
type tunnel struct {
	file      conf.File
	addresses []netip.Prefix
	routes    []netip.Prefix
	mtu       int
	// endpoints by peer public key, as written (a name stays a name, so it can be re-resolved).
	endpoints map[string]string
}

func readTunnel(path string) (*tunnel, error) {
	raw, err := fsutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := conf.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	t := &tunnel{file: f, mtu: gate.DefaultMTU, endpoints: map[string]string{}}
	if _, ok := f.Interface.Get("PrivateKey"); !ok {
		return nil, fmt.Errorf("%s: [Interface] has no PrivateKey", path)
	}
	for _, a := range f.Interface.List("Address") {
		p, err := netip.ParsePrefix(a)
		if err != nil || !p.Addr().Is4() {
			return nil, fmt.Errorf("%s: Address %q: only IPv4 prefixes are supported", path, a)
		}
		t.addresses = append(t.addresses, p)
	}
	if len(t.addresses) == 0 {
		return nil, fmt.Errorf("%s: [Interface] has no Address", path)
	}
	if v, ok := f.Interface.Get("MTU"); ok {
		if t.mtu, err = strconv.Atoi(v); err != nil || t.mtu < 1280 || t.mtu > 1420 {
			return nil, fmt.Errorf("%s: MTU %q must be 1280..1420", path, v)
		}
	}
	if len(f.Peers) == 0 {
		return nil, fmt.Errorf("%s: no [Peer]", path)
	}
	for _, p := range f.Peers {
		pub, _ := p.Get("PublicKey")
		if ep, ok := p.Get("Endpoint"); ok {
			t.endpoints[pub] = ep
		}
		for _, a := range p.List("AllowedIPs") {
			pr, err := netip.ParsePrefix(a)
			if err != nil || !pr.Addr().Is4() {
				return nil, fmt.Errorf("%s: AllowedIPs %q: only IPv4 prefixes are supported", path, a)
			}
			if pr.Bits() == 0 {
				return nil, fmt.Errorf("%s: AllowedIPs %s would route everything; awg-gate is split-tunnel only", path, pr)
			}
			t.routes = append(t.routes, pr.Masked())
		}
	}
	return t, nil
}

// RunUp brings a tunnel up and keeps it healthy until ctx ends.
func RunUp(ctx context.Context, o UpOptions) error {
	log := o.Log
	t, err := readTunnel(o.ConfigPath)
	if err != nil {
		return err
	}
	if o.Firewall.Lockdown && link.HasInterface("docker0") {
		return errors.New("lockdown drops everything that does not come through the tunnel, and this " +
			"network namespace has docker0 — it looks like a host's; lockdown is for a sidecar's own namespace only")
	}
	dev, err := link.Start(ctx, o.Interface)
	if err != nil {
		return err
	}
	defer func() {
		if err := dev.Stop(); err != nil {
			log.Warn("stop the tunnel", "err", err)
		}
	}()
	// An endpoint given by name is set after the device is up and re-resolved
	// until a handshake happens: a name that does not resolve yet (DNS not up
	// at boot, the hub starting later) must not take the tunnel down.
	if err := link.SetConf(ctx, o.Interface, t.withoutNamedEndpoints().Render(), false); err != nil {
		return err
	}
	for pub, ep := range t.endpoints {
		if hasHostname(ep) {
			if err := link.SetEndpoint(ctx, o.Interface, pub, ep); err != nil {
				log.Warn("endpoint does not resolve yet, retrying", "endpoint", ep, "err", err)
			}
		}
	}
	for _, a := range t.addresses {
		if err := link.SetAddress(ctx, o.Interface, a.String()); err != nil {
			return err
		}
	}
	if err := link.Up(ctx, o.Interface, t.mtu); err != nil {
		return err
	}
	for _, r := range t.routes {
		if err := link.Route(ctx, o.Interface, r.String()); err != nil {
			return err
		}
	}
	if rules := nft.Spoke(o.Interface, o.Firewall); rules != "" {
		if err := nft.Apply(ctx, rules); err != nil {
			return err
		}
		defer func() {
			if err := nft.Remove(context.Background()); err != nil {
				log.Warn("remove firewall table", "err", err)
			}
		}()
	}
	addrs := make([]string, len(t.addresses))
	for i, a := range t.addresses {
		addrs[i] = a.String()
	}
	log.Info("tunnel up", "interface", o.Interface, "address", strings.Join(addrs, ","),
		"allow_ports", fmt.Sprint(o.Firewall.AllowPorts), "lockdown", o.Firewall.Lockdown)

	interval := o.Interval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	stale := o.StaleAfter
	if stale <= 0 {
		stale = 3 * time.Minute
	}
	started := time.Now()
	health := Health{Role: "up", Interface: o.Interface, ConfigPath: o.ConfigPath}
	wasUp := false
	check := func() {
		stats, err := link.Stats(ctx, o.Interface)
		if err != nil {
			health.OK, health.Reason = false, err.Error()
			_ = writeHealth(health)
			return
		}
		var newest time.Time
		for pub, s := range stats {
			if s.LastHandshake.After(newest) {
				newest = s.LastHandshake
			}
			// No recent handshake: resolve the endpoint's name again, it may have moved.
			if ep := t.endpoints[pub]; ep != "" && time.Since(s.LastHandshake) > stale && hasHostname(ep) {
				if err := link.SetEndpoint(ctx, o.Interface, pub, ep); err != nil {
					log.Warn("re-resolve endpoint", "endpoint", ep, "err", err)
				}
			}
		}
		switch {
		case !newest.IsZero() && time.Since(newest) <= stale:
			health.OK, health.Reason = true, ""
			if !wasUp {
				log.Info("handshake with the hub", "at", newest.UTC().Format(time.RFC3339))
			}
			wasUp = true
		case newest.IsZero():
			health.OK, health.Reason = false, fmt.Sprintf("no handshake yet (%s since start)", time.Since(started).Round(time.Second))
		default:
			health.OK, health.Reason = false, fmt.Sprintf("last handshake %s ago", time.Since(newest).Round(time.Second))
		}
		if !health.OK && wasUp {
			log.Warn("tunnel down", "reason", health.Reason)
			wasUp = false
		}
		if err := writeHealth(health); err != nil {
			log.Warn("write health", "err", err)
		}
	}
	check()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("stopping")
			return nil
		case <-dev.Exited():
			return fmt.Errorf("amneziawg-go exited: %w", dev.Err())
		case <-tick.C:
			check()
		}
	}
}

// withoutNamedEndpoints is the setconf file with every Endpoint that is a
// name left out; RunUp sets those itself.
func (t *tunnel) withoutNamedEndpoints() conf.File {
	f := t.file.ForSetconf()
	peers := make([]conf.Section, len(f.Peers))
	for i, p := range f.Peers {
		for _, kv := range p {
			if strings.EqualFold(kv.Key, "Endpoint") && hasHostname(kv.Value) {
				continue
			}
			peers[i] = append(peers[i], kv)
		}
	}
	f.Peers = peers
	return f
}

func hasHostname(endpoint string) bool {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return false
	}
	_, err = netip.ParseAddr(host)
	return err != nil
}
