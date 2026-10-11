package rules

import (
	"encoding/json"
	"fmt"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/config"
	"math/big"
	"net/netip"
	"regexp"
	"sort"
	"strings"
)

type Rule struct {
	Prefix    netip.Prefix `json:"prefix"`
	RateMbps  uint64       `json:"rate_mbps"`
	Lock      bool         `json:"lock"`
	NoRoute   bool         `json:"noroute"`
	Table     *uint32      `json:"table,omitempty"`
	Interface string       `json:"interface,omitempty"`
	Sources   []string     `json:"sources"`
}

func (r Rule) Equal(v Rule) bool {
	return r.Prefix == v.Prefix && r.RateMbps == v.RateMbps && r.Lock == v.Lock && r.NoRoute == v.NoRoute && r.Interface == v.Interface && table(r.Table) == table(v.Table)
}
func table(p *uint32) uint32 {
	if p != nil {
		return *p
	}
	return 0
}

var rateRE = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(bps|Kbps|Mbps|Gbps)?$`)

func Rate(raw json.RawMessage) (uint64, error) {
	s := string(raw)
	if strings.HasPrefix(s, `"`) {
		if e := json.Unmarshal(raw, &s); e != nil {
			return 0, e
		}
	}
	m := rateRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("invalid bit rate")
	}
	n, ok := new(big.Rat).SetString(m[1])
	if !ok {
		return 0, fmt.Errorf("invalid rate")
	}
	unit := map[string]int64{"": 1000000, "bps": 1, "Kbps": 1000, "Mbps": 1000000, "Gbps": 1000000000}[m[2]]
	n.Mul(n, big.NewRat(unit, 1000000))
	if !n.IsInt() || n.Sign() <= 0 || !n.Num().IsUint64() {
		return 0, fmt.Errorf("rate must convert to positive integer Mbps")
	}
	v := n.Num().Uint64()
	if v > 1<<32-1 {
		return 0, fmt.Errorf("rate exceeds Beta uint32 Mbps limit")
	}
	return v, nil
}
func Bits(raw json.RawMessage, a netip.Addr) (int, error) {
	max := 128
	if a.Is4() {
		max = 32
	}
	if len(raw) == 0 {
		return max, nil
	}
	var n int
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		var obj struct {
			V4 *int `json:"v4"`
			V6 *int `json:"v6"`
		}
		if e := config.Strict(raw, &obj); e != nil {
			return 0, e
		}
		if obj.V4 != nil && (*obj.V4 < 0 || *obj.V4 > 32) {
			return 0, fmt.Errorf("invalid v4 cidr")
		}
		if obj.V6 != nil && (*obj.V6 < 0 || *obj.V6 > 128) {
			return 0, fmt.Errorf("invalid v6 cidr")
		}
		p := obj.V6
		if a.Is4() {
			p = obj.V4
		}
		if p == nil {
			return max, nil
		}
		n = *p
	} else {
		if string(raw) == "null" {
			return 0, fmt.Errorf("null cidr")
		}
		if e := json.Unmarshal(raw, &n); e != nil {
			return 0, e
		}
	}
	if n < 0 || n > max {
		return 0, fmt.Errorf("cidr outside address family range")
	}
	return n, nil
}
func Normalize(c config.Rule, addrs []netip.Addr, source string) ([]Rule, error) {
	rate, e := Rate(c.Rate)
	if e != nil {
		return nil, e
	}
	if (c.IP == "") == (c.Domain == "") {
		return nil, fmt.Errorf("exactly one of ip/domain required")
	}
	if c.Table != nil && (*c.Table == 0 || *c.Table == 255) {
		return nil, fmt.Errorf("table 0/local unsupported")
	}
	if c.IP != "" {
		a, e := netip.ParseAddr(c.IP)
		if e != nil || a.Zone() != "" || a.Is4In6() {
			return nil, fmt.Errorf("invalid pure IP")
		}
		addrs = []netip.Addr{a}
		if strings.HasPrefix(string(c.CIDR), "{") {
			return nil, fmt.Errorf("IP cidr must be integer")
		}
	}
	lock := true
	if c.Lock != nil {
		lock = *c.Lock
	}
	out := []Rule{}
	for _, a := range addrs {
		bits, e := Bits(c.CIDR, a)
		if e != nil {
			return nil, e
		}
		out = append(out, Rule{netip.PrefixFrom(a, bits).Masked(), rate, lock, c.NoRoute || c.Table != nil || c.Interface != "", c.Table, c.Interface, []string{source}})
	}
	return out, nil
}
func Finalize(in []Rule) ([]Rule, map[string]bool) {
	seen := map[netip.Prefix]int{}
	bad := map[netip.Prefix]bool{}
	out := []Rule{}
	for _, r := range in {
		if i, ok := seen[r.Prefix]; ok {
			if !out[i].Equal(r) {
				bad[r.Prefix] = true
			}
			out[i].Sources = append(out[i].Sources, r.Sources...)
		} else {
			seen[r.Prefix] = len(out)
			out = append(out, r)
		}
	}
	failed := map[string]bool{}
	filtered := out[:0]
	for _, r := range out {
		if bad[r.Prefix] {
			for _, src := range r.Sources {
				failed[src] = true
			}
			continue
		}
		filtered = append(filtered, r)
	}
	sort.Slice(filtered, func(i, j int) bool {
		a, b := filtered[i].Prefix, filtered[j].Prefix
		if a.Addr().Is4() != b.Addr().Is4() {
			return a.Addr().Is4()
		}
		if n := a.Addr().Compare(b.Addr()); n != 0 {
			return n < 0
		}
		return a.Bits() < b.Bits()
	})
	return filtered, failed
}
