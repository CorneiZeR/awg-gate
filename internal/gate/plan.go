package gate

import (
	"net/netip"
	"slices"
	"time"

	"github.com/CorneiZeR/awg-gate/internal/keys"
)

// Peer kinds.
const (
	KindServer = "server"
	KindPerson = "person"
)

// Peer is one key the hub accepts.
type Peer struct {
	Kind      string
	Name      string // "shop-dev" for a server, "alice/laptop" for a device
	Person    string // set for devices
	PublicKey keys.Key
	Address   netip.Addr
	Expires   string
}

// Rule lets one device reach one server, on every port or only on Ports.
type Rule struct {
	From, To         netip.Addr
	FromName, ToName string
	Ports            []int
}

// Plan is what the hub runs right now.
type Plan struct {
	Peers []Peer
	Rules []Rule
	// Expired lists the devices left out because their person's access ended.
	Expired []string
}

// Active reports whether access ending on expires (a YYYY-MM-DD day, inclusive,
// UTC) still holds at now. An empty or unparsable date never expires; Validate
// rejects the unparsable kind before a plan is made.
func Active(expires string, now time.Time) bool {
	if expires == "" {
		return true
	}
	day, err := time.Parse(DateLayout, expires)
	if err != nil {
		return true
	}
	return now.UTC().Before(day.AddDate(0, 0, 1))
}

// Plan derives peers and rules from a validated config at a moment.
func (c *Config) Plan(now time.Time) Plan {
	var p Plan
	for _, name := range sortedKeys(c.Servers) {
		s := c.Servers[name]
		p.Peers = append(p.Peers, Peer{
			Kind: KindServer, Name: name,
			PublicKey: mustKey(s.PublicKey), Address: netip.MustParseAddr(s.Address),
		})
	}
	for _, person := range sortedKeys(c.People) {
		pp := c.People[person]
		targets := pp.Reach
		if slices.Contains(targets, ReachAll) {
			targets = sortedKeys(c.Servers)
		}
		for _, dn := range sortedKeys(pp.Devices) {
			d := pp.Devices[dn]
			name := person + "/" + dn
			if !Active(pp.Expires, now) {
				p.Expired = append(p.Expired, name)
				continue
			}
			addr := netip.MustParseAddr(d.Address)
			p.Peers = append(p.Peers, Peer{
				Kind: KindPerson, Name: name, Person: person,
				PublicKey: mustKey(d.PublicKey), Address: addr, Expires: pp.Expires,
			})
			for _, srv := range targets {
				s := c.Servers[srv]
				p.Rules = append(p.Rules, Rule{
					From: addr, To: netip.MustParseAddr(s.Address),
					FromName: name, ToName: srv,
					Ports: slices.Sorted(slices.Values(s.Ports)),
				})
			}
		}
	}
	return p
}

// NextExpiry returns the earliest moment after now at which some access ends,
// so a hub can re-plan exactly then; zero when nothing will expire.
func (c *Config) NextExpiry(now time.Time) time.Time {
	var next time.Time
	for _, p := range c.People {
		if p.Expires == "" {
			continue
		}
		day, err := time.Parse(DateLayout, p.Expires)
		if err != nil {
			continue
		}
		end := day.AddDate(0, 0, 1)
		if end.After(now) && (next.IsZero() || end.Before(next)) {
			next = end
		}
	}
	return next
}

func mustKey(s string) keys.Key {
	k, err := keys.Parse(s)
	if err != nil {
		panic("plan of an unvalidated config: " + err.Error())
	}
	return k
}
