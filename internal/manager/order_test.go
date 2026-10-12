package manager

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"testing"

	"github.com/XHQ6666/TCP-Brutal-Manager/internal/rules"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/state"
)

func TestOrderExcludesUnclaimedExternalRules(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current []int
		owned   []int
		reorder bool
	}{
		{"external appended after owned", []int{1, 3, 2}, []int{1, 3}, false},
		{"all external reversed", []int{3, 2, 1}, nil, false},
		{"owned order still checked", []int{3, 1, 2}, []int{1, 3}, true},
	} {
		for _, write := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/write=%t", tc.name, write), func(t *testing.T) {
				m, f := fixture(t, `{"log":{"enabled":false},"rules":[{"ip":"192.0.2.1","rate":200},{"ip":"192.0.2.2","rate":200},{"ip":"192.0.2.3","rate":200}]}`)
				var contents string
				for _, n := range tc.current {
					contents += fmt.Sprintf("dst=192.0.2.%d/32 rate=25000000 lock=1\n", n)
				}
				if err := os.WriteFile(m.ProcPath, []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
				data := state.Data{Version: 1}
				for _, n := range tc.owned {
					ip := fmt.Sprintf("192.0.2.%d", n)
					data.Records = append(data.Records, state.Record{Rule: rules.Rule{Prefix: netip.MustParsePrefix(ip + "/32"), RateMbps: 200, Lock: true, Sources: []string{m.Config.Path + "|ip|" + ip + "|1"}}})
				}
				if err := state.Save(m.StateDir, data); err != nil {
					t.Fatal(err)
				}
				p, code, err := m.Sync(context.Background(), write, true)
				want := 0
				if tc.reorder {
					want = 3
					if write {
						want = 7
					}
				}
				if err != nil || code != want || p.Reorder != tc.reorder || len(p.Issues) != 0 || len(f.calls) != 0 {
					t.Fatalf("plan=%+v code=%d want=%d err=%v calls=%v", p, code, want, err, f.calls)
				}
				saved, bad, err := state.Load(m.StateDir)
				if err != nil || bad || len(saved.Records) != len(tc.owned) {
					t.Fatalf("external ownership changed: %+v bad=%t err=%v", saved, bad, err)
				}
			})
		}
	}
}

func TestNewManagedRulesOrderedAroundExternalRule(t *testing.T) {
	m, f := fixture(t, `{"log":{"enabled":false},"rules":[{"ip":"192.0.2.1","rate":200},{"ip":"192.0.2.2","rate":200},{"ip":"192.0.2.3","rate":200}]}`)
	if err := os.WriteFile(m.ProcPath, []byte("dst=192.0.2.2/32 rate=25000000 lock=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p, code, err := m.Sync(context.Background(), true, true)
	if err != nil || code != 0 || p.Reorder || len(f.calls) != 2 {
		t.Fatalf("plan=%+v code=%d err=%v calls=%v", p, code, err, f.calls)
	}
	saved, bad, err := state.Load(m.StateDir)
	if err != nil || bad || len(saved.Records) != 2 {
		t.Fatalf("state=%+v bad=%t err=%v", saved, bad, err)
	}
	for _, r := range saved.Records {
		if r.Rule.Prefix.String() == "192.0.2.2/32" {
			t.Fatal("external rule was claimed")
		}
	}
}
