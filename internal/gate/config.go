// Package gate is the hub's desired state: gate.yaml, its defaults and its
// validation, and the plan of peers and access rules derived from it.
package gate

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/CorneiZeR/awg-gate/internal/fsutil"
	"github.com/CorneiZeR/awg-gate/internal/keys"
	"github.com/CorneiZeR/awg-gate/internal/obfs"
)

// Defaults a gate.yaml may leave out.
const (
	DefaultListenPort = 443
	DefaultInterface  = "awg0"
	DefaultMTU        = 1280
	DefaultKeepalive  = 25
	DateLayout        = "2006-01-02"
	// ReachAll in a person's reach list grants every server.
	ReachAll = "*"
)

// Default address plan: 10.247.0.0/16 stays clear of Docker (172.16/12),
// Tailscale (100.64/10), OpenVPN's habitual 10.8/24 and most home networks.
var (
	DefaultNetwork       = netip.MustParsePrefix("10.247.0.0/16")
	DefaultServersSubnet = netip.MustParsePrefix("10.247.0.0/24")
	DefaultPeopleSubnet  = netip.MustParsePrefix("10.247.1.0/24")
)

var (
	nameRE  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)
	ifaceRE = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,15}$`)
)

// Config is gate.yaml.
type Config struct {
	// Endpoint is host:port as clients dial it: the hub's public name or address
	// and the port published to the internet.
	Endpoint string `yaml:"endpoint"`
	// ListenPort is the UDP port inside the container.
	ListenPort int    `yaml:"listen_port,omitempty"`
	Interface  string `yaml:"interface,omitempty"`
	// Profile picks the obfuscation profile; see package obfs.
	Profile       string `yaml:"profile,omitempty"`
	MTU           int    `yaml:"mtu,omitempty"`
	Keepalive     int    `yaml:"keepalive,omitempty"`
	Network       string `yaml:"network,omitempty"`
	ServersSubnet string `yaml:"servers_subnet,omitempty"`
	PeopleSubnet  string `yaml:"people_subnet,omitempty"`
	HubAddress    string `yaml:"hub_address,omitempty"`

	Servers map[string]Server `yaml:"servers"`
	People  map[string]Person `yaml:"people"`

	// Parsed forms, filled by Validate.
	network, serversSubnet, peopleSubnet netip.Prefix
	hubAddress                           netip.Addr
}

// Server is a machine people reach through the hub.
type Server struct {
	Address   string `yaml:"address"`
	PublicKey string `yaml:"public_key"`
	// Ports, when set, are the only TCP/UDP ports people may reach on it.
	Ports []int `yaml:"ports,omitempty"`
}

// Person is somebody with one or more devices.
type Person struct {
	// Reach names the servers this person may reach, or "*" for all of them;
	// empty reaches nothing (the devices connect but every server is closed).
	Reach []string `yaml:"reach"`
	// Expires is the last day (UTC) access lasts; empty for no end.
	Expires string `yaml:"expires,omitempty"`
	// Disabled keeps the person in the file, keys and addresses reserved, but
	// off the hub; clearing it lets the same configurations connect again.
	Disabled bool              `yaml:"disabled,omitempty"`
	Devices  map[string]Device `yaml:"devices"`
}

// Device is one key of a person: a laptop, a phone.
type Device struct {
	Address   string `yaml:"address"`
	PublicKey string `yaml:"public_key"`
	// Disabled takes this one device off the hub, as Person.Disabled does all of them.
	Disabled bool `yaml:"disabled,omitempty"`
}

// Load reads and validates a gate.yaml.
func Load(path string) (*Config, error) {
	raw, err := fsutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

// Parse decodes and validates gate.yaml content. Unknown keys are errors:
// a misspelt "expire:" must not silently grant access forever.
func Parse(raw []byte) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("gate.yaml: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.ListenPort == 0 {
		c.ListenPort = DefaultListenPort
	}
	if c.Interface == "" {
		c.Interface = DefaultInterface
	}
	if c.Profile == "" {
		c.Profile = obfs.ProfileV1
	}
	if c.MTU == 0 {
		c.MTU = DefaultMTU
	}
	if c.Keepalive == 0 {
		c.Keepalive = DefaultKeepalive
	}
	if c.Network == "" {
		c.Network = DefaultNetwork.String()
	}
	if c.ServersSubnet == "" {
		c.ServersSubnet = DefaultServersSubnet.String()
	}
	if c.PeopleSubnet == "" {
		c.PeopleSubnet = DefaultPeopleSubnet.String()
	}
}

// Validate fills defaults and checks every rule; it reports all problems at once.
func (c *Config) Validate() error {
	c.applyDefaults()
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if host, port, err := net.SplitHostPort(c.Endpoint); err != nil || host == "" {
		add("endpoint %q: want host:port", c.Endpoint)
	} else if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		add("endpoint %q: port must be 1..65535", c.Endpoint)
	}
	if c.ListenPort < 1 || c.ListenPort > 65535 {
		add("listen_port %d: must be 1..65535", c.ListenPort)
	}
	if !ifaceRE.MatchString(c.Interface) {
		add("interface %q: 1-15 letters, digits, '_', '.' or '-'", c.Interface)
	}
	if c.Profile != obfs.ProfileV1 && c.Profile != obfs.ProfileV3 {
		add("profile %q: want v1 or v3", c.Profile)
	}
	if c.MTU < 1280 || c.MTU > 1420 {
		add("mtu %d: must be 1280..1420", c.MTU)
	}
	if c.Keepalive < 0 || c.Keepalive > 600 {
		add("keepalive %d: must be 0..600 seconds", c.Keepalive)
	}

	var err error
	if c.network, err = parseIPv4Prefix(c.Network); err != nil {
		add("network: %v", err)
	}
	if c.serversSubnet, err = parseIPv4Prefix(c.ServersSubnet); err != nil {
		add("servers_subnet: %v", err)
	}
	if c.peopleSubnet, err = parseIPv4Prefix(c.PeopleSubnet); err != nil {
		add("people_subnet: %v", err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	for name, sub := range map[string]netip.Prefix{"servers_subnet": c.serversSubnet, "people_subnet": c.peopleSubnet} {
		if !within(c.network, sub) {
			add("%s %s lies outside network %s", name, sub, c.network)
		}
	}
	if c.serversSubnet.Overlaps(c.peopleSubnet) {
		add("servers_subnet %s and people_subnet %s overlap", c.serversSubnet, c.peopleSubnet)
	}

	if c.HubAddress == "" {
		c.hubAddress = c.serversSubnet.Addr().Next()
		c.HubAddress = c.hubAddress.String()
	} else if c.hubAddress, err = netip.ParseAddr(c.HubAddress); err != nil || !c.hubAddress.Is4() {
		add("hub_address %q: not an IPv4 address", c.HubAddress)
	}
	if c.hubAddress.IsValid() && !usable(c.serversSubnet, c.hubAddress) {
		add("hub_address %s: must be a host address inside servers_subnet %s", c.hubAddress, c.serversSubnet)
	}

	usedAddr := map[netip.Addr]string{c.hubAddress: "the hub"}
	usedKey := map[keys.Key]string{}
	claim := func(who, addrS, keyS string, subnet netip.Prefix, subnetName string) {
		addr, err := netip.ParseAddr(addrS)
		switch {
		case err != nil || !addr.Is4():
			add("%s: address %q is not an IPv4 address", who, addrS)
		case !usable(subnet, addr):
			add("%s: address %s must be a host address inside %s %s", who, addr, subnetName, subnet)
		case usedAddr[addr] != "":
			add("%s: address %s is already %s's", who, addr, usedAddr[addr])
		default:
			usedAddr[addr] = who
		}
		key, err := keys.Parse(keyS)
		switch {
		case err != nil:
			add("%s: public_key: %v", who, err)
		case usedKey[key] != "":
			add("%s: public_key is already %s's", who, usedKey[key])
		default:
			usedKey[key] = who
		}
	}

	for _, name := range sortedKeys(c.Servers) {
		s := c.Servers[name]
		who := "server " + name
		if !nameRE.MatchString(name) {
			add("%s: names are 1-32 lowercase letters, digits and inner '-'", who)
		}
		claim(who, s.Address, s.PublicKey, c.serversSubnet, "servers_subnet")
		for _, p := range s.Ports {
			if p < 1 || p > 65535 {
				add("%s: port %d must be 1..65535", who, p)
			}
		}
	}
	for _, name := range sortedKeys(c.People) {
		p := c.People[name]
		who := "person " + name
		if !nameRE.MatchString(name) {
			add("%s: names are 1-32 lowercase letters, digits and inner '-'", who)
		}
		for _, r := range p.Reach {
			if r == ReachAll {
				if len(p.Reach) > 1 {
					add("%s: %q stands alone in reach", who, ReachAll)
				}
				continue
			}
			if _, ok := c.Servers[r]; !ok {
				add("%s: reach names %q, which is not a server", who, r)
			}
		}
		if p.Expires != "" {
			if _, err := time.Parse(DateLayout, p.Expires); err != nil {
				add("%s: expires %q is not a YYYY-MM-DD date", who, p.Expires)
			}
		}
		if len(p.Devices) == 0 {
			add("%s: has no devices", who)
		}
		for _, dn := range sortedKeys(p.Devices) {
			d := p.Devices[dn]
			dwho := fmt.Sprintf("person %s, device %s", name, dn)
			if !nameRE.MatchString(dn) {
				add("%s: names are 1-32 lowercase letters, digits and inner '-'", dwho)
			}
			claim(dwho, d.Address, d.PublicKey, c.peopleSubnet, "people_subnet")
		}
	}
	return errors.Join(errs...)
}

// NetworkPrefix is the whole access network.
func (c *Config) NetworkPrefix() netip.Prefix { return c.network }

// ServersPrefix is the subnet servers live in.
func (c *Config) ServersPrefix() netip.Prefix { return c.serversSubnet }

// PeoplePrefix is the subnet people's devices live in.
func (c *Config) PeoplePrefix() netip.Prefix { return c.peopleSubnet }

// HubAddr is the hub's own address on the network.
func (c *Config) HubAddr() netip.Addr { return c.hubAddress }

// NextFree returns the first free host address in subnet.
func (c *Config) NextFree(subnet netip.Prefix) (netip.Addr, error) {
	used := map[netip.Addr]bool{c.hubAddress: true}
	for _, s := range c.Servers {
		if a, err := netip.ParseAddr(s.Address); err == nil {
			used[a] = true
		}
	}
	for _, p := range c.People {
		for _, d := range p.Devices {
			if a, err := netip.ParseAddr(d.Address); err == nil {
				used[a] = true
			}
		}
	}
	for a := subnet.Masked().Addr().Next(); subnet.Contains(a); a = a.Next() {
		if usable(subnet, a) && !used[a] {
			return a, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("no free address left in %s", subnet)
}

func parseIPv4Prefix(s string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil || !p.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("%q is not an IPv4 prefix", s)
	}
	if p != p.Masked() {
		return netip.Prefix{}, fmt.Errorf("%q has host bits set; did you mean %s?", s, p.Masked())
	}
	if p.Bits() > 30 {
		return netip.Prefix{}, fmt.Errorf("%q is too small, use /30 or larger", s)
	}
	return p, nil
}

func within(outer, inner netip.Prefix) bool {
	return outer.Bits() <= inner.Bits() && outer.Contains(inner.Addr())
}

// usable: inside the subnet, neither its network nor its broadcast address.
func usable(subnet netip.Prefix, a netip.Addr) bool {
	if !subnet.Contains(a) || a == subnet.Masked().Addr() {
		return false
	}
	return subnet.Contains(a.Next())
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
