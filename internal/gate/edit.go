package gate

import (
	"bytes"
	"errors"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Edits of gate.yaml done on its YAML tree, so comments and order survive and
// the operator's file stays theirs. Every edit re-validates the result.

// AddServer adds a server entry.
func AddServer(raw []byte, name string, s Server) ([]byte, error) {
	return edit(raw, func(root *yaml.Node) error {
		servers := ensureMap(root, "servers")
		if lookup(servers, name) != nil {
			return fmt.Errorf("server %q already exists", name)
		}
		return appendPair(servers, name, s)
	})
}

// AddDevice adds a device to a person, creating the person when new. reach
// and expires are only used for a new person; an existing person's are kept.
func AddDevice(raw []byte, person, device string, d Device, reach []string, expires string) ([]byte, error) {
	return edit(raw, func(root *yaml.Node) error {
		people := ensureMap(root, "people")
		p := lookup(people, person)
		if p == nil {
			if len(reach) == 0 {
				return fmt.Errorf("person %q is new: say which servers they reach", person)
			}
			return appendPair(people, person, Person{
				Reach: reach, Expires: expires, Devices: map[string]Device{device: d},
			})
		}
		devices := ensureMap(p, "devices")
		if lookup(devices, device) != nil {
			return fmt.Errorf("person %q already has a device %q", person, device)
		}
		return appendPair(devices, device, d)
	})
}

// RemoveServer removes a server and every person's reach entry naming it.
func RemoveServer(raw []byte, name string) ([]byte, error) {
	return edit(raw, func(root *yaml.Node) error {
		if !removeKey(lookup(root, "servers"), name) {
			return fmt.Errorf("no server %q", name)
		}
		people := lookup(root, "people")
		if people == nil || people.Kind != yaml.MappingNode {
			return nil
		}
		for i := 1; i < len(people.Content); i += 2 {
			reach := lookup(people.Content[i], "reach")
			if reach == nil || reach.Kind != yaml.SequenceNode {
				continue
			}
			kept := reach.Content[:0]
			for _, item := range reach.Content {
				if item.Value != name {
					kept = append(kept, item)
				}
			}
			reach.Content = kept
		}
		return nil
	})
}

// RemovePerson removes a person with every device, or only one device; a
// person left without devices is removed whole.
func RemovePerson(raw []byte, person, device string) ([]byte, error) {
	return edit(raw, func(root *yaml.Node) error {
		people := lookup(root, "people")
		p := lookup(people, person)
		if p == nil {
			return fmt.Errorf("no person %q", person)
		}
		if device == "" {
			removeKey(people, person)
			return nil
		}
		devices := lookup(p, "devices")
		if !removeKey(devices, device) {
			return fmt.Errorf("person %q has no device %q", person, device)
		}
		if len(devices.Content) == 0 {
			removeKey(people, person)
		}
		return nil
	})
}

// SetDisabled disables or re-enables a person, or only one of their devices
// when device is given. Enabling removes the key rather than writing false, so
// a file that never disabled anybody reads the same after a round trip.
func SetDisabled(raw []byte, person, device string, disabled bool) ([]byte, error) {
	return edit(raw, func(root *yaml.Node) error {
		target := lookup(lookup(root, "people"), person)
		if target == nil {
			return fmt.Errorf("no person %q", person)
		}
		if device != "" {
			if target = lookup(lookup(target, "devices"), device); target == nil {
				return fmt.Errorf("person %q has no device %q", person, device)
			}
		}
		if !disabled {
			removeKey(target, "disabled")
			return nil
		}
		if v := lookup(target, "disabled"); v != nil {
			v.Kind, v.Tag, v.Value, v.Style = yaml.ScalarNode, "!!bool", "true", 0
			return nil
		}
		target.Content = append(target.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "disabled"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		return nil
	})
}

func edit(raw []byte, change func(root *yaml.Node) error) ([]byte, error) {
	var doc yaml.Node
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("gate.yaml: %w", err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("gate.yaml: the top level must be a mapping")
	}
	if err := change(doc.Content[0]); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	if _, err := Parse(out.Bytes()); err != nil {
		return nil, fmt.Errorf("the edited gate.yaml would not be valid: %w", err)
	}
	return out.Bytes(), nil
}

func lookup(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// ensureMap returns the mapping under key, turning a missing or empty
// ("servers:" with nothing after it) value into an empty mapping.
func ensureMap(m *yaml.Node, key string) *yaml.Node {
	v := lookup(m, key)
	if v == nil {
		v = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
		return v
	}
	if v.Kind == yaml.ScalarNode && (v.Tag == "!!null" || v.Value == "") {
		v.Kind, v.Tag, v.Value, v.Style = yaml.MappingNode, "!!map", "", 0
	}
	return v
}

func appendPair(m *yaml.Node, key string, value any) error {
	var v yaml.Node
	if err := v.Encode(value); err != nil {
		return err
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &v)
	return nil
}

func removeKey(m *yaml.Node, key string) bool {
	if m == nil || m.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return true
		}
	}
	return false
}
