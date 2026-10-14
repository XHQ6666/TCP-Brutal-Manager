package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Issue struct {
	Index     int    `json:"index"`
	Component string `json:"component"`
	Error     string `json:"error"`
	Source    string `json:"source,omitempty"`
}
type Log struct {
	Enabled *bool  `json:"enabled"`
	Level   string `json:"level"`
	Path    string `json:"path"`
}
type Resolver struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Address string `json:"address,omitempty"`
	Path    string `json:"path,omitempty"`
	Port    int    `json:"port,omitempty"`
}
type DNSRule struct {
	Domain   json.RawMessage `json:"domain"`
	Resolver string          `json:"resolver"`
	Fallback bool            `json:"fallback"`
	Patterns []string        `json:"-"`
}
type DNS struct {
	Resolvers []Resolver        `json:"resolvers"`
	Rules     []json.RawMessage `json:"rules"`
}
type Rule struct {
	RouteStrict bool            `json:"route_strict,omitempty"`
	IP          string          `json:"ip,omitempty"`
	Domain      string          `json:"domain,omitempty"`
	Rate        json.RawMessage `json:"rate"`
	CIDR        json.RawMessage `json:"cidr,omitempty"`
	Lock        *bool           `json:"lock,omitempty"`
	NoRoute     bool            `json:"noroute,omitempty"`
	Table       *uint32         `json:"table,omitempty"`
	Interface   string          `json:"interface,omitempty"`
}
type Settings struct {
	DNS struct {
		QueryInterval json.RawMessage `json:"query_interval"`
		Timeout       json.RawMessage `json:"timeout"`
	} `json:"dns"`
	Config struct {
		Debounce json.RawMessage `json:"debounce"`
		Rescan   json.RawMessage `json:"rescan_interval"`
	} `json:"config"`
	Route struct {
		Debounce json.RawMessage `json:"debounce"`
		Resync   json.RawMessage `json:"resync_interval"`
		Timeout  json.RawMessage `json:"operation_timeout"`
	} `json:"route"`
	Brutal struct {
		Poll    json.RawMessage `json:"poll_interval"`
		Timeout json.RawMessage `json:"operation_timeout"`
	} `json:"brutal"`
}
type Durations struct{ Query, DNS, ConfigDebounce, Rescan, RouteDebounce, Resync, Route, Poll, Brutal time.Duration }
type Config struct {
	Settings  Settings          `json:"settings"`
	Log       Log               `json:"log"`
	DNS       DNS               `json:"dns"`
	Rules     []json.RawMessage `json:"rules"`
	Path      string            `json:"-"`
	Durations Durations         `json:"-"`
	DNSRules  []DNSRule         `json:"-"`
	Issues    []Issue           `json:"-"`
}

