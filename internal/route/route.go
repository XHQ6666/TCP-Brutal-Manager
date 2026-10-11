package route

import (
	"context"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/rules"
	"net/netip"
)

type Route struct {
	Prefix    netip.Prefix `json:"prefix"`
	Table     uint32       `json:"table"`
	Interface string       `json:"interface"`
	Index     uint32       `json:"index"`
	Protocol  uint8        `json:"protocol"`
	Algorithm string       `json:"algorithm"`
	Locked    bool         `json:"locked"`
	Raw       []byte       `json:"-"`
}
type Manager interface {
	Read(context.Context) ([]Route, error)
	Change(context.Context, Route, string, bool) error
	Watch(context.Context, chan<- struct{}) error
}

func Matches(r Route, w rules.Rule) bool {
	return r.Prefix == w.Prefix && (w.Table == nil || r.Table == *w.Table) && (w.Interface == "" || r.Interface == w.Interface)
}
