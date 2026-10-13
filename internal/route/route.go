package route

import (
	"context"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/rules"
	"net/netip"
)

type Route struct {
	Gateway         string       `json:"gateway,omitempty"`
	PreferredSource string       `json:"preferred_source,omitempty"`
	TOS             uint8        `json:"tos,omitempty"`
	Scope           uint8        `json:"scope,omitempty"`
	Prefix          netip.Prefix `json:"prefix"`
	Table           uint32       `json:"table"`
	Interface       string       `json:"interface"`
	Index           uint32       `json:"index"`
	Protocol        uint8        `json:"protocol"`
	Algorithm       string       `json:"algorithm"`
	Locked          bool         `json:"locked"`
	Priority        uint32       `json:"priority,omitempty"`
	NextHops        []Hop        `json:"next_hops,omitempty"`
	Raw             []byte       `json:"-"`
}
type Manager interface {
	Read(context.Context) ([]Route, error)
	Change(context.Context, Route, string, bool) error
	Watch(context.Context, chan<- struct{}) error
}

type Hop struct {
	Index     uint32 `json:"index"`
	Interface string `json:"interface"`
	Gateway   string `json:"gateway,omitempty"`
	Weight    uint8  `json:"weight"`
	Flags     uint8  `json:"flags"`
}

func InRange(r Route, w rules.Rule) bool {
	return r.Prefix.IsValid() && w.Prefix.IsValid() && r.Prefix.Addr().Is4() == w.Prefix.Addr().Is4() && r.Prefix.Bits() >= w.Prefix.Bits() && w.Prefix.Masked().Contains(r.Prefix.Masked().Addr()) && (!w.RouteStrict || r.Prefix.Masked() == w.Prefix.Masked()) && (w.Table == nil || r.Table == *w.Table)
}
func Matches(r Route, w rules.Rule) bool {
	if !InRange(r, w) {
		return false
	}
	if w.Interface == "" {
		return true
	}
	if len(r.NextHops) > 0 {
		for _, h := range r.NextHops {
			if h.Interface != w.Interface {
				return false
			}
		}
		return true
	}
	return r.Interface == w.Interface
}
func SameIdentity(a, b Route) bool {
	if a.Gateway != b.Gateway || a.PreferredSource != b.PreferredSource || a.TOS != b.TOS || a.Scope != b.Scope {
		return false
	}
	if a.Prefix != b.Prefix || a.Table != b.Table || a.Index != b.Index || a.Protocol != b.Protocol || a.Priority != b.Priority || len(a.NextHops) != len(b.NextHops) {
		return false
	}
	for i := range a.NextHops {
		if a.NextHops[i] != b.NextHops[i] {
			return false
		}
	}
	return true
}