func Strict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
func duration(raw json.RawMessage, def time.Duration) (time.Duration, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return def, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return def, nil
	}
	d, e := time.ParseDuration(s)
	if e != nil || d <= 0 {
		return 0, fmt.Errorf("invalid positive duration %q", s)
	}
	return d, nil
}
func Load(path string) (*Config, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	return Parse(b, path)
}
func Parse(b []byte, path string) (*Config, error) {
	c := new(Config)
	if e := Strict(b, c); e != nil {
		return nil, e
	}
	var e error
	c.Path, e = filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	if len(c.Rules) == 0 {
		return nil, fmt.Errorf("rules must not be empty")
	}
	if c.Log.Enabled == nil {
		v := true
		c.Log.Enabled = &v
	}
	if c.Log.Level == "" {
		c.Log.Level = "default"
	}
	switch c.Log.Level {
	case "default", "debug", "warning", "error":
	default:
		return nil, fmt.Errorf("invalid log level")
	}
	if c.Log.Path == "" {
		c.Log.Path = "/var/log/tcp-brutal-manager.log"
	}
	if !filepath.IsAbs(c.Log.Path) {
		c.Log.Path = filepath.Join(filepath.Dir(c.Path), c.Log.Path)
	}
	ds := []struct {
		raw json.RawMessage
		def time.Duration
		dst *time.Duration
	}{{c.Settings.DNS.QueryInterval, time.Minute, &c.Durations.Query}, {c.Settings.DNS.Timeout, 5 * time.Second, &c.Durations.DNS}, {c.Settings.Config.Debounce, 500 * time.Millisecond, &c.Durations.ConfigDebounce}, {c.Settings.Config.Rescan, time.Minute, &c.Durations.Rescan}, {c.Settings.Route.Debounce, 500 * time.Millisecond, &c.Durations.RouteDebounce}, {c.Settings.Route.Resync, time.Minute, &c.Durations.Resync}, {c.Settings.Route.Timeout, 10 * time.Second, &c.Durations.Route}, {c.Settings.Brutal.Poll, 30 * time.Second, &c.Durations.Poll}, {c.Settings.Brutal.Timeout, 10 * time.Second, &c.Durations.Brutal}}
	for _, x := range ds {
		*x.dst, e = duration(x.raw, x.def)
		if e != nil {
			return nil, e
		}
	}
	if len(c.DNS.Resolvers) == 0 {
		c.DNS.Resolvers = []Resolver{{Name: "system", Type: "System"}}
	}
	names := map[string]bool{}
	for i := range c.DNS.Resolvers {
		v := &c.DNS.Resolvers[i]
		if v.Name == "" || names[v.Name] {
			return nil, fmt.Errorf("missing or duplicate resolver name")
		}
		names[v.Name] = true
		switch v.Type {
		case "System":
			if v.Address != "" || v.Port != 0 || v.Path != "" {
				return nil, fmt.Errorf("System accepts no endpoint")
			}
		case "DoH", "DoT", "DoQ":
			if v.Address == "" || strings.ContainsAny(v.Address, "/:?#") {
				return nil, fmt.Errorf("resolver address must be a hostname")
			}
			if v.Port == 0 {
				v.Port = 853
				if v.Type == "DoH" {
					v.Port = 443
				}
			}
			if v.Port < 1 || v.Port > 65535 {
				return nil, fmt.Errorf("invalid resolver port")
			}
			if v.Type == "DoH" {
				if v.Path == "" {
					v.Path = "/dns-query"
				}
				if !strings.HasPrefix(v.Path, "/") || strings.ContainsAny(v.Path, "?#") {
					return nil, fmt.Errorf("invalid DoH path")
				}
			} else if v.Path != "" {
				return nil, fmt.Errorf("path only valid for DoH")
			}
		default:
			return nil, fmt.Errorf("invalid resolver type %q", v.Type)
		}
	}
	for i, b := range c.DNS.Rules {
		var v DNSRule
		e = Strict(b, &v)
		if e == nil {
			e = json.Unmarshal(v.Domain, &v.Patterns)
			if e != nil {
				var one string
				e = json.Unmarshal(v.Domain, &one)
				v.Patterns = []string{one}
			}
		}
		if e == nil && !names[v.Resolver] {
			e = fmt.Errorf("unknown resolver")
		}
		if e == nil && len(v.Patterns) == 0 {
			e = fmt.Errorf("empty patterns")
		}
		if e == nil {
			for _, p := range v.Patterns {
				kind, value := Pattern(p)
				if value == "" {
					e = fmt.Errorf("empty pattern")
					break
				}
				switch kind {
				case "full", "domain", "keyword":
				case "regexp":
					_, e = regexp.Compile(value)
				default:
					e = fmt.Errorf("unsupported pattern")
				}
				if e != nil {
					break
				}
			}
		}
		if e != nil {
			c.Issues = append(c.Issues, Issue{i, "dns", e.Error(), ""})
			continue
		}
		c.DNSRules = append(c.DNSRules, v)
	}
	return c, nil
}
func Canon(s string) string { return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".") }
func Pattern(s string) (string, string) {
	if k, v, ok := strings.Cut(s, ":"); ok {
		return k, v
	}
	return "full", s
}
func Match(pattern, domain string) bool {
	k, v := Pattern(pattern)
	domain = Canon(domain)
	if k == "regexp" {
		r, e := regexp.Compile(v)
		return e == nil && r.MatchString(domain)
	}
	v = Canon(v)
	switch k {
	case "full":
		return domain == v
	case "domain":
		return domain == v || strings.HasSuffix(domain, "."+v)
	case "keyword":
		return strings.Contains(domain, v)
	}
	return false
}
func (c *Config) Select(domain string) (Resolver, bool) {
	for _, v := range c.DNSRules {
		for _, p := range v.Patterns {
			if Match(p, domain) {
				for _, r := range c.DNS.Resolvers {
					if r.Name == v.Resolver {
						return r, v.Fallback
					}
				}
			}
		}
	}
	return c.DNS.Resolvers[0], false
}
