package manager

import (
	"context"
	"fmt"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/brutal"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/config"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/dns"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/route"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/rules"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/state"
	"net/netip"
	"os"
	"strings"
)

type Resolver interface {
	Resolve(context.Context, string) ([]netip.Addr, error)
}
type Controller interface {
	Add(context.Context, rules.Rule) error
	Delete(context.Context, netip.Prefix) error
	Exec(context.Context, ...string) error
}
type Manager struct {
	Config                       *config.Config
	Resolver                     Resolver
	Routes                       route.Manager
	Control                      Controller
	ProcPath, StateDir, LockPath string
	Cache                        map[string][]netip.Addr
	Before                       func(string) error
}
type Plan struct {
	Desired      []rules.Rule      `json:"desired"`
	Added        []rules.Rule      `json:"would_add"`
	Updated      []rules.Rule      `json:"would_update"`
	Removed      []rules.Rule      `json:"would_remove"`
	RouteRepair  []route.Route     `json:"would_repair_route"`
	RouteCleanup []state.Attribute `json:"would_cleanup_attributes"`
	Issues       []config.Issue    `json:"issues"`
	Reorder      bool              `json:"would_reorder"`
	Recovery     bool              `json:"would_flush"`
}

func New(c *config.Config) *Manager {
	return &Manager{c, dns.Engine{Config: c}, route.Native{}, brutal.Controller{Binary: "brutalctl", Timeout: c.Durations.Brutal}, "/proc/net/tcp_brutal/rules", "/var/lib/tcp-brutal-config", "/run/tcp-brutal-config.lock", map[string][]netip.Addr{}, nil}
}
func (m *Manager) desired(ctx context.Context, refresh bool) ([]rules.Rule, map[string]bool, []config.Issue) {
	candidate := []rules.Rule{}
	failed := map[string]bool{}
	issues := append([]config.Issue{}, m.Config.Issues...)
	counts := map[string]int{}
	for i, b := range m.Config.Rules {
		var r config.Rule
		e := config.Strict(b, &r)
		source := m.Config.Path + "|ip|" + r.IP
		if r.Domain != "" {
			source = m.Config.Path + "|domain|" + config.Canon(r.Domain)
		}
		counts[source]++
		source = fmt.Sprintf("%s|%d", source, counts[source])
		var addrs []netip.Addr
		if e == nil && r.Domain != "" {
			if strings.ContainsAny(r.Domain, "/:* ") {
				e = fmt.Errorf("invalid query domain")
			} else {
				if !refresh {
					var ok bool
					addrs, ok = m.Cache[r.Domain]
					if !ok {
						e = fmt.Errorf("no successful DNS result in this process")
					}
				} else {
					addrs, e = m.Resolver.Resolve(ctx, r.Domain)
					if e == nil {
						m.Cache[r.Domain] = addrs
					}
				}
			}
		}
		var expanded []rules.Rule
		if e == nil {
			expanded, e = rules.Normalize(r, addrs, source)
		}
		if e != nil {
			failed[source] = true
			issues = append(issues, config.Issue{i, "rules", e.Error(), source})
			continue
		}
		candidate = append(candidate, expanded...)
	}
	final, conflicts := rules.Finalize(candidate)
	for src := range conflicts {
		failed[src] = true
		issues = append(issues, config.Issue{-1, "rules", "same-prefix parameter conflict", src})
	}
	return final, failed, issues
}
func protected(r rules.Rule, failed map[string]bool) bool {
	for _, s := range r.Sources {
		if failed[s] {
			return true
		}
	}
	return false
}
func (m *Manager) Sync(ctx context.Context, write, refresh bool) (Plan, int, error) {
	desired, failed, issues := m.desired(ctx, refresh)
	plan := Plan{Desired: desired, Issues: issues}
	unlock, e := state.Lock(m.LockPath)
	if e != nil {
		return plan, 1, e
	}
	defer unlock()
	current, e := brutal.Read(m.ProcPath)
	if e != nil {
		return plan, 6, e
	}
	data, recovery, e := state.Load(m.StateDir)
	if e != nil {
		return plan, 1, e
	}
	plan.Recovery = recovery || data.Recovery != ""
	all, e := m.Routes.Read(ctx)
	if e != nil {
		return plan, 1, e
	}
	owned := map[netip.Prefix]state.Record{}
	for _, r := range data.Records {
		owned[r.Rule.Prefix] = r
	}
	now := map[netip.Prefix]brutal.Current{}
	for _, r := range current {
		now[r.Prefix] = r
	}
	for _, pending := range data.Pending {
		if c, ok := now[pending.Rule.Prefix]; ok && c.Matches(pending.Rule) {
			owned[pending.Rule.Prefix] = pending
		}
	}
	targets := map[netip.Prefix]rules.Rule{}
	selected := map[netip.Prefix]route.Route{}
	issue := func(r rules.Rule, msg string) {
		for _, s := range r.Sources {
			failed[s] = true
		}
		plan.Issues = append(plan.Issues, config.Issue{-1, "apply", msg, strings.Join(r.Sources, ",")})
	}
	for _, r := range desired {
		targets[r.Prefix] = r
		if c, ok := now[r.Prefix]; ok {
			if _, ours := owned[r.Prefix]; !ours && !c.Matches(r) && !plan.Recovery {
				issue(r, "external same-prefix rule conflict")
				continue
			}
		}
		if r.Table != nil || r.Interface != "" {
			matches := []route.Route{}
			for _, v := range all {
				if route.Matches(v, r) {
					matches = append(matches, v)
				}
			}
			if len(matches) != 1 {
				issue(r, "policy route must uniquely match exact prefix/interface/table")
				continue
			}
			selected[r.Prefix] = matches[0]
			for _, v := range all {
				if v.Prefix == r.Prefix && v.Protocol == 233 && v.Table == 254 {
					managed := false
					if rec, ok := owned[r.Prefix]; ok && !rec.Rule.NoRoute {
						managed = true
					}
					for _, a := range data.Attributes {
						if a.Route.Prefix == v.Prefix && a.Route.Table == v.Table {
							managed = true
						}
					}
					if !managed {
						issue(r, "noroute would remove external proto 233 route")
					}
				}
			}
			if protected(r, failed) {
				continue
			}
			if matches[0].Algorithm != "brutal" || matches[0].Locked != r.Lock {
				plan.RouteRepair = append(plan.RouteRepair, matches[0])
			}
		}
		if c, ok := now[r.Prefix]; !ok || recovery || data.Recovery == "clearing" {
			plan.Added = append(plan.Added, r)
		} else if rec, ours := owned[r.Prefix]; !c.Matches(r) || (ours && !rec.Rule.Equal(r)) {
			plan.Updated = append(plan.Updated, r)
		}
	}
	for _, rec := range data.Records {
		if _, ok := targets[rec.Rule.Prefix]; !ok && !protected(rec.Rule, failed) {
			plan.Removed = append(plan.Removed, rec.Rule)
		}
	}
	wantedOrder := []netip.Prefix{}
	for _, r := range desired {
		if !protected(r, failed) {
			wantedOrder = append(wantedOrder, r.Prefix)
		}
	}
	actualOrder := []netip.Prefix{}
	for _, r := range current {
		if _, ok := targets[r.Prefix]; ok {
			actualOrder = append(actualOrder, r.Prefix)
		}
	}
	if len(actualOrder) == len(wantedOrder) {
		for i := range actualOrder {
			if actualOrder[i] != wantedOrder[i] {
				plan.Reorder = true
				break
			}
		}
	}
	for _, a := range data.Attributes {
		if _, wanted := targets[a.Route.Prefix]; !wanted {
			keep := false
			for _, rec := range data.Records {
				if rec.Rule.Prefix == a.Route.Prefix && protected(rec.Rule, failed) {
					keep = true
				}
			}
			if !keep {
				plan.RouteCleanup = append(plan.RouteCleanup, a)
			}
		}
	}
	different := len(plan.RouteCleanup)+len(plan.Added)+len(plan.Updated)+len(plan.Removed)+len(plan.RouteRepair) > 0 || plan.Reorder || plan.Recovery
	if !write {
		if len(plan.Issues) > 0 {
			return plan, 7, nil
		}
		if different {
			return plan, 3, nil
		}
		return plan, 0, nil
	}
	raw, _ := os.ReadFile(m.Config.Path)
	data.Generation = state.Generation(raw)
	if recovery || data.Recovery == "clearing" {
		data.Recovery = "clearing"
		if e = state.Save(m.StateDir, data); e != nil {
			return plan, 1, e
		}
		if e = m.note("flush"); e != nil {
			return plan, 1, e
		}
		if e = m.Control.Exec(ctx, "flush"); e != nil {
			return plan, 1, e
		}
		for _, v := range all {
			if v.Algorithm == "brutal" {
				if e = m.change(ctx, v, "", false); e != nil {
					return plan, 1, fmt.Errorf("recovery cleanup: %w", e)
				}
			}
		}
		data = state.Data{Version: 1, Recovery: "rebuilding", Generation: data.Generation}
		if e = state.Save(m.StateDir, data); e != nil {
			return plan, 1, e
		}
		owned = map[netip.Prefix]state.Record{}
	}
	if e = state.Save(m.StateDir, data); e != nil {
		return plan, 1, e
	}
	additions := map[netip.Prefix]bool{}
	for _, r := range plan.Added {
		additions[r.Prefix] = true
	}
	for _, r := range plan.Updated {
		additions[r.Prefix] = true
	}
	verified := map[netip.Prefix]bool{}
	for _, r := range desired {
		if protected(r, failed) {
			continue
		}
		if additions[r.Prefix] {
			data.Pending = append(data.Pending, state.Record{r})
			if e = state.Save(m.StateDir, data); e != nil {
				return plan, 1, e
			}
			if e = m.note("add " + r.Prefix.String()); e != nil {
				return plan, 1, e
			}
			if e = m.Control.Add(ctx, r); e != nil {
				issue(r, e.Error())
				continue
			}
		}
		cur, er := brutal.Read(m.ProcPath)
		if er != nil {
			return plan, 6, er
		}
		ok := false
		for _, v := range cur {
			if v.Matches(r) {
				ok = true
			}
		}
		if !ok {
			issue(r, "rule verification failed")
			continue
		}
		if v, policy := selected[r.Prefix]; policy {
			if v.Algorithm != "brutal" || v.Locked != r.Lock {
				a := state.Attribute{Route: v, AddedAlgorithm: v.Algorithm != "brutal", ChangedLock: v.Locked != r.Lock}
				a.Route.Algorithm = "brutal"
				a.Route.Locked = r.Lock
				merged := false
				for i := range data.Attributes {
					prev := &data.Attributes[i]
					if prev.Route.Prefix == a.Route.Prefix && prev.Route.Table == a.Route.Table && prev.Route.Interface == a.Route.Interface {
						a.AddedAlgorithm = a.AddedAlgorithm || prev.AddedAlgorithm
						a.ChangedLock = a.ChangedLock || prev.ChangedLock
						*prev = a
						merged = true
						break
					}
				}
				if !merged {
					data.Attributes = append(data.Attributes, a)
				}
				if er = state.Save(m.StateDir, data); er != nil {
					return plan, 1, er
				}
				if er = m.change(ctx, v, "brutal", r.Lock); er != nil {
					issue(r, er.Error())
					continue
				}
			}
		}
		verified[r.Prefix] = true
		_, wasOwned := owned[r.Prefix]
		if wasOwned || additions[r.Prefix] {
			owned[r.Prefix] = state.Record{r}
			data.Records = nil
			for _, rec := range owned {
				data.Records = append(data.Records, rec)
			}
			if er = state.Save(m.StateDir, data); er != nil {
				return plan, 1, er
			}
		}
	}
	for _, r := range desired {
		if !verified[r.Prefix] {
			for _, src := range r.Sources {
				failed[src] = true
			}
		}
	}
	for _, r := range plan.Removed {
		if protected(r, failed) {
			continue
		}
		if e = m.note("delete " + r.Prefix.String()); e != nil {
			return plan, 1, e
		}
		if e = m.Control.Delete(ctx, r.Prefix); e != nil {
			issue(r, e.Error())
			continue
		}
		delete(owned, r.Prefix)
	}
	data.Records = nil
	for _, v := range owned {
		data.Records = append(data.Records, v)
	}
	remaining := []state.Attribute{}
	for _, a := range data.Attributes {
		_, ownedNeeded := owned[a.Route.Prefix]
		_, desiredNeeded := targets[a.Route.Prefix]
		if ownedNeeded || desiredNeeded {
			remaining = append(remaining, a)
			continue
		}
		if e = m.cleanup(ctx, a); e != nil {
			remaining = append(remaining, a)
			plan.Issues = append(plan.Issues, config.Issue{-1, "route", e.Error(), ""})
		}
	}
	data.Attributes = remaining
	data.Recovery = ""
	pending := []state.Record{}
	for _, rec := range data.Pending {
		if protected(rec.Rule, failed) {
			pending = append(pending, rec)
		}
	}
	data.Pending = pending
	if e = state.Save(m.StateDir, data); e != nil {
		return plan, 1, e
	}
	final, er := brutal.Read(m.ProcPath)
	if er != nil {
		return plan, 6, er
	}
	actual := []netip.Prefix{}
	for _, v := range final {
		if wanted, ok := targets[v.Prefix]; ok && !protected(wanted, failed) {
			if !v.Matches(wanted) {
				issue(wanted, "final rule verification failed")
			}
			actual = append(actual, v.Prefix)
		}
	}
	if len(actual) == len(wantedOrder) {
		for i := range actual {
			if actual[i] != wantedOrder[i] {
				plan.Reorder = true
				break
			}
		}
	}
	if len(plan.Issues) > 0 || plan.Reorder {
		return plan, 7, nil
	}
	return plan, 0, nil
}
func (m *Manager) change(ctx context.Context, r route.Route, algo string, lock bool) error {
	ctx, cancel := context.WithTimeout(ctx, m.Config.Durations.Route)
	defer cancel()
	if e := m.note("route " + r.Prefix.String()); e != nil {
		return e
	}
	if e := m.Routes.Change(ctx, r, algo, lock); e != nil {
		return e
	}
	all, e := m.Routes.Read(ctx)
	if e != nil {
		return e
	}
	for _, v := range all {
		if v.Prefix == r.Prefix && v.Table == r.Table && v.Index == r.Index && v.Algorithm == algo && v.Locked == lock {
			return nil
		}
	}
	return fmt.Errorf("route verification failed")
}
func (m *Manager) cleanup(ctx context.Context, a state.Attribute) error {
	all, e := m.Routes.Read(ctx)
	if e != nil {
		return e
	}
	for _, r := range all {
		if r.Prefix == a.Route.Prefix && r.Table == a.Route.Table && r.Interface == a.Route.Interface {
			if r.Algorithm != a.Route.Algorithm || r.Locked != a.Route.Locked {
				return nil
			}
			algo, lock := r.Algorithm, r.Locked
			if a.AddedAlgorithm {
				algo = ""
			}
			if a.ChangedLock {
				lock = false
			}
			return m.change(ctx, r, algo, lock)
		}
	}
	return nil
}
func (m *Manager) Flush(ctx context.Context, dry bool) (Plan, int, error) {
	unlock, e := state.Lock(m.LockPath)
	if e != nil {
		return Plan{}, 1, e
	}
	defer unlock()
	cur, e := brutal.Read(m.ProcPath)
	if e != nil {
		return Plan{}, 6, e
	}
	data, broken, e := state.Load(m.StateDir)
	if e != nil {
		return Plan{}, 1, e
	}
	p := Plan{Recovery: true, RouteCleanup: data.Attributes}
	for _, v := range cur {
		p.Removed = append(p.Removed, rules.Rule{Prefix: v.Prefix})
	}
	if broken {
		return p, 1, fmt.Errorf("management record damaged; use configured apply recovery")
	}
	if dry {
		return p, 0, nil
	}
	if e = m.note("flush"); e != nil {
		return p, 1, e
	}
	if e = m.Control.Exec(ctx, "flush"); e != nil {
		return p, 1, e
	}
	left := []state.Attribute{}
	for _, a := range data.Attributes {
		if e = m.cleanup(ctx, a); e != nil {
			left = append(left, a)
			p.Issues = append(p.Issues, config.Issue{-1, "route", e.Error(), ""})
		}
	}
	data.Records = nil
	data.Pending = nil
	data.Attributes = left
	data.Recovery = ""
	if e = state.Save(m.StateDir, data); e != nil {
		return p, 1, e
	}
	if len(p.Issues) > 0 {
		return p, 7, nil
	}
	return p, 0, nil
}

func (m *Manager) note(message string) error {
	if m.Before != nil {
		return m.Before(message)
	}
	return nil
}
