// Package issue renders the configurations awg-gate hands out: a person's
// client configuration and a server's spoke configuration.
//
// Neither carries a DNS line. The tunnel only routes the access network; the
// names people type keep resolving through their usual DNS, to private
// addresses (see the wiki's Cloudflare page).
package issue

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"rsc.io/qr"

	"github.com/CorneiZeR/awg-gate/internal/conf"
	"github.com/CorneiZeR/awg-gate/internal/keys"
	"github.com/CorneiZeR/awg-gate/internal/state"
)

// Person renders a device's client configuration: it routes only the servers'
// subnet, so the rest of the device's traffic never touches the hub.
func Person(i state.Info, priv keys.Key, addr netip.Addr) conf.File {
	return client(i, priv, addr, i.ServersSubnet)
}

// Server renders a spoke's configuration: it routes only the people's subnet,
// so one server can never reach another through the hub.
func Server(i state.Info, priv keys.Key, addr netip.Addr) conf.File {
	return client(i, priv, addr, i.PeopleSubnet)
}

func client(i state.Info, priv keys.Key, addr netip.Addr, allowed string) conf.File {
	iface := conf.Section{
		{Key: "PrivateKey", Value: priv.String()},
		{Key: "Address", Value: addr.String() + "/32"},
		{Key: "MTU", Value: strconv.Itoa(i.MTU)},
	}
	for _, l := range i.Params.Lines() {
		iface = append(iface, conf.KV{Key: l[0], Value: l[1]})
	}
	peer := conf.Section{
		{Key: "PublicKey", Value: i.PublicKey},
		{Key: "Endpoint", Value: i.Endpoint},
		{Key: "AllowedIPs", Value: allowed},
	}
	if i.Keepalive > 0 {
		peer = append(peer, conf.KV{Key: "PersistentKeepalive", Value: strconv.Itoa(i.Keepalive)})
	}
	return conf.File{Interface: iface, Peers: []conf.Section{peer}}
}

// Header is the comment block on top of an issued file.
func Header(what string) string {
	return fmt.Sprintf("# awg-gate: %s\n# Split tunnel without DNS: only the access network goes through it.\n# Contains a private key - hand it to its owner only, never commit it.\n\n", what)
}

// QRPNG encodes a configuration as a QR code image the AmneziaVPN and
// AmneziaWG apps can scan.
func QRPNG(text string) ([]byte, error) {
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		return nil, fmt.Errorf("encode QR: %w", err)
	}
	code.Scale = 6
	return code.PNG(), nil
}

// QRTerminal draws a configuration as a QR code with half-block characters,
// two modules per character cell, with the quiet zone a scanner needs.
func QRTerminal(text string) (string, error) {
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		return "", fmt.Errorf("encode QR: %w", err)
	}
	const quiet = 2
	black := func(x, y int) bool {
		x, y = x-quiet, y-quiet
		return x >= 0 && y >= 0 && x < code.Size && y < code.Size && code.Black(x, y)
	}
	n := code.Size + 2*quiet
	var b strings.Builder
	for y := 0; y < n; y += 2 {
		for x := range n {
			top, bottom := black(x, y), black(x, y+1)
			// Light modules are drawn, dark ones left blank, so the code reads on a dark terminal.
			switch {
			case !top && !bottom:
				b.WriteString("█")
			case !top:
				b.WriteString("▀")
			case !bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
