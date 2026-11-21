// Package obfs draws and checks the AmneziaWG obfuscation parameters a hub
// and every one of its peers must share.
//
// Two profiles exist:
//
//   - v1: junk packets, handshake paddings S1/S2 and four fixed message
//     headers H1-H4. Every AmneziaWG and AmneziaVPN client accepts it.
//   - v3: v1 plus cookie and transport paddings S3/S4, header ranges instead
//     of fixed values, and header protection. It needs AmneziaWG 3 on both
//     ends (AmneziaVPN 5.x, AmneziaWG for Android 3.x and later).
//
// The values are drawn once, when a hub is first started, and kept: changing
// any of them strands every issued client configuration.
package obfs

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"

	"github.com/CorneiZeR/awg-gate/internal/keys"
)

// Profiles a hub may be started with.
const (
	ProfileV1 = "v1"
	ProfileV3 = "v3"
)

// Bounds every profile draws from. The handshake initiation is 148 bytes and
// the response 92, so S1+56 == S2 would make both the same size and the
// packet types indistinguishable to the receiver.
const (
	jcMin, jcMax       = 4, 8
	junkMin, junkMax   = 40, 70
	padMin, padMax     = 15, 150
	initRespSizeDelta  = 148 - 92
	headerLow          = 5
	headerHigh         = 1<<31 - 1
	v3CookiePadMin     = 15
	v3CookiePadMax     = 64
	v3TransportPadMin  = 12
	v3TransportPadMax  = 32
	v3HeaderRangeWidth = 1 << 24
	headerProtectMinS  = 12
	maxJunkSize        = 1280
)

// Params are the obfuscation values of one hub.
type Params struct {
	Profile             string `json:"profile"`
	Jc                  int    `json:"jc"`
	Jmin                int    `json:"jmin"`
	Jmax                int    `json:"jmax"`
	S1                  int    `json:"s1"`
	S2                  int    `json:"s2"`
	S3                  int    `json:"s3,omitempty"`
	S4                  int    `json:"s4,omitempty"`
	H1                  string `json:"h1"`
	H2                  string `json:"h2"`
	H3                  string `json:"h3"`
	H4                  string `json:"h4"`
	HeaderProtectionKey string `json:"header_protection_key,omitempty"`
}

// ErrProfile is returned for a profile name this version does not know.
var ErrProfile = errors.New(`unknown obfuscation profile (want "v1" or "v3")`)

// Generate draws a fresh parameter set for profile from r (crypto/rand when nil).
func Generate(profile string, r io.Reader) (Params, error) {
	if r == nil {
		r = rand.Reader
	}
	d := drawer{r: r}
	p := Params{Profile: profile, Jmin: junkMin, Jmax: junkMax}
	p.Jc = d.between(jcMin, jcMax)
	p.S1 = d.between(padMin, padMax)
	for {
		p.S2 = d.between(padMin, padMax)
		if p.S1+initRespSizeDelta != p.S2 {
			break
		}
	}
	switch profile {
	case ProfileV1:
		seen := map[int64]bool{}
		hs := make([]string, 4)
		for i := range hs {
			for {
				v := d.between64(headerLow, headerHigh)
				if !seen[v] {
					seen[v] = true
					hs[i] = strconv.FormatInt(v, 10)
					break
				}
			}
		}
		p.H1, p.H2, p.H3, p.H4 = hs[0], hs[1], hs[2], hs[3]
	case ProfileV3:
		p.S3 = d.between(v3CookiePadMin, v3CookiePadMax)
		p.S4 = d.between(v3TransportPadMin, v3TransportPadMax)
		// One range per quarter of the header space, quarters dealt in random order,
		// so the four ranges can never overlap.
		quarter := int64(headerHigh-headerLow+1) / 4
		order := d.perm(4)
		hs := make([]string, 4)
		for i, q := range order {
			lo := headerLow + int64(q)*quarter
			start := d.between64(lo, lo+quarter-v3HeaderRangeWidth)
			hs[i] = fmt.Sprintf("%d-%d", start, start+v3HeaderRangeWidth-1)
		}
		p.H1, p.H2, p.H3, p.H4 = hs[0], hs[1], hs[2], hs[3]
		key, err := keys.Generate(r)
		if err != nil {
			return Params{}, err
		}
		p.HeaderProtectionKey = key.String()
	default:
		return Params{}, ErrProfile
	}
	if d.err != nil {
		return Params{}, fmt.Errorf("draw obfuscation parameters: %w", d.err)
	}
	return p, p.Validate()
}

