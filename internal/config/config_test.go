package config

import (
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	for _, settings := range []string{"", "\"settings\":{},", "\"settings\":null,", "\"settings\":{\"dns\":{\"query_interval\":\" \"}},"} {
		c, e := Parse([]byte("{"+settings+`"log":{"enabled":false},"rules":[{"ip":"192.0.2.1","rate":1}]}`), "/tmp/config.json")
		if e != nil || c.Durations.Query != time.Minute || c.Durations.DNS != 5*time.Second {
			t.Fatal(c, e)
		}
	}
}
func TestLocalDNSIssue(t *testing.T) {
	c, e := Parse([]byte(`{"rules":[{"ip":"192.0.2.1","rate":1}],"dns":{"rules":[{"domain":"x","resolver":"missing"}]}}`), "/tmp/c.json")
	if e != nil || len(c.Issues) != 1 {
		t.Fatal(c, e)
	}
}
func TestPatterns(t *testing.T) {
	for p, target := range map[string]string{"domain:example.com": "A.B.Example.com.", "full:example.com": "EXAMPLE.com.", "keyword:example": "myexample.test", "regexp:^a\\.": "a.test"} {
		if !Match(p, target) {
			t.Fatal(p, target)
		}
	}
	if Match("example.com", "a.example.com") || Match("domain:example.com", "badexample.com") {
		t.Fatal("boundary failure")
	}
}
func TestInvalidGlobal(t *testing.T) {
	for _, b := range []string{`{"rules":[]}`, `{"rules":[{}],"settings":{"dns":{"query_interval":"0s"}}}`, `{"rules":[{}],"log":{"level":"none"}}`, `{"rules":[{}],"unknown":1}`} {
		if _, e := Parse([]byte(b), "/tmp/c"); e == nil {
			t.Fatal(b)
		}
	}
}
