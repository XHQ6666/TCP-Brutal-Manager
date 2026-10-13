package route

import (
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/rules"
	"net/netip"
	"testing"
)

func TestCIDRScope(t *testing.T) {
	table := uint32(52)
	w := rules.Rule{Prefix: netip.MustParsePrefix("192.0.2.0/24"), Table: &table, Interface: "test0"}
	for _, tc := range []struct {
		prefix string
		want   bool
	}{{"192.0.2.0/24", true}, {"192.0.2.4/32", true}, {"192.0.3.4/32", false}, {"192.0.0.0/16", false}, {"::/0", false}} {
		r := Route{Prefix: netip.MustParsePrefix(tc.prefix), Table: 52, Interface: "test0"}
		if Matches(r, w) != tc.want {
			t.Fatal(tc)
		}
	}
	r := Route{Prefix: netip.MustParsePrefix("192.0.2.4/32"), Table: 52, Interface: "test0"}
	w.RouteStrict = true
	if Matches(r, w) {
		t.Fatal("strict accepted child")
	}
	w.RouteStrict = false
	r.Table = 53
	if Matches(r, w) {
		t.Fatal("wrong table")
	}
}
func TestMultipathInterfaceScope(t *testing.T) {
	w := rules.Rule{Prefix: netip.MustParsePrefix("2001:db8::/32"), Interface: "test0"}
	r := Route{Prefix: netip.MustParsePrefix("2001:db8:1::/64"), NextHops: []Hop{{Index: 1, Interface: "test0"}, {Index: 2, Interface: "test0"}}}
	if !Matches(r, w) {
		t.Fatal("matching ECMP rejected")
	}
	r.NextHops[1].Interface = "test1"
	if Matches(r, w) {
		t.Fatal("mixed ECMP accepted")
	}
	w.Interface = ""
	if !Matches(r, w) {
		t.Fatal("table-only ECMP rejected")
	}
}
