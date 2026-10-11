//go:build linux

package route

import (
	"golang.org/x/sys/unix"
	"testing"
)

func TestMetricsPreservation(t *testing.T) {
	raw := make([]byte, 12)
	raw[0] = unix.AF_INET
	raw[1] = 32
	raw[4] = 52
	raw[7] = unix.RTN_UNICAST
	raw = append(raw, attr(unix.RTA_DST, []byte{192, 0, 2, 1})...)
	raw = append(raw, attr(unix.RTA_OIF, word(1))...)
	metrics := attr(unix.RTAX_MTU, word(1400))
	metrics = append(metrics, attr(unix.RTAX_LOCK, word(1<<unix.RTAX_MTU))...)
	raw = append(raw, attr(unix.RTA_METRICS, metrics)...)
	p, e := patch(raw, "brutal", true)
	if e != nil {
		t.Fatal(e)
	}
	r, e := decode(p)
	if e != nil || r.Algorithm != "brutal" || !r.Locked {
		t.Fatal(r, e)
	}
	a, _ := attrs(p[12:])
	m, _ := attrs(a[unix.RTA_METRICS])
	if u32(m[unix.RTAX_MTU]) != 1400 || u32(m[unix.RTAX_LOCK])&(1<<unix.RTAX_MTU) == 0 {
		t.Fatal("unrelated metric lost")
	}
	p, e = patch(p, "", false)
	if e != nil {
		t.Fatal(e)
	}
	a, _ = attrs(p[12:])
	m, _ = attrs(a[unix.RTA_METRICS])
	if m[unix.RTAX_CC_ALGO] != nil || u32(m[unix.RTAX_LOCK]) != 1<<unix.RTAX_MTU {
		t.Fatal("cleanup changed other locks")
	}
}
