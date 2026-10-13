package manager

import (
	"context"
	"fmt"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/config"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/route"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/rules"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/state"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

type fakeRoutes struct{ routes []route.Route }

func (f *fakeRoutes) Read(context.Context) ([]route.Route, error) {
	return append([]route.Route(nil), f.routes...), nil
}
func (f *fakeRoutes) Change(_ context.Context, old route.Route, algo string, lock bool) error {
	for i := range f.routes {
		if sameRouteIdentity(f.routes[i], old) {
			f.routes[i].Algorithm = algo
			f.routes[i].Locked = lock
			return nil
		}
	}
	return fmt.Errorf("route not found")
}
func (f *fakeRoutes) Watch(context.Context, chan<- struct{}) error { return nil }

type fakeControl struct {
	path  string
	calls []string
}

type failedResolver struct{}

func (failedResolver) Resolve(context.Context, string) ([]netip.Addr, error) {
	return nil, fmt.Errorf("temporary DNS failure")
}

func (f *fakeControl) Add(_ context.Context, r rules.Rule) error {
	f.calls = append(f.calls, "add")
	b, _ := os.ReadFile(f.path)
	b = append(b, []byte(fmt.Sprintf("dst=%s rate=%d lock=%t\n", r.Prefix, r.RateMbps*125000, r.Lock))...)
	return os.WriteFile(f.path, b, 0600)
}
func (f *fakeControl) Delete(context.Context, netip.Prefix) error {
	f.calls = append(f.calls, "del")
	return nil
}
func (f *fakeControl) Exec(context.Context, ...string) error {
	f.calls = append(f.calls, "flush")
	return os.WriteFile(f.path, nil, 0600)
}
func fixture(t *testing.T, raw string) (*Manager, *fakeControl) {
	t.Helper()
	dir := t.TempDir()
	c, e := config.Parse([]byte(raw), filepath.Join(dir, "config.json"))
	if e != nil {
		t.Fatal(e)
	}
	m := New(c)
	m.ProcPath = filepath.Join(dir, "proc")
	os.WriteFile(m.ProcPath, nil, 0600)
	m.LockPath = filepath.Join(dir, "lock")
	m.StateDir = filepath.Join(dir, "state")
	m.Routes = &fakeRoutes{}
	f := &fakeControl{path: m.ProcPath}
	m.Control = f
	return m, f
}

func TestCleanupRemovesOnlyMatchingManagedAttributes(t *testing.T) {
	m, _ := fixture(t, `{"log":{"enabled":false},"rules":[{"ip":"192.0.2.1","rate":200}]}`)
	current := route.Route{Prefix: netip.MustParsePrefix("192.0.2.0/24"), Table: 254, Interface: "ens3", Index: 2, Protocol: 3, Algorithm: "brutal", Locked: true}
	routes := &fakeRoutes{routes: []route.Route{current}}
	m.Routes = routes
	managed := current
	managed.Locked = false // Go wrote an unlocked value; another actor changed this bit afterward.
	if e := m.cleanup(context.Background(), state.Attribute{Route: managed, AddedAlgorithm: true, ChangedLock: true}); e != nil {
		t.Fatal(e)
	}
	if routes.routes[0].Algorithm != "" || !routes.routes[0].Locked {
		t.Fatalf("cleanup should remove the still-matching algorithm and preserve the changed lock: %+v", routes.routes[0])
	}
}

func TestRouteAttributeCleanupTracksRouteIdentity(t *testing.T) {
	prefix := netip.MustParsePrefix("192.0.2.0/24")
	old := route.Route{Prefix: prefix, Table: 52, Interface: "ens3", Index: 2, Protocol: 3}
	newRoute := route.Route{Prefix: prefix, Table: 52, Interface: "tailscale0", Index: 9, Protocol: 3}
	a := state.Attribute{Route: old, AddedAlgorithm: true}
	if routeAttributeNeeded(a, map[netip.Prefix][]route.Route{prefix: {newRoute}}, nil, nil) {
		t.Fatal("a different selected route must not retain old route attributes")
	}
}

func TestRecoveryCleaningDoesNotFlushAgain(t *testing.T) {
	m, f := fixture(t, `{"log":{"enabled":false},"rules":[{"ip":"192.0.2.1","rate":200}]}`)
	if e := state.Save(m.StateDir, state.Data{Version: 1, Recovery: "cleaning"}); e != nil {
		t.Fatal(e)
	}
	_, code, e := m.Sync(context.Background(), true, true)
	if e != nil || code != 0 || len(f.calls) != 1 || f.calls[0] != "add" {
		t.Fatalf("recovery resume should continue without repeating flush: code=%d err=%v calls=%v", code, e, f.calls)
	}
}

func TestFailedSourceProtectsOldSamePrefixParameters(t *testing.T) {
	m, f := fixture(t, `{"log":{"enabled":false},"rules":[{"domain":"example.com","rate":200},{"ip":"192.0.2.1","rate":300}]}`)
	m.Resolver = failedResolver{}
	if e := os.WriteFile(m.ProcPath, []byte("dst=192.0.2.1/32 rate=25000000 lock=1\n"), 0600); e != nil {
		t.Fatal(e)
	}
	old := rules.Rule{
		Prefix:   netip.MustParsePrefix("192.0.2.1/32"),
		RateMbps: 200,
		Lock:     true,
		Sources:  []string{m.Config.Path + "|domain|example.com|1"},
	}
	if e := state.Save(m.StateDir, state.Data{Version: 1, Records: []state.Record{{Rule: old}}}); e != nil {
		t.Fatal(e)
	}
	_, code, e := m.Sync(context.Background(), true, true)
	if e != nil || code != 7 || len(f.calls) != 0 {
		t.Fatalf("failed source must protect old same-prefix parameters: code=%d err=%v calls=%v", code, e, f.calls)
	}
	data, bad, e := state.Load(m.StateDir)
	if e != nil || bad || len(data.Records) != 1 || data.Records[0].Rule.RateMbps != 200 {
		t.Fatalf("old state was not preserved: data=%+v recovery=%t err=%v", data, bad, e)
	}
}
func TestDryRunNoWrites(t *testing.T) {
	m, f := fixture(t, `{"log":{"enabled":false},"rules":[{"ip":"192.0.2.1","rate":200}]}`)
	p, code, e := m.Sync(context.Background(), false, true)
	if e != nil || code != 3 || len(p.Added) != 1 || len(f.calls) != 0 {
		t.Fatal(p, code, e, f.calls)
	}
	if _, e = os.Stat(m.StateDir); !os.IsNotExist(e) {
		t.Fatal("dry run wrote state")
	}
}
func TestLocalFailureContinues(t *testing.T) {
	m, f := fixture(t, `{"log":{"enabled":false},"rules":[{"ip":"bad","rate":200},{"ip":"192.0.2.1","rate":200}]}`)
	p, code, e := m.Sync(context.Background(), true, true)
	if e != nil || code != 7 || len(f.calls) != 1 || f.calls[0] != "add" {
		t.Fatal(p, code, e, f.calls)
	}
	d, bad, e := state.Load(m.StateDir)
	if e != nil || bad || len(d.Records) != 1 {
		t.Fatal(d, bad, e)
	}
}
func TestNoDifferenceDoesNotFlush(t *testing.T) {
	m, f := fixture(t, `{"log":{"enabled":false},"rules":[{"ip":"192.0.2.1","rate":200}]}`)
	os.WriteFile(m.ProcPath, []byte("dst=192.0.2.1/32 rate=25000000 lock=1\n"), 0600)
	p, code, e := m.Sync(context.Background(), true, true)
	if e != nil || code != 0 || len(f.calls) != 0 {
		t.Fatal(p, code, e, f.calls)
	}
}
