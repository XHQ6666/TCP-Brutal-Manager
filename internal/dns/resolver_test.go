package dns

import (
	"context"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/config"
	"net/netip"
	"testing"
	"time"
)

func TestCanceledRetry(t *testing.T) {
	c, e := config.Parse([]byte(`{"log":{"enabled":false},"rules":[{"ip":"192.0.2.1","rate":1}]}`), "/tmp/c")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = (Engine{Config: c, RetryDelay: time.Hour}).Resolve(ctx, "example.invalid")
	if e == nil {
		t.Fatal("cancel ignored")
	}
}

var _ netip.Addr
