package obfs

import (
	"strings"
	"testing"
)

func TestGenerateIsValid(t *testing.T) {
	for _, profile := range []string{ProfileV1, ProfileV3} {
		for range 200 {
			p, err := Generate(profile, nil)
			if err != nil {
				t.Fatalf("%s: %v", profile, err)
			}
			if p.Jc < jcMin || p.Jc > jcMax || p.S1 < padMin || p.S2 > padMax {
				t.Fatalf("%s: value outside the drawn bounds: %+v", profile, p)
			}
		}
	}
}

func TestGenerateUnknownProfile(t *testing.T) {
	if _, err := Generate("v2", nil); err == nil {
		t.Fatal("expected an error for an unknown profile")
	}
}

func valid(profile string) Params {
	p := Params{Profile: profile, Jc: 5, Jmin: 40, Jmax: 70, S1: 20, S2: 30, H1: "100", H2: "200", H3: "300", H4: "400"}
	if profile == ProfileV3 {
		p.S3, p.S4 = 20, 16
		p.H1, p.H2, p.H3, p.H4 = "100-199", "200-299", "300-399", "400-499"
		p.HeaderProtectionKey = "YAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk="
	}
	return p
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]struct {
		profile string
		mutate  func(*Params)
		want    string
	}{
		"s1+56 == s2":             {ProfileV1, func(p *Params) { p.S2 = p.S1 + 56 }, "s1+56"},
		"duplicate header":        {ProfileV1, func(p *Params) { p.H2 = p.H1 }, "overlap"},
		"range in v1":             {ProfileV1, func(p *Params) { p.H1 = "100-150" }, "single value"},
		"header below 5":          {ProfileV1, func(p *Params) { p.H1 = "4" }, "within"},
		"overlapping v3 ranges":   {ProfileV3, func(p *Params) { p.H2 = "150-250" }, "overlap"},
		"v3 small padding":        {ProfileV3, func(p *Params) { p.S4 = 8 }, "at least 12"},
		"v3 without key":          {ProfileV3, func(p *Params) { p.HeaderProtectionKey = "" }, "header_protection_key"},
		"v1 with v3 extras":       {ProfileV1, func(p *Params) { p.S3 = 20 }, "carries no"},
		"jmin above jmax":         {ProfileV1, func(p *Params) { p.Jmin = 80 }, "junk size"},
		"junk larger than an MTU": {ProfileV1, func(p *Params) { p.Jmax = 1500 }, "junk size"},
		"unknown profile":         {"v2", func(*Params) {}, "unknown"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := valid(tc.profile)
			if tc.profile == "v2" {
				p = valid(ProfileV1)
				p.Profile = "v2"
			}
			tc.mutate(&p)
			err := p.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want an error mentioning %q", err, tc.want)
			}
		})
	}
}

func TestValidateAccepts(t *testing.T) {
	for _, profile := range []string{ProfileV1, ProfileV3} {
		if err := valid(profile).Validate(); err != nil {
			t.Errorf("%s: %v", profile, err)
		}
	}
}

func TestLinesOrder(t *testing.T) {
	keysOf := func(p Params) string {
		var k []string
		for _, l := range p.Lines() {
			k = append(k, l[0])
		}
		return strings.Join(k, ",")
	}
	if got := keysOf(valid(ProfileV1)); got != "Jc,Jmin,Jmax,S1,S2,H1,H2,H3,H4" {
		t.Errorf("v1 lines = %s", got)
	}
	if got := keysOf(valid(ProfileV3)); got != "Jc,Jmin,Jmax,S1,S2,S3,S4,H1,H2,H3,H4,HeaderProtectionKey" {
		t.Errorf("v3 lines = %s", got)
	}
}
