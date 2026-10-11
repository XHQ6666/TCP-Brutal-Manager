package brutal

import (
	"bufio"
	"context"
	"fmt"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/rules"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Current struct {
	Prefix         netip.Prefix      `json:"prefix"`
	BytesPerSecond uint64            `json:"bytes_per_second"`
	Lock           bool              `json:"lock"`
	Fields         map[string]string `json:"fields,omitempty"`
}

func Bool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("unknown lock %q", s)
}
func Read(path string) ([]Current, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return Parse(bufio.NewScanner(f))
}
func Parse(sc *bufio.Scanner) ([]Current, error) {
	out := []Current{}
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		m := map[string]string{}
		for _, f := range strings.Fields(sc.Text()) {
			k, v, ok := strings.Cut(f, "=")
			if ok {
				m[k] = v
			}
		}
		p, e := netip.ParsePrefix(m["dst"])
		if e != nil {
			return nil, e
		}
		rate, e := strconv.ParseUint(m["rate"], 10, 64)
		if e != nil {
			return nil, e
		}
		lock, e := Bool(m["lock"])
		if e != nil {
			return nil, e
		}
		out = append(out, Current{p.Masked(), rate, lock, m})
	}
	return out, sc.Err()
}
func (c Current) Matches(r rules.Rule) bool {
	return c.Prefix == r.Prefix && c.BytesPerSecond == r.RateMbps*125000 && c.Lock == r.Lock
}

type Controller struct {
	Binary  string
	Timeout time.Duration
}

func (c Controller) Exec(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	b, e := exec.CommandContext(ctx, c.Binary, args...).CombinedOutput()
	if e != nil {
		return fmt.Errorf("brutalctl %v: %w: %s", args, e, strings.TrimSpace(string(b)))
	}
	return nil
}
func (c Controller) Add(ctx context.Context, r rules.Rule) error {
	args := []string{"add", r.Prefix.String(), strconv.FormatUint(r.RateMbps, 10)}
	if !r.Lock {
		args = append(args, "nolock")
	}
	if r.NoRoute {
		args = append(args, "noroute")
	}
	return c.Exec(ctx, args...)
}
func (c Controller) Delete(ctx context.Context, p netip.Prefix) error {
	return c.Exec(ctx, "del", p.String())
}