// Validate checks the parameters against the rules AmneziaWG clients enforce.
func (p Params) Validate() error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if p.Profile != ProfileV1 && p.Profile != ProfileV3 {
		errs = append(errs, ErrProfile)
	}
	if p.Jc < 0 || p.Jc > 128 {
		add("jc %d outside 0..128", p.Jc)
	}
	if p.Jmin < 0 || p.Jmin > p.Jmax || p.Jmax > maxJunkSize {
		add("junk size %d..%d must satisfy 0 <= jmin <= jmax <= %d", p.Jmin, p.Jmax, maxJunkSize)
	}
	for name, s := range map[string]int{"s1": p.S1, "s2": p.S2, "s3": p.S3, "s4": p.S4} {
		if s < 0 || s > 1000 {
			add("%s %d outside 0..1000", name, s)
		}
	}
	if p.S1+initRespSizeDelta == p.S2 {
		add("s1+56 must differ from s2 (%d+56 == %d)", p.S1, p.S2)
	}
	ranges := make([][2]int64, 0, 4)
	for i, h := range []string{p.H1, p.H2, p.H3, p.H4} {
		lo, hi, err := parseRange(h)
		if err != nil {
			add("h%d: %v", i+1, err)
			continue
		}
		if p.Profile == ProfileV1 && lo != hi {
			add("h%d: profile v1 takes a single value, not the range %q", i+1, h)
		}
		ranges = append(ranges, [2]int64{lo, hi})
	}
	for i := range ranges {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[i][0] <= ranges[j][1] && ranges[j][0] <= ranges[i][1] {
				add("message headers must not overlap (%s and %s)", rangeString(ranges[i]), rangeString(ranges[j]))
			}
		}
	}
	if p.Profile == ProfileV3 {
		if _, err := keys.Parse(p.HeaderProtectionKey); err != nil {
			add("header_protection_key: %v", err)
		}
		for name, s := range map[string]int{"s1": p.S1, "s2": p.S2, "s3": p.S3, "s4": p.S4} {
			if s < headerProtectMinS {
				add("%s %d: header protection needs every padding to be at least %d", name, s, headerProtectMinS)
			}
		}
	}
	if p.Profile == ProfileV1 && (p.S3 != 0 || p.S4 != 0 || p.HeaderProtectionKey != "") {
		add("profile v1 carries no s3, s4 or header protection key")
	}
	return errors.Join(errs...)
}

// Lines returns the parameters as configuration-file keys, in a fixed order.
func (p Params) Lines() [][2]string {
	lines := [][2]string{
		{"Jc", strconv.Itoa(p.Jc)},
		{"Jmin", strconv.Itoa(p.Jmin)},
		{"Jmax", strconv.Itoa(p.Jmax)},
		{"S1", strconv.Itoa(p.S1)},
		{"S2", strconv.Itoa(p.S2)},
	}
	if p.Profile == ProfileV3 {
		lines = append(lines, [2]string{"S3", strconv.Itoa(p.S3)}, [2]string{"S4", strconv.Itoa(p.S4)})
	}
	lines = append(lines,
		[2]string{"H1", p.H1},
		[2]string{"H2", p.H2},
		[2]string{"H3", p.H3},
		[2]string{"H4", p.H4},
	)
	if p.Profile == ProfileV3 {
		lines = append(lines, [2]string{"HeaderProtectionKey", p.HeaderProtectionKey})
	}
	return lines
}

func parseRange(s string) (int64, int64, error) {
	loS, hiS, isRange := strings.Cut(s, "-")
	lo, err := strconv.ParseInt(strings.TrimSpace(loS), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("%q is not a number or a range", s)
	}
	hi := lo
	if isRange {
		if hi, err = strconv.ParseInt(strings.TrimSpace(hiS), 10, 64); err != nil {
			return 0, 0, fmt.Errorf("%q is not a number or a range", s)
		}
	}
	if lo < headerLow || hi > headerHigh || lo > hi {
		return 0, 0, fmt.Errorf("%q must lie within %d..%d, low end first", s, headerLow, headerHigh)
	}
	return lo, hi, nil
}

func rangeString(r [2]int64) string {
	if r[0] == r[1] {
		return strconv.FormatInt(r[0], 10)
	}
	return fmt.Sprintf("%d-%d", r[0], r[1])
}

// drawer turns a byte stream into uniform integers and remembers the first error.
type drawer struct {
	r   io.Reader
	err error
}

func (d *drawer) between64(lo, hi int64) int64 {
	if d.err != nil {
		return lo
	}
	n, err := rand.Int(d.r, big.NewInt(hi-lo+1))
	if err != nil {
		d.err = err
		return lo
	}
	return lo + n.Int64()
}

func (d *drawer) between(lo, hi int) int {
	return int(d.between64(int64(lo), int64(hi)))
}

func (d *drawer) perm(n int) []int {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	for i := n - 1; i > 0; i-- {
		j := d.between(0, i)
		p[i], p[j] = p[j], p[i]
	}
	return p
}
