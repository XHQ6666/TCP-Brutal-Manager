package rules

import (
	"encoding/json"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/config"
	"net/netip"
	"testing"
)

func TestRate(t *testing.T) {
	for in, want := range map[string]uint64{`200`: 200, `"200"`: 200, `"200Mbps"`: 200, `"2.5Gbps"`: 2500, `"1000000bps"`: 1} {
		got, e := Rate(json.RawMessage(in))
		if e != nil || got != want {
			t.Fatalf("%s: %d %v", in, got, e)
		}
	}
	for _, in := range []string{`0`, `-1`, `"2.5Mbps"`, `"1MBps"`, `null`, `"NaN"`, `true`} {
		if _, e := Rate(json.RawMessage(in)); e == nil {
			t.Fatal(in)
		}
	}
}
func TestNormalize(t *testing.T) {
	c := config.Rule{IP: "192.0.2.123", Rate: json.RawMessage(`200`), CIDR: json.RawMessage(`24`)}
	out, e := Normalize(c, nil, "a")
	if e != nil || out[0].Prefix.String() != "192.0.2.0/24" || !out[0].Lock {
		t.Fatalf("%v %v", out, e)
	}
	c.CIDR = json.RawMessage(`0`)
	out, e = Normalize(c, nil, "a")
	if e != nil || out[0].Prefix.Bits() != 0 {
		t.Fatal(out, e)
	}
	c.IP = "192.0.2.1/24"
	if _, e = Normalize(c, nil, "a"); e == nil {
		t.Fatal("CIDR string accepted")
	}
}
func TestFinalize(t *testing.T) {
	makeRule := func(p, src string) Rule {
		return Rule{Prefix: netip.MustParsePrefix(p), RateMbps: 200, Lock: true, Sources: []string{src}}
	}
	out, bad := Finalize([]Rule{makeRule("2001:db8::1/128", "v6"), makeRule("192.0.2.10/32", "b"), makeRule("192.0.2.2/32", "a"), makeRule("192.0.2.2/32", "c")})
	if len(bad) != 0 || len(out) != 3 || out[0].Prefix.String() != "192.0.2.2/32" || len(out[0].Sources) != 2 {
		t.Fatal(out, bad)
	}
	conflict := makeRule("192.0.2.2/32", "d")
	conflict.RateMbps = 300
	out, bad = Finalize(append(out, conflict))
	if len(out) != 2 || !bad["a"] || !bad["c"] || !bad["d"] {
		t.Fatal(out, bad)
	}
}
func TestCIDR(t *testing.T) {
	v4 := netip.MustParseAddr("192.0.2.1")
	v6 := netip.MustParseAddr("2001:db8::1")
	if n, e := Bits(json.RawMessage(`{"v6":64}`), v4); n != 32 || e != nil {
		t.Fatal(n, e)
	}
	if n, e := Bits(json.RawMessage(`{"v6":64}`), v6); n != 64 || e != nil {
		t.Fatal(n, e)
	}
	if _, e := Bits(json.RawMessage(`64`), v4); e == nil {
		t.Fatal("v4 /64")
	}
}

func TestIntegerMbpsOfficialConversion(t *testing.T) {
	for _, v := range []uint64{1, 200, 2500, 1 << 20, 1<<32 - 1} {
		got := uint64(float64(v)*1e6/8 + 0.5)
		if got != v*125000 {
			t.Fatal(v, got)
		}
	}
}
