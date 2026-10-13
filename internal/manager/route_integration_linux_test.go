//go:build linux

package manager

import (
	"context"
	"fmt"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/brutal"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/config"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/state"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Explicit opt-in: modifies only the reserved documentation scope in an unused table.
func TestLinuxCIDRIntegration(t *testing.T) {
	if os.Getenv("TCPBM_ROUTE_INTEGRATION") != "1" {
		t.Skip("requires explicit isolated Linux test approval")
	}
	ctx := context.Background()
	ip := func(args ...string) string {
		t.Helper()
		b, e := exec.Command("ip", args...).CombinedOutput()
		if e != nil {
			t.Fatalf("ip test fixture failed: %v", e)
		}
		return string(b)
	}
	tableOut, tableErr := exec.Command("ip", "-4", "route", "show", "table", "20261").CombinedOutput()
	if tableErr != nil && !strings.Contains(string(tableOut), "FIB table does not exist") {
		t.Fatal("test table read failed")
	}
	if tableErr == nil && len(tableOut) > 0 {
		t.Fatal("test table occupied")
	}
	cur, e := brutal.Read("/proc/net/tcp_brutal/rules")
	if e != nil {
		t.Fatal(e)
	}
	testPrefix := netip.MustParsePrefix("192.0.2.0/24")
	for _, v := range cur {
		if v.Prefix.Overlaps(testPrefix) {
			t.Fatal("documentation scope already used")
		}
	}
	if out := ip("-4", "route", "show", "table", "main", "exact", "192.0.2.0/24"); out != "" {
		t.Fatal("main test scope occupied")
	}
	if out := ip("-4", "rule", "show"); strings.Contains(out, "lookup 20261") {
		t.Fatal("test table is referenced by policy rule")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	load := func(strict bool) {
		t.Helper()
		raw := fmt.Sprintf(`{"log":{"enabled":false},"rules":[{"ip":"192.0.2.0","cidr":24,"rate":200,"table":20261,"route_strict":%t}]}`, strict)
		if e := os.WriteFile(path, []byte(raw), 0600); e != nil {
			t.Fatal(e)
		}
	}
	load(false)
	c, e := config.Load(path)
	if e != nil {
		t.Fatal(e)
	}
	m := New(c)
	m.StateDir = filepath.Join(dir, "state")
	t.Cleanup(func() {
		m.Control.Delete(ctx, testPrefix)
		exec.Command("ip", "-4", "route", "flush", "table", "20261").Run()
	})
	ip("-4", "route", "add", "192.0.2.10/32", "dev", "lo", "table", "20261", "mtu", "1400")
	ip("-4", "route", "add", "192.0.2.20/32", "dev", "lo", "table", "20261", "mtu", "1400")
	p, code, e := m.Sync(ctx, true, true)
	if e != nil || code != 0 || p.RouteSummary.Succeeded != 2 {
		t.Fatalf("range apply code=%d err=%v summary=%+v", code, e, p.RouteSummary)
	}
	for _, dst := range []string{"192.0.2.10/32", "192.0.2.20/32"} {
		out := ip("-4", "route", "show", "table", "20261", "exact", dst)
		if !strings.Contains(out, "congctl lock brutal") || !strings.Contains(out, "mtu 1400") {
			t.Fatal("attributes or MTU incorrect")
		}
	}
	ip("-4", "route", "change", "192.0.2.10/32", "dev", "lo", "table", "20261", "mtu", "1400")
	_, code, e = m.Sync(ctx, true, false)
	if code != 0 || e != nil {
		t.Fatal("repair", code, e)
	}
	ip("-4", "route", "add", "192.0.2.30/32", "dev", "lo", "table", "20261")
	p, code, e = m.Sync(ctx, true, false)
	if code != 0 || e != nil || p.RouteSummary.Matched != 3 {
		t.Fatal("new route", code, e)
	}
	load(true)
	m.Config, e = config.Load(path)
	if e != nil {
		t.Fatal(e)
	}
	_, code, e = m.Sync(ctx, true, false)
	if code != 7 || e != nil {
		t.Fatal("strict missing", code, e)
	}
	for _, dst := range []string{"192.0.2.10/32", "192.0.2.20/32", "192.0.2.30/32"} {
		if strings.Contains(ip("-4", "route", "show", "table", "20261", "exact", dst), "congctl") {
			t.Fatal("strict cleanup failed")
		}
	}
	d, bad, e := state.Load(m.StateDir)
	if e != nil || bad || len(d.Records) != 1 || len(d.Attributes) != 0 {
		t.Fatal("state integrity")
	}
	t.Log("range apply, attribute preservation, repair, new route, strict cleanup and isolated state passed")
}
