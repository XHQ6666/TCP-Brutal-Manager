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

type fakeRoutes struct{}

func (fakeRoutes) Read(context.Context) ([]route.Route, error)             { return nil, nil }
func (fakeRoutes) Change(context.Context, route.Route, string, bool) error { return nil }
func (fakeRoutes) Watch(context.Context, chan<- struct{}) error            { return nil }

type fakeControl struct {
	path  string
	calls []string
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
	m.Routes = fakeRoutes{}
	f := &fakeControl{path: m.ProcPath}
	m.Control = f
	return m, f
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
