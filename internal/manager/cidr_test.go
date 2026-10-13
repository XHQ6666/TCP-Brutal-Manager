package manager

import (
	"context"
	"fmt"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/route"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/state"
	"net/netip"
	"testing"
)

func TestCIDRMultipleRoutesAndCleanup(t *testing.T) {
	m, _ := fixture(t, `{"rules":[{"ip":"192.0.2.0","cidr":24,"rate":200,"interface":"test0"}]}`)
	f := &fakeRoutes{routes: []route.Route{{Prefix: netip.MustParsePrefix("192.0.2.1/32"), Table: 52, Interface: "test0", Index: 2}, {Prefix: netip.MustParsePrefix("192.0.2.2/32"), Table: 52, Interface: "test0", Index: 2}, {Prefix: netip.MustParsePrefix("192.0.0.0/16"), Table: 52, Interface: "test0", Index: 2}}}
	m.Routes = f
	p, code, e := m.Sync(context.Background(), true, true)
	if e != nil || code != 0 || p.RouteSummary.Succeeded != 2 {
		t.Fatal(p, code, e)
	}
	for i, v := range f.routes {
		if (v.Algorithm == "brutal") != (i < 2) {
			t.Fatal(f.routes)
		}
	}
	d, bad, e := state.Load(m.StateDir)
	if e != nil || bad || len(d.Attributes) != 2 || len(d.Attributes[0].Sources) != 1 {
		t.Fatal(d, bad, e)
	}
	_, code, e = m.Sync(context.Background(), false, true)
	if code != 0 || e != nil {
		t.Fatal(code, e)
	}
	f.routes[0].Algorithm = ""
	_, code, e = m.Sync(context.Background(), true, false)
	if code != 0 || e != nil || f.routes[0].Algorithm != "brutal" {
		t.Fatal(code, e)
	}
	f.routes[0].Interface = "other"
	_, code, e = m.Sync(context.Background(), true, false)
	if code != 0 || e != nil || f.routes[0].Algorithm != "" {
		t.Fatal(code, e, f.routes)
	}
}
func TestMissingRouteStillAddsRule(t *testing.T) {
	m, f := fixture(t, `{"rules":[{"ip":"192.0.2.0","cidr":24,"rate":200,"table":52}]}`)
	_, code, e := m.Sync(context.Background(), true, true)
	if e != nil || code != 7 || len(f.calls) != 1 {
		t.Fatal(code, e, f.calls)
	}
	d, _, _ := state.Load(m.StateDir)
	if len(d.Records) != 1 {
		t.Fatal(d)
	}
}
func TestOverlappingRouteLock(t *testing.T) {
	m, _ := fixture(t, `{"rules":[{"ip":"192.0.2.0","cidr":24,"rate":200,"table":52},{"ip":"192.0.2.0","cidr":25,"rate":300,"table":52,"lock":false}]}`)
	f := &fakeRoutes{routes: []route.Route{{Prefix: netip.MustParsePrefix("192.0.2.1/32"), Table: 52, Index: 2}}}
	m.Routes = f
	p, code, e := m.Sync(context.Background(), true, true)
	if code != 0 || e != nil || f.routes[0].Locked || p.RouteSummary.Succeeded != 1 {
		t.Fatal(code, e, p, f.routes)
	}
	_, code, e = m.Sync(context.Background(), false, true)
	if code != 0 || e != nil {
		t.Fatal(code, e)
	}
}

type partialRoutes struct {
	fakeRoutes
	fail netip.Prefix
}

func (f *partialRoutes) Change(ctx context.Context, r route.Route, a string, l bool) error {
	if r.Prefix == f.fail {
		return fmt.Errorf("injected route failure")
	}
	return f.fakeRoutes.Change(ctx, r, a, l)
}
func TestRouteFailureDoesNotBlockSiblings(t *testing.T) {
	m, _ := fixture(t, `{"rules":[{"ip":"192.0.2.0","cidr":24,"rate":200,"table":52}]}`)
	f := &partialRoutes{fakeRoutes: fakeRoutes{routes: []route.Route{{Prefix: netip.MustParsePrefix("192.0.2.1/32"), Table: 52, Index: 2}, {Prefix: netip.MustParsePrefix("192.0.2.2/32"), Table: 52, Index: 2}}}, fail: netip.MustParsePrefix("192.0.2.1/32")}
	m.Routes = f
	p, code, e := m.Sync(context.Background(), true, true)
	if e != nil || code != 7 || p.RouteSummary.Failed != 1 || f.routes[1].Algorithm != "brutal" || f.routes[0].Algorithm != "" {
		t.Fatal(p, code, e, f.routes)
	}
	f.fail = netip.Prefix{}
	_, code, e = m.Sync(context.Background(), true, false)
	if code != 0 || e != nil || f.routes[0].Algorithm != "brutal" {
		t.Fatal(code, e)
	}
}
func TestStrictAllowsMultipleExactCandidates(t *testing.T) {
	m, _ := fixture(t, `{"rules":[{"ip":"192.0.2.0","cidr":24,"rate":200,"table":52,"route_strict":true}]}`)
	f := &fakeRoutes{routes: []route.Route{{Prefix: netip.MustParsePrefix("192.0.2.0/24"), Table: 52, Index: 2}, {Prefix: netip.MustParsePrefix("192.0.2.0/24"), Table: 52, Index: 3}, {Prefix: netip.MustParsePrefix("192.0.2.1/32"), Table: 52, Index: 2}}}
	m.Routes = f
	p, code, e := m.Sync(context.Background(), true, true)
	if code != 0 || e != nil || p.RouteSummary.Succeeded != 2 || f.routes[2].Algorithm != "" {
		t.Fatal(p, code, e)
	}
}
