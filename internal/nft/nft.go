// Package nft renders the nftables rules awg-gate keeps in its own table,
// `inet awg_gate`.
//
// The rules only ever drop traffic that crosses the tunnel interface (lockdown,
// for a sidecar's own namespace, is the one exception). A drop
// is final in nftables whatever table it sits in, while an accept is not, so
// this table can narrow what the tunnel reaches and can never open anything
// the host's own firewall closes, nor touch traffic that does not use the
// tunnel. Chains accept by default for the same reason: a hub or spoke run in
// a host's network namespace must not take the host's other traffic with it.
package nft

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/CorneiZeR/awg-gate/internal/gate"
)

// Table is the one table awg-gate owns.
const Table = "inet awg_gate"

// Hub renders the hub's rules: people reach the servers their rules name, on
// the ports the server allows; nothing else crosses the hub, and the hub
// itself answers only pings over the tunnel.
func Hub(iface string, rules []gate.Rule) string {
	var b strings.Builder
	q := quote(iface)
	fmt.Fprintf(&b, "table %s {\n", Table)
	fmt.Fprintf(&b, "\tchain input {\n\t\ttype filter hook input priority filter; policy accept;\n")
	fmt.Fprintf(&b, "\t\tiifname %s ct state established,related accept\n", q)
	fmt.Fprintf(&b, "\t\tiifname %s icmp type echo-request accept\n", q)
	fmt.Fprintf(&b, "\t\tiifname %s counter drop comment \"nothing on the hub itself\"\n", q)
	fmt.Fprintf(&b, "\t}\n")
	fmt.Fprintf(&b, "\tchain forward {\n\t\ttype filter hook forward priority filter; policy accept;\n")
	fmt.Fprintf(&b, "\t\tiifname %s ct state established,related accept\n", q)
	fmt.Fprintf(&b, "\t\tiifname %s ct state invalid drop\n", q)
	for _, r := range rules {
		match := fmt.Sprintf("iifname %s oifname %s ip saddr %s ip daddr %s", q, q, r.From, r.To)
		comment := quote(r.FromName + " -> " + r.ToName)
		if len(r.Ports) == 0 {
			fmt.Fprintf(&b, "\t\t%s accept comment %s\n", match, comment)
			continue
		}
		fmt.Fprintf(&b, "\t\t%s icmp type echo-request accept comment %s\n", match, comment)
		fmt.Fprintf(&b, "\t\t%s meta l4proto { tcp, udp } th dport { %s } accept comment %s\n", match, ports(r.Ports), comment)
	}
	fmt.Fprintf(&b, "\t\tiifname %s counter drop comment \"no rule lets this through\"\n", q)
	fmt.Fprintf(&b, "\t\toifname %s counter drop comment \"nothing else enters the network\"\n", q)
	fmt.Fprintf(&b, "\t}\n}\n")
	return b.String()
}

// SpokeOptions narrow what arrives over a spoke's tunnel.
type SpokeOptions struct {
	// AllowPorts, when set, are the only TCP/UDP ports reachable over the
	// tunnel, whether a process on the host or a published container port
	// answers them.
	AllowPorts []int
	// Lockdown drops every new inbound connection that does not come over the
	// tunnel. Meant for a sidecar's own network namespace, never a host's.
	Lockdown bool
}

// Spoke renders a spoke's rules; empty when the options ask for nothing.
func Spoke(iface string, o SpokeOptions) string {
	if len(o.AllowPorts) == 0 && !o.Lockdown {
		return ""
	}
	var b strings.Builder
	q := quote(iface)
	fmt.Fprintf(&b, "table %s {\n", Table)
	fmt.Fprintf(&b, "\tchain input {\n\t\ttype filter hook input priority filter; policy accept;\n")
	if len(o.AllowPorts) > 0 {
		fmt.Fprintf(&b, "\t\tiifname %s ct state established,related accept\n", q)
		fmt.Fprintf(&b, "\t\tiifname %s icmp type echo-request accept\n", q)
		fmt.Fprintf(&b, "\t\tiifname %s meta l4proto { tcp, udp } th dport { %s } accept\n", q, ports(o.AllowPorts))
		fmt.Fprintf(&b, "\t\tiifname %s counter drop comment \"port not allowed over the tunnel\"\n", q)
	}
	if o.Lockdown {
		fmt.Fprintf(&b, "\t\tiifname != { \"lo\", %s } ct state established,related accept\n", q)
		fmt.Fprintf(&b, "\t\tiifname != { \"lo\", %s } counter drop comment \"lockdown: only the tunnel comes in\"\n", q)
	}
	fmt.Fprintf(&b, "\t}\n")
	if len(o.AllowPorts) > 0 {
		// A published container port is DNATed and forwarded, not delivered to
		// input; the port it was addressed to is the conntrack original.
		fmt.Fprintf(&b, "\tchain forward {\n\t\ttype filter hook forward priority filter; policy accept;\n")
		fmt.Fprintf(&b, "\t\tiifname %s ct state established,related accept\n", q)
		fmt.Fprintf(&b, "\t\tiifname %s meta l4proto { tcp, udp } ct original proto-dst { %s } accept\n", q, ports(o.AllowPorts))
		fmt.Fprintf(&b, "\t\tiifname %s counter drop comment \"port not allowed over the tunnel\"\n", q)
		fmt.Fprintf(&b, "\t}\n")
	}
	fmt.Fprintf(&b, "}\n")
	return b.String()
}

// Script wraps a ruleset so `nft -f` replaces the table atomically: the
// first line makes sure there is a table to delete, the second deletes it,
// and the whole file is one transaction. An empty ruleset only removes it.
func Script(ruleset string) string {
	return fmt.Sprintf("table %s\ndelete table %s\n%s", Table, Table, ruleset)
}

// Apply loads a ruleset, replacing the table.
func Apply(ctx context.Context, ruleset string) error {
	cmd := exec.CommandContext(ctx, "nft", "-f", "-")
	cmd.Stdin = strings.NewReader(Script(ruleset))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nft: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Remove deletes the table, if present.
func Remove(ctx context.Context) error {
	return Apply(ctx, "")
}

func ports(ps []int) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = strconv.Itoa(p)
	}
	return strings.Join(s, ", ")
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, ``, `"`, ``).Replace(s) + `"`
}
