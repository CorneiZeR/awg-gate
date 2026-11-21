// Command awg-gate is an AmneziaWG access network for reaching private
// servers: one hub, servers as spokes, people as peers, access per person.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/CorneiZeR/awg-gate/internal/daemon"
	"github.com/CorneiZeR/awg-gate/internal/fsutil"
	"github.com/CorneiZeR/awg-gate/internal/gate"
	"github.com/CorneiZeR/awg-gate/internal/issue"
	"github.com/CorneiZeR/awg-gate/internal/keys"
	"github.com/CorneiZeR/awg-gate/internal/link"
	"github.com/CorneiZeR/awg-gate/internal/nft"
	"github.com/CorneiZeR/awg-gate/internal/state"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = ""

const usage = `awg-gate - an AmneziaWG access network for reaching private servers

Daemons (run in the container):
  hub        run the hub: peers and per-person access from gate.yaml
  up         run a tunnel end from a configuration file: a server (spoke) or a Linux client

Operator commands:
  hub-info   print what clients need from a hub (no private key); --init sets a new hub up
  issue      issue a configuration: issue person NAME DEVICE | issue server NAME
  revoke     remove from gate.yaml:  revoke person NAME [DEVICE] | revoke server NAME
  check      validate gate.yaml and print what it grants
  status     show peers, handshakes and traffic of the running hub or tunnel
  health     exit 0 when the running daemon is healthy (the container health check)
  version    print the version

Run "awg-gate COMMAND -h" for a command's flags. Every flag also reads an
AWG_GATE_* environment variable, named in its help.
`

func main() {
	if len(os.Args) < 2 {
		_, _ = fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "hub":
		err = cmdHub(args)
	case "up":
		err = cmdUp(args)
	case "hub-info":
		err = cmdHubInfo(args, os.Stdout)
	case "issue":
		err = cmdIssue(args, os.Stdout)
	case "revoke":
		err = cmdRevoke(args, os.Stdout)
	case "check":
		err = cmdCheck(args, os.Stdout)
	case "status":
		err = cmdStatus(args, os.Stdout)
	case "health":
		err = cmdHealth()
	case "version", "--version", "-v":
		fmt.Println(buildVersion())
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		_, _ = fmt.Fprintln(os.Stderr, "awg-gate "+cmd+": "+err.Error())
		os.Exit(1)
	}
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "dev"
}

