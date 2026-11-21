// Package conf reads and writes the INI-like configuration files WireGuard,
// AmneziaWG and their clients exchange: one [Interface] section and any
// number of [Peer] sections.
//
// Keys are matched case-insensitively, as wg-quick matches them; order and
// spelling are kept, so a file written back is the file that was read.
package conf

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// KV is one key = value line.
type KV struct {
	Key   string
	Value string
}

// Section is the ordered lines of one section.
type Section []KV

// File is a parsed configuration.
type File struct {
	Interface Section
	Peers     []Section
}

// quickOnly are the [Interface] keys only wg-quick understands; `awg setconf`
// rejects them, so they are stripped before a file is handed to it.
var quickOnly = map[string]bool{
	"address": true, "dns": true, "mtu": true, "table": true, "saveconfig": true,
	"preup": true, "postup": true, "predown": true, "postdown": true,
}

// Parse reads a configuration file.
func Parse(r io.Reader) (File, error) {
	var (
		f       File
		current *Section
		lineNo  int
	)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		lineNo++
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			switch name := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1])); name {
			case "interface":
				if f.Interface != nil {
					return File{}, fmt.Errorf("line %d: a second [Interface] section", lineNo)
				}
				f.Interface = Section{}
				current = &f.Interface
			case "peer":
				f.Peers = append(f.Peers, Section{})
				current = &f.Peers[len(f.Peers)-1]
			default:
				return File{}, fmt.Errorf("line %d: unknown section [%s]", lineNo, name)
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return File{}, fmt.Errorf("line %d: expected key = value", lineNo)
		}
		if current == nil {
			return File{}, fmt.Errorf("line %d: %q outside any section", lineNo, strings.TrimSpace(key))
		}
		*current = append(*current, KV{Key: strings.TrimSpace(key), Value: strings.TrimSpace(value)})
	}
	if err := sc.Err(); err != nil {
		return File{}, err
	}
	if f.Interface == nil {
		return File{}, fmt.Errorf("no [Interface] section")
	}
	return f, nil
}

// Get returns the first value of key.
func (s Section) Get(key string) (string, bool) {
	for _, kv := range s {
		if strings.EqualFold(kv.Key, key) {
			return kv.Value, true
		}
	}
	return "", false
}

// List returns every comma-separated item of every line of key, so
// "AllowedIPs = a, b" and two AllowedIPs lines read the same.
func (s Section) List(key string) []string {
	var out []string
	for _, kv := range s {
		if !strings.EqualFold(kv.Key, key) {
			continue
		}
		for item := range strings.SplitSeq(kv.Value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				out = append(out, item)
			}
		}
	}
	return out
}

// ForSetconf returns a copy without the keys only wg-quick understands.
func (f File) ForSetconf() File {
	out := File{Peers: f.Peers}
	for _, kv := range f.Interface {
		if !quickOnly[strings.ToLower(kv.Key)] {
			out.Interface = append(out.Interface, kv)
		}
	}
	if out.Interface == nil {
		out.Interface = Section{}
	}
	return out
}

// Render writes the file back out.
func (f File) Render() string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	writeSection(&b, f.Interface)
	for _, p := range f.Peers {
		b.WriteString("\n[Peer]\n")
		writeSection(&b, p)
	}
	return b.String()
}

func writeSection(b *strings.Builder, s Section) {
	for _, kv := range s {
		fmt.Fprintf(b, "%s = %s\n", kv.Key, kv.Value)
	}
}
