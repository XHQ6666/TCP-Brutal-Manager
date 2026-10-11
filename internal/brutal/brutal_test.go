package brutal

import (
	"bufio"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	v, e := Parse(bufio.NewScanner(strings.NewReader("dst=192.0.2.1/32 rate=25000000 lock=1 gain=10\ndst=2001:db8::1/128 rate=125000 lock=false\n")))
	if e != nil || len(v) != 2 || !v[0].Lock || v[1].Lock {
		t.Fatal(v, e)
	}
}
func TestRejectMalformed(t *testing.T) {
	if _, e := Parse(bufio.NewScanner(strings.NewReader("dst=bad rate=0 lock=1"))); e == nil {
		t.Fatal("accepted")
	}
}