func logger() *slog.Logger {
	level := slog.LevelInfo
	if strings.EqualFold(os.Getenv("AWG_GATE_LOG_LEVEL"), "debug") {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("awg-gate "+name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}

const (
	defaultGate   = "/config/gate.yaml"
	defaultData   = "/data"
	defaultTunnel = "/config/awg.conf"
)

func cmdHub(args []string) error {
	fs := newFlags("hub")
	configPath := fs.String("config", env("AWG_GATE_CONFIG", defaultGate), "gate.yaml (AWG_GATE_CONFIG)")
	dataDir := fs.String("data", env("AWG_GATE_DATA", defaultData), "state directory: hub key and obfuscation parameters (AWG_GATE_DATA)")
	interval := fs.Duration("interval", 30*time.Second, "how often gate.yaml and access dates are re-checked; SIGHUP re-reads at once")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signalContext()
	defer stop()
	return daemon.RunHub(ctx, daemon.HubOptions{ConfigPath: *configPath, DataDir: *dataDir, Interval: *interval, Log: logger()})
}

func cmdUp(args []string) error {
	fs := newFlags("up")
	configPath := fs.String("config", env("AWG_GATE_CONFIG", defaultTunnel), "tunnel configuration issued by `awg-gate issue` (AWG_GATE_CONFIG)")
	iface := fs.String("interface", env("AWG_GATE_INTERFACE", gate.DefaultInterface), "interface name (AWG_GATE_INTERFACE)")
	allow := fs.String("allow-ports", os.Getenv("AWG_GATE_ALLOW_PORTS"), "comma-separated TCP/UDP ports reachable over the tunnel; empty for all (AWG_GATE_ALLOW_PORTS)")
	lockdown := fs.Bool("lockdown", envBool("AWG_GATE_LOCKDOWN"), "sidecar only: drop every new connection that does not come over the tunnel (AWG_GATE_LOCKDOWN)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ports, err := parsePorts(*allow)
	if err != nil {
		return fmt.Errorf("allow-ports: %w", err)
	}
	ctx, stop := signalContext()
	defer stop()
	return daemon.RunUp(ctx, daemon.UpOptions{
		ConfigPath: *configPath, Interface: *iface,
		Firewall: nft.SpokeOptions{AllowPorts: ports, Lockdown: *lockdown},
		Log:      logger(),
	})
}

func envBool(name string) bool {
	v, _ := strconv.ParseBool(os.Getenv(name))
	return v
}

func parsePorts(s string) ([]int, error) {
	var out []int
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("%q is not a port", p)
		}
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

func cmdHubInfo(args []string, w io.Writer) error {
	fs := newFlags("hub-info")
	configPath := fs.String("config", env("AWG_GATE_CONFIG", defaultGate), "gate.yaml (AWG_GATE_CONFIG)")
	dataDir := fs.String("data", env("AWG_GATE_DATA", defaultData), "state directory (AWG_GATE_DATA)")
	initNew := fs.Bool("init", false, "create the hub key and obfuscation parameters when missing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := gate.Load(*configPath)
	if err != nil {
		return err
	}
	var h *state.Hub
	if *initNew {
		h, _, err = state.Init(*dataDir, c.Profile)
	} else {
		h, err = state.Load(*dataDir)
	}
	if err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(state.NewInfo(c, h), "", "  ")
	_, err = fmt.Fprintln(w, string(raw))
	return err
}

func cmdIssue(args []string, w io.Writer) error {
	if len(args) == 0 || (args[0] != "person" && args[0] != "server") {
		return errors.New("issue person NAME DEVICE [flags] | issue server NAME [flags]")
	}
	kind := args[0]
	nArgs := 2
	if kind == "server" {
		nArgs = 1
	}
	if len(args) < 1+nArgs {
		return fmt.Errorf("issue %s needs %d name(s)", kind, nArgs)
	}
	names := args[1 : 1+nArgs]
	fs := newFlags("issue " + kind)
	infoPath := fs.String("hub-info", env("AWG_GATE_HUB_INFO", "hub.json"), "the hub's `awg-gate hub-info` output (AWG_GATE_HUB_INFO)")
	gatePath := fs.String("gate", os.Getenv("AWG_GATE_GATE"), "gate.yaml to add the entry to; picks a free address (AWG_GATE_GATE)")
	address := fs.String("address", "", "the address to give; required without --gate")
	outDir := fs.String("out", ".", "directory the configuration (and for a person the QR code) is written to")
	reach := fs.String("reach", "", "person: comma-separated servers a NEW person reaches, or *")
	expires := fs.String("expires", "", "person: last day of access of a NEW person, YYYY-MM-DD")
	ports := fs.String("ports", "", "server: the only TCP/UDP ports people may reach on it")
	showQR := fs.Bool("qr", false, "person: also draw the QR code in the terminal")
	if err := fs.Parse(args[1+nArgs:]); err != nil {
		return err
	}
	info, err := state.ReadInfo(*infoPath)
	if err != nil {
		return err
	}
	var (
		cfg     *gate.Config
		gateRaw []byte
	)
	if *gatePath != "" {
		if gateRaw, err = fsutil.ReadFile(*gatePath); err != nil {
			return err
		}
		if cfg, err = gate.Parse(gateRaw); err != nil {
			return err
		}
		if cfg.ServersPrefix().String() != info.ServersSubnet || cfg.PeoplePrefix().String() != info.PeopleSubnet {
			return fmt.Errorf("%s and %s describe different networks; is the hub info from this hub?", *gatePath, *infoPath)
		}
	}
	subnet := netip.MustParsePrefix(info.PeopleSubnet)
	if kind == "server" {
		subnet = netip.MustParsePrefix(info.ServersSubnet)
	}
	var addr netip.Addr
	switch {
	case *address != "":
		if addr, err = netip.ParseAddr(*address); err != nil {
			return fmt.Errorf("address: %w", err)
		}
	case cfg != nil:
		if addr, err = cfg.NextFree(subnet); err != nil {
			return err
		}
	default:
		return errors.New("give --address, or --gate so a free one is picked")
	}
	if !subnet.Contains(addr) {
		return fmt.Errorf("address %s is outside %s", addr, subnet)
	}

	priv, err := keys.Generate(nil)
	if err != nil {
		return err
	}
	pub := priv.Public().String()
	var (
		label, base, snippet string
		file                 string
		updated              []byte
	)
	if kind == "server" {
		label, base = "server "+names[0], names[0]
		portList, err := parsePorts(*ports)
		if err != nil {
			return fmt.Errorf("ports: %w", err)
		}
		srv := gate.Server{Address: addr.String(), PublicKey: pub, Ports: portList}
		file = issue.Header(label) + issue.Server(info, priv, addr).Render()
		if gateRaw != nil {
			if updated, err = gate.AddServer(gateRaw, names[0], srv); err != nil {
				return err
			}
		}
		snippet = fmt.Sprintf("servers:\n  %s:\n    address: %s\n    public_key: %s\n", names[0], addr, pub)
		if len(portList) > 0 {
			snippet += fmt.Sprintf("    ports: %v\n", strings.Join(strings.Fields(fmt.Sprint(portList)), ", "))
		}
	} else {
		person, device := names[0], names[1]
		label, base = person+"/"+device, person+"-"+device
		var reachList []string
		for r := range strings.SplitSeq(*reach, ",") {
			if r = strings.TrimSpace(r); r != "" {
				reachList = append(reachList, r)
			}
		}
		dev := gate.Device{Address: addr.String(), PublicKey: pub}
		file = issue.Header(label) + issue.Person(info, priv, addr).Render()
		if gateRaw != nil {
			if updated, err = gate.AddDevice(gateRaw, person, device, dev, reachList, *expires); err != nil {
				return err
			}
		}
		snippet = fmt.Sprintf("people:\n  %s:\n    devices:\n      %s: {address: %s, public_key: %s}\n", person, device, addr, pub)
	}

	if err := os.MkdirAll(filepath.Clean(*outDir), 0o700); err != nil {
		return err
	}
	outPath := filepath.Clean(*outDir)
	confPath := filepath.Join(outPath, base+".conf")
	for _, p := range []string{confPath, filepath.Join(outPath, base+".png")} {
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("%s already exists; an issued key is never overwritten", p)
		}
	}
	if err := fsutil.WriteNew(confPath, []byte(file), 0o600); err != nil {
		return err
	}
	written := []string{confPath}
	if kind == "person" {
		png, err := issue.QRPNG(file)
		if err != nil {
			return err
		}
		pngPath := filepath.Join(outPath, base+".png")
		if err := fsutil.WriteNew(pngPath, png, 0o600); err != nil {
			return err
		}
		written = append(written, pngPath)
	}
	out := &printer{w: w}
	if updated != nil {
		if err := writeGate(*gatePath, updated); err != nil {
			return err
		}
		out.f("added %s (%s) to %s\n", label, addr, *gatePath)
	} else {
		out.f("add to gate.yaml:\n\n%s\n", snippet)
	}
	out.f("wrote %s - it holds a private key: deliver it to its owner and delete your copy\n", strings.Join(written, ", "))
	if *showQR && kind == "person" {
		term, err := issue.QRTerminal(file)
		if err != nil {
			return err
		}
		out.f("\n%s", term)
	}
	return out.err
}

func cmdRevoke(args []string, w io.Writer) error {
	if len(args) < 2 || (args[0] != "person" && args[0] != "server") {
		return errors.New("revoke person NAME [DEVICE] [--gate FILE] | revoke server NAME [--gate FILE]")
	}
	kind, name := args[0], args[1]
	rest := args[2:]
	device := ""
	if kind == "person" && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		device, rest = rest[0], rest[1:]
	}
	fs := newFlags("revoke " + kind)
	gatePath := fs.String("gate", env("AWG_GATE_GATE", "gate.yaml"), "gate.yaml to remove the entry from (AWG_GATE_GATE)")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	raw, err := fsutil.ReadFile(*gatePath)
	if err != nil {
		return err
	}
	var out []byte
	if kind == "server" {
		out, err = gate.RemoveServer(raw, name)
	} else {
		out, err = gate.RemovePerson(raw, name, device)
	}
	if err != nil {
		return err
	}
	if err := writeGate(*gatePath, out); err != nil {
		return err
	}
	what := kind + " " + name
	if device != "" {
		what += "/" + device
	}
	msg := &printer{w: w}
	msg.f("removed %s from %s; the hub drops it on its next re-read (or at once on SIGHUP)\n", what, *gatePath)
	return msg.err
}

func cmdCheck(args []string, w io.Writer) error {
	fs := newFlags("check")
	configPath := fs.String("config", env("AWG_GATE_CONFIG", "gate.yaml"), "gate.yaml (AWG_GATE_CONFIG)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := gate.Load(*configPath)
	if err != nil {
		return err
	}
	plan := c.Plan(time.Now())
	out := &printer{w: w}
	out.f("%s is valid: %d servers, %d people, profile %s, network %s\n",
		*configPath, len(c.Servers), len(c.People), c.Profile, c.NetworkPrefix())
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	table := &printer{w: tw}
	table.f("DEVICE\tSERVER\tPORTS\n")
	for _, r := range plan.Rules {
		ports := "all"
		if len(r.Ports) > 0 {
			ports = strings.Trim(strings.Join(strings.Fields(fmt.Sprint(r.Ports)), ","), "[]")
		}
		table.f("%s\t%s\t%s\n", r.FromName, r.ToName, ports)
	}
	if err := errors.Join(table.err, tw.Flush()); err != nil {
		return err
	}
	if len(plan.Expired) > 0 {
		out.f("expired, not admitted: %s\n", strings.Join(plan.Expired, ", "))
	}
	return out.err
}

func cmdStatus(args []string, w io.Writer) error {
	fs := newFlags("status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	h, err := daemon.ReadHealth()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stats, err := link.Stats(ctx, h.Interface)
	if err != nil {
		return err
	}
	state := "healthy"
	if !h.OK {
		state = "UNHEALTHY: " + h.Reason
	}
	out := &printer{w: w}
	out.f("%s on %s, %s (reported %s ago)\n", h.Role, h.Interface, state, time.Since(h.At).Round(time.Second))
	if h.ConfigError != "" {
		out.f("gate.yaml REJECTED, serving the last good one: %s\n", h.ConfigError)
	}
	names := map[string][3]string{}
	if h.Role == "hub" {
		c, err := gate.Load(h.ConfigPath)
		if err != nil {
			out.f("peers are shown by key: %v\n", err)
		} else {
			for _, p := range c.Plan(time.Now()).Peers {
				names[p.PublicKey.String()] = [3]string{p.Name, p.Address.String(), p.Expires}
			}
		}
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	table := &printer{w: tw}
	table.f("PEER\tADDRESS\tENDPOINT\tHANDSHAKE\tRX\tTX\tEXPIRES\n")
	pubs := make([]string, 0, len(stats))
	for pub := range stats {
		pubs = append(pubs, pub)
	}
	sort.Slice(pubs, func(i, j int) bool { return names[pubs[i]][0]+pubs[i] < names[pubs[j]][0]+pubs[j] })
	for _, pub := range pubs {
		s := stats[pub]
		n := names[pub]
		if n[0] == "" {
			n[0] = pub[:10] + "..."
		}
		hs := "never"
		if !s.LastHandshake.IsZero() {
			hs = time.Since(s.LastHandshake).Round(time.Second).String() + " ago"
		}
		table.f("%s\t%s\t%s\t%s\t%s\t%s\t%s\n", n[0], dash(n[1]), dash(s.Endpoint), hs,
			bytesHuman(s.RxBytes), bytesHuman(s.TxBytes), dash(n[2]))
	}
	return errors.Join(out.err, table.err, tw.Flush())
}

func cmdHealth() error {
	h, err := daemon.CheckHealth(time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("%s ok\n", h.Role)
	return nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func bytesHuman(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// writeGate replaces gate.yaml atomically, keeping its permissions.
func writeGate(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(filepath.Clean(path)); err == nil {
		mode = st.Mode().Perm()
	}
	return fsutil.WriteAtomic(path, data, mode)
}

// printer writes command output and keeps the first write error, so a command
// can print freely and still report a closed stdout.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) f(format string, a ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, a...)
	}
}
