package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/CorneiZeR/awg-gate/internal/conf"
	"github.com/CorneiZeR/awg-gate/internal/gate"
	"github.com/CorneiZeR/awg-gate/internal/link"
	"github.com/CorneiZeR/awg-gate/internal/nft"
	"github.com/CorneiZeR/awg-gate/internal/state"
)

// HubOptions configure the hub.
type HubOptions struct {
	ConfigPath string
	DataDir    string
	// Interval is how often gate.yaml is re-read and access dates re-checked.
	Interval time.Duration
	Log      *slog.Logger
}

// HubConf renders the device configuration of a hub for a plan.
func HubConf(c *gate.Config, h *state.Hub, p gate.Plan) conf.File {
	iface := conf.Section{
		{Key: "PrivateKey", Value: h.PrivateKey.String()},
		{Key: "ListenPort", Value: strconv.Itoa(c.ListenPort)},
	}
	for _, l := range h.Params.Lines() {
		iface = append(iface, conf.KV{Key: l[0], Value: l[1]})
	}
	f := conf.File{Interface: iface}
	for _, peer := range p.Peers {
		f.Peers = append(f.Peers, conf.Section{
			{Key: "PublicKey", Value: peer.PublicKey.String()},
			{Key: "AllowedIPs", Value: peer.Address.String() + "/32"},
		})
	}
	return f
}

// restartOnly are settings a running hub cannot take over from a reload.
func restartOnly(a, b *gate.Config) []string {
	var changed []string
	check := func(name string, same bool) {
		if !same {
			changed = append(changed, name)
		}
	}
	check("interface", a.Interface == b.Interface)
	check("listen_port", a.ListenPort == b.ListenPort)
	check("profile", a.Profile == b.Profile)
	check("mtu", a.MTU == b.MTU)
	check("network", a.NetworkPrefix() == b.NetworkPrefix())
	check("hub_address", a.HubAddr() == b.HubAddr())
	return changed
}

// RunHub runs the hub until ctx ends or the tunnel process dies.
func RunHub(ctx context.Context, o HubOptions) error {
	log := o.Log
	cfg, err := gate.Load(o.ConfigPath)
	if err != nil {
		return err
	}
	if fwd, err := link.IPForwarding(); err != nil || !fwd {
		return errors.New("IPv4 forwarding is off in this network namespace: run the container with the sysctl net.ipv4.ip_forward=1")
	}
	hub, created, err := state.Init(o.DataDir, cfg.Profile)
	if err != nil {
		return err
	}
	if created {
		log.Info("new hub: drew a key and obfuscation parameters; back up the data volume and export `awg-gate hub-info`",
			"data", o.DataDir, "profile", hub.Params.Profile)
	}
	log.Info("hub key", "public_key", hub.PrivateKey.Public().String())

	dev, err := link.Start(ctx, cfg.Interface)
	if err != nil {
		return err
	}
	defer func() {
		if err := dev.Stop(); err != nil {
			log.Warn("stop the tunnel", "err", err)
		}
	}()
	defer func() {
		if err := nft.Remove(context.Background()); err != nil {
			log.Warn("remove firewall table", "err", err)
		}
	}()

	prefix := fmt.Sprintf("%s/%d", cfg.HubAddr(), cfg.NetworkPrefix().Bits())
	if err := link.SetAddress(ctx, cfg.Interface, prefix); err != nil {
		return err
	}
	if err := link.Up(ctx, cfg.Interface, cfg.MTU); err != nil {
		return err
	}

	health := Health{Role: "hub", Interface: cfg.Interface, ConfigPath: o.ConfigPath}
	var applied string
	apply := func(c *gate.Config) error {
		plan := c.Plan(time.Now())
		devConf := HubConf(c, hub, plan).Render()
		rules := nft.Hub(c.Interface, plan.Rules)
		if devConf+rules == applied {
			return nil
		}
		// The firewall goes before the peers: a removed person loses their rules
		// before their key, and a new one has rules before they can connect.
		if err := nft.Apply(ctx, rules); err != nil {
			return err
		}
		if err := link.SetConf(ctx, c.Interface, devConf, applied != ""); err != nil {
			return err
		}
		applied = devConf + rules
		servers, devices := 0, 0
		for _, p := range plan.Peers {
			if p.Kind == gate.KindServer {
				servers++
			} else {
				devices++
			}
		}
		log.Info("applied", "servers", servers, "devices", devices, "rules", len(plan.Rules),
			"expired", strings.Join(plan.Expired, ","))
		return nil
	}
	if err := apply(cfg); err != nil {
		return err
	}
	health.OK = true
	if err := writeHealth(health); err != nil {
		log.Warn("write health", "err", err)
	}

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	interval := o.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()

	reload := func(reason string) {
		next, err := gate.Load(o.ConfigPath)
		if err != nil {
			if health.ConfigError != err.Error() {
				log.Error("gate.yaml rejected, still serving the last good one", "reason", reason, "err", err)
			}
			health.ConfigError = err.Error()
		} else {
			health.ConfigError = ""
			if changed := restartOnly(cfg, next); len(changed) > 0 {
				log.Warn("these settings take effect only after a restart; peers and rules are applied now",
					"settings", strings.Join(changed, ","))
				// Peers and rules follow the new file; device-level settings stay
				// as running, so the next comparison is against what runs.
				next.Interface, next.ListenPort, next.MTU = cfg.Interface, cfg.ListenPort, cfg.MTU
			}
			if err := apply(next); err != nil {
				log.Error("apply", "err", err)
				health.OK, health.Reason = false, "apply failed: "+err.Error()
			} else {
				cfg = next
				health.OK, health.Reason = true, ""
			}
		}
		if err := writeHealth(health); err != nil {
			log.Warn("write health", "err", err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("stopping")
			return nil
		case <-dev.Exited():
			return fmt.Errorf("amneziawg-go exited: %w", dev.Err())
		case <-hup:
			reload("SIGHUP")
		case <-tick.C:
			reload("interval")
		}
	}
}
