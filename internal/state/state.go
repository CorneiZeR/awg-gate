// Package state is what a hub keeps in its data volume: its private key and
// its obfuscation parameters, both drawn once on the first start. Losing them
// strands every issued configuration, so the volume is the one thing a hub
// needs backed up.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CorneiZeR/awg-gate/internal/fsutil"
	"github.com/CorneiZeR/awg-gate/internal/gate"
	"github.com/CorneiZeR/awg-gate/internal/keys"
	"github.com/CorneiZeR/awg-gate/internal/obfs"
)

const (
	keyFile    = "hub.key"
	paramsFile = "params.json"
)

// Hub is the persisted identity of a hub.
type Hub struct {
	PrivateKey keys.Key
	Params     obfs.Params
}

// Load reads the state in dir; it does not create anything.
func Load(dir string) (*Hub, error) {
	raw, err := fsutil.ReadFile(filepath.Join(dir, keyFile))
	if err != nil {
		return nil, fmt.Errorf("read hub key: %w", err)
	}
	k, err := keys.Parse(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, keyFile), err)
	}
	raw, err = fsutil.ReadFile(filepath.Join(dir, paramsFile))
	if err != nil {
		return nil, fmt.Errorf("read obfuscation parameters: %w", err)
	}
	var p obfs.Params
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, paramsFile), err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, paramsFile), err)
	}
	return &Hub{PrivateKey: k, Params: p}, nil
}

// Init loads the state in dir, creating whatever is missing. A profile that
// differs from the stored one is an error, never a silent redraw.
func Init(dir, profile string) (h *Hub, created bool, err error) {
	keyPath, paramsPath := filepath.Join(dir, keyFile), filepath.Join(dir, paramsFile)
	_, keyErr := os.Stat(keyPath)
	_, paramsErr := os.Stat(paramsPath)
	switch {
	case keyErr == nil && paramsErr == nil:
		h, err = Load(dir)
		if err != nil {
			return nil, false, err
		}
		if h.Params.Profile != profile {
			return nil, false, fmt.Errorf(
				"gate.yaml asks for profile %s but this hub was set up with %s; changing it means a new "+
					"%s and reissuing every configuration — delete it deliberately if that is the intent",
				profile, h.Params.Profile, paramsPath)
		}
		return h, false, nil
	case !errors.Is(keyErr, fs.ErrNotExist) && keyErr != nil:
		return nil, false, keyErr
	case !errors.Is(paramsErr, fs.ErrNotExist) && paramsErr != nil:
		return nil, false, paramsErr
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, err
	}
	h = &Hub{}
	if keyErr == nil {
		if h, err = loadKeyOnly(keyPath); err != nil {
			return nil, false, err
		}
	} else {
		if h.PrivateKey, err = keys.Generate(nil); err != nil {
			return nil, false, err
		}
		if err := fsutil.WriteAtomic(keyPath, []byte(h.PrivateKey.String()+"\n"), 0o600); err != nil {
			return nil, false, err
		}
	}
	if paramsErr == nil {
		loaded, err := Load(dir)
		if err != nil {
			return nil, false, err
		}
		h.Params = loaded.Params
	} else {
		if h.Params, err = obfs.Generate(profile, nil); err != nil {
			return nil, false, err
		}
		raw, _ := json.MarshalIndent(h.Params, "", "  ")
		if err := fsutil.WriteAtomic(paramsPath, append(raw, '\n'), 0o600); err != nil {
			return nil, false, err
		}
	}
	return h, true, nil
}

func loadKeyOnly(path string) (*Hub, error) {
	raw, err := fsutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	k, err := keys.Parse(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &Hub{PrivateKey: k}, nil
}

// Info is everything a client configuration needs from a hub, and nothing that
// lets anyone in: no private key. It is still kept out of public places — the
// obfuscation parameters are what make the hub's traffic hard to recognise.
type Info struct {
	Version       int         `json:"version"`
	Endpoint      string      `json:"endpoint"`
	PublicKey     string      `json:"public_key"`
	MTU           int         `json:"mtu"`
	Keepalive     int         `json:"keepalive"`
	Network       string      `json:"network"`
	ServersSubnet string      `json:"servers_subnet"`
	PeopleSubnet  string      `json:"people_subnet"`
	HubAddress    string      `json:"hub_address"`
	Params        obfs.Params `json:"params"`
}

// InfoVersion is the format of Info this build writes and reads.
const InfoVersion = 1

// NewInfo describes a hub.
func NewInfo(c *gate.Config, h *Hub) Info {
	return Info{
		Version:       InfoVersion,
		Endpoint:      c.Endpoint,
		PublicKey:     h.PrivateKey.Public().String(),
		MTU:           c.MTU,
		Keepalive:     c.Keepalive,
		Network:       c.NetworkPrefix().String(),
		ServersSubnet: c.ServersPrefix().String(),
		PeopleSubnet:  c.PeoplePrefix().String(),
		HubAddress:    c.HubAddr().String(),
		Params:        h.Params,
	}
}

// ReadInfo loads a hub-info file and checks it.
func ReadInfo(path string) (Info, error) {
	raw, err := fsutil.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	var i Info
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&i); err != nil {
		return Info{}, fmt.Errorf("%s: %w", path, err)
	}
	if i.Version != InfoVersion {
		return Info{}, fmt.Errorf("%s: format version %d, this build reads %d", path, i.Version, InfoVersion)
	}
	if _, err := keys.Parse(i.PublicKey); err != nil {
		return Info{}, fmt.Errorf("%s: public_key: %w", path, err)
	}
	if err := i.Params.Validate(); err != nil {
		return Info{}, fmt.Errorf("%s: %w", path, err)
	}
	return i, nil
}
