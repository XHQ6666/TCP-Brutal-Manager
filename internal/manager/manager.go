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
type RouteDetail struct {
	BusinessPrefix netip.Prefix `json:"business_prefix"`
	Route          route.Route  `json:"route"`
	Strict         bool         `json:"route_strict"`
	Action         string       `json:"action"`
}
type RouteSummary struct {
	Matched   int `json:"matched"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}
type Plan struct {
	RouteDetails []RouteDetail     `json:"route_details,omitempty"`
	RouteSummary RouteSummary      `json:"route_summary"`
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
			issues = append(issues, config.Issue{Index: i, Component: "rules", Error: e.Error(), Source: source})
			continue
		}
		candidate = append(candidate, expanded...)
	}
	final, conflicts := rules.Finalize(candidate)
	for src := range conflicts {
		failed[src] = true
		issues = append(issues, config.Issue{Index: -1, Component: "rules", Error: "same-prefix parameter conflict", Source: src})
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
func protectedSources(sources []string, failed map[string]bool) []string {
	out := []string{}
	for _, source := range sources {
		if failed[source] {
			out = append(out, source)
		}
	}
	return out
}
func mergeSources(a, b []string) []string {
	seen := map[string]bool{}
	out := append([]string{}, a...)
	for _, source := range a {
		seen[source] = true
	}
	for _, source := range b {
		if !seen[source] {
			out = append(out, source)
			seen[source] = true
		}
	}
	return out
}
func sameRouteIdentity(a, b route.Route) bool {
	return route.SameIdentity(a, b)
}
func routeAttributeNeeded(a state.Attribute, selected map[netip.Prefix][]route.Route, records []state.Record, failed map[string]bool) bool {
	if a.Unresolved {
		return true
	}
	for _, list := range selected {
		for _, current := range list {
			if sameRouteIdentity(a.Route, current) {
				return true
			}
		}
	}
	for _, record := range records {
		if protected(record.Rule, failed) && (record.Rule.Table != nil || record.Rule.Interface != "") && route.Matches(a.Route, record.Rule) {
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
	if data.Version == 1 {
		for i := range data.Attributes {
			a := &data.Attributes[i]
			matches := []route.Route{}
			for _, v := range all {
				if v.Prefix == a.Route.Prefix && v.Table == a.Route.Table && v.Index == a.Route.Index && v.Protocol == a.Route.Protocol {
					matches = append(matches, v)
				}
			}
			if len(matches) == 1 {
				modern := matches[0]
				modern.Algorithm = a.Route.Algorithm
				modern.Locked = a.Route.Locked
				a.Route = modern
			} else if len(matches) > 1 {
				a.Unresolved = true
			}
		}
	}
	for _, a := range data.Attributes {
		if a.Unresolved {
			plan.Issues = append(plan.Issues, config.Issue{Index: -1, Component: "state", Error: "legacy route identity requires review"})
		}
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
	selected := map[netip.Prefix][]route.Route{}
	issue := func(r rules.Rule, msg string) {
		for _, s := range r.Sources {
			failed[s] = true
		}
		plan.Issues = append(plan.Issues, config.Issue{Index: -1, Component: "apply", Error: msg, Source: strings.Join(r.Sources, ",")})
	}
	for i := range desired {
		r := desired[i]
		targets[r.Prefix] = r
		if previous, ok := owned[r.Prefix]; ok && protected(previous.Rule, failed) {
			if !previous.Rule.Equal(r) {
				issue(r, "existing same-prefix state is protected by a failed source")
				continue
			}
			r.Sources = mergeSources(r.Sources, protectedSources(previous.Rule.Sources, failed))
			desired[i] = r
			targets[r.Prefix] = r
		}
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
				} else if route.InRange(v, r) && len(v.NextHops) > 0 && r.Interface != "" {
					plan.Issues = append(plan.Issues, config.Issue{Index: -1, Component: "route", Error: "multipath interfaces do not all match", Source: strings.Join(r.Sources, ",")})
					plan.RouteDetails = append(plan.RouteDetails, RouteDetail{r.Prefix, v, r.RouteStrict, "skip: interface mismatch"})
				}
			}
			if len(matches) == 0 {
				plan.Issues = append(plan.Issues, config.Issue{Index: -1, Component: "route", Error: "no matching policy route", Source: strings.Join(r.Sources, ",")})
			}
			selected[r.Prefix] = matches
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
				delete(selected, r.Prefix)
				continue
			}
			for _, v := range matches {
				if v.Algorithm != "brutal" || v.Locked != r.Lock {
					plan.RouteRepair = append(plan.RouteRepair, v)
				}
			}
		}
		if c, ok := now[r.Prefix]; !ok || recovery || data.Recovery == "clearing" {
			plan.Added = append(plan.Added, r)
		} else if rec, ours := owned[r.Prefix]; !c.Matches(r) || (ours && !rec.Rule.Equal(r)) {
			plan.Updated = append(plan.Updated, r)
		}
	}
	// Resolve overlapping policies once per route, with the most specific policy first.
	resolveSelections(desired, selected, failed)
	plan.RouteRepair = nil
	for _, r := range desired {
		for _, v := range selected[r.Prefix] {
			action := "unchanged"
			if v.Algorithm != "brutal" || v.Locked != r.Lock {
				action = "repair"
				plan.RouteRepair = append(plan.RouteRepair, v)
			}
			plan.RouteDetails = append(plan.RouteDetails, RouteDetail{r.Prefix, v, r.RouteStrict, action})
			plan.RouteSummary.Matched++
		}
	}
	for _, rec := range data.Records {
		if _, ok := targets[rec.Rule.Prefix]; !ok && !protected(rec.Rule, failed) {
			plan.Removed = append(plan.Removed, rec.Rule)
		}
	}
	wantedOrder := []netip.Prefix{}
	orderScope := map[netip.Prefix]bool{}
	for _, r := range desired {
		if protected(r, failed) {
			continue
		}
		// Existing external rules may be reused without becoming managed.
		// New rules and recovery rebuilds belong to the managed order.
		if _, ours := owned[r.Prefix]; !ours && !plan.Recovery {
			if _, exists := now[r.Prefix]; exists {
				continue
			}
		}
		orderScope[r.Prefix] = true
		wantedOrder = append(wantedOrder, r.Prefix)
	}
	actualOrder := []netip.Prefix{}
	for _, r := range current {
		if orderScope[r.Prefix] {
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
		if !routeAttributeNeeded(a, selected, data.Records, failed) {
			plan.RouteCleanup = append(plan.RouteCleanup, a)
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
		all, e = m.Routes.Read(ctx)
		if e != nil {
			return plan, 1, fmt.Errorf("recovery route read: %w", e)
		}
		data.Recovery = "cleaning"
		data.Attributes = nil
		for _, v := range all {
			if v.Algorithm == "brutal" {
				data.Attributes = append(data.Attributes, state.Attribute{Route: v, AddedAlgorithm: true, ChangedLock: true})
			}
		}
		if e = state.Save(m.StateDir, data); e != nil {
			return plan, 1, e
		}
	}
	if data.Recovery == "cleaning" {
		remainingRecovery := []state.Attribute{}
		for _, a := range data.Attributes {
			if er := m.cleanup(ctx, a); er != nil {
				remainingRecovery = append(remainingRecovery, a)
				plan.Issues = append(plan.Issues, config.Issue{Index: -1, Component: "route", Error: er.Error()})
			}
		}
		data.Attributes = remainingRecovery
		if e = state.Save(m.StateDir, data); e != nil {
			return plan, 1, e
		}
		if len(remainingRecovery) > 0 {
			return plan, 7, nil
		}
		data.Recovery = "rebuilding"
		if e = state.Save(m.StateDir, data); e != nil {
			return plan, 1, e
		}
		owned = map[netip.Prefix]state.Record{}
	}
	if plan.Recovery && data.Recovery == "rebuilding" {
		all, e = m.Routes.Read(ctx)
		if e != nil {
			return plan, 1, fmt.Errorf("recovery route reread: %w", e)
		}
		selected = map[netip.Prefix][]route.Route{}
		plan.RouteRepair = nil
		for _, r := range desired {
			if protected(r, failed) || (r.Table == nil && r.Interface == "") {
				continue
			}
			matches := []route.Route{}
			for _, v := range all {
				if route.Matches(v, r) {
					matches = append(matches, v)
				} else if route.InRange(v, r) && len(v.NextHops) > 0 && r.Interface != "" {
					plan.Issues = append(plan.Issues, config.Issue{Index: -1, Component: "route", Error: "multipath interfaces do not all match", Source: strings.Join(r.Sources, ",")})
					plan.RouteDetails = append(plan.RouteDetails, RouteDetail{r.Prefix, v, r.RouteStrict, "skip: interface mismatch"})
				}
			}
			selected[r.Prefix] = matches
			for _, v := range matches {
				if v.Algorithm != "brutal" || v.Locked != r.Lock {
					plan.RouteRepair = append(plan.RouteRepair, v)
				}
			}
		}
	}
	resolveSelections(desired, selected, failed)
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
			data.Pending = append(data.Pending, state.Record{Rule: r})
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
		routeOK := true
		for _, v := range selected[r.Prefix] {
			owner := r
			for _, candidate := range desired {
				if !protected(candidate, failed) && candidate.Prefix.Bits() > owner.Prefix.Bits() {
					for _, cv := range selected[candidate.Prefix] {
						if sameRouteIdentity(cv, v) {
							owner = candidate
						}
					}
				}
			}
			if owner.Prefix != r.Prefix {
				continue
			}
			if v.Algorithm != "brutal" || v.Locked != owner.Lock {
				a := state.Attribute{Sources: routeSources(v, desired, failed), Route: v, AddedAlgorithm: v.Algorithm != "brutal", ChangedLock: v.Locked != owner.Lock}
				a.Route.Algorithm = "brutal"
				a.Route.Locked = owner.Lock
				merged := false
				for i := range data.Attributes {
					prev := &data.Attributes[i]
					if sameRouteIdentity(prev.Route, a.Route) {
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
				if er = m.change(ctx, v, "brutal", owner.Lock); er != nil {
					plan.Issues = append(plan.Issues, config.Issue{Index: -1, Component: "route", Error: er.Error(), Source: strings.Join(r.Sources, ",")})
					routeOK = false
					plan.RouteSummary.Failed++
					continue
				}
			}
			plan.RouteSummary.Succeeded++
		}
		verified[r.Prefix] = routeOK
		_, wasOwned := owned[r.Prefix]
		if wasOwned || additions[r.Prefix] {
			owned[r.Prefix] = state.Record{Rule: r}
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
		if routeAttributeNeeded(a, selected, data.Records, failed) {
			a.Sources = mergeSources(routeSources(a.Route, desired, failed), protectedSources(a.Sources, failed))
			remaining = append(remaining, a)
			continue
		}
		if e = m.cleanup(ctx, a); e != nil {
			remaining = append(remaining, a)
			plan.Issues = append(plan.Issues, config.Issue{Index: -1, Component: "route", Error: e.Error()})
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
			if orderScope[v.Prefix] {
				actual = append(actual, v.Prefix)
			}
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
		if sameRouteIdentity(v, r) && v.Algorithm == algo && v.Locked == lock {
			return nil
		}
	}
	return fmt.Errorf("route verification failed")
}
func (m *Manager) cleanup(ctx context.Context, a state.Attribute) error {
	if a.Unresolved {
		return fmt.Errorf("legacy route identity requires review")
	}
	all, e := m.Routes.Read(ctx)
	if e != nil {
		return e
	}
	for _, r := range all {
		if !sameRouteIdentity(r, a.Route) {
			continue
		}
		algo, lock := r.Algorithm, r.Locked
		changed := false
		if a.AddedAlgorithm && r.Algorithm == a.Route.Algorithm {
			algo = ""
			changed = true
		}
		if a.ChangedLock && r.Locked == a.Route.Locked {
			lock = false
			changed = true
		}
		if changed {
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
			p.Issues = append(p.Issues, config.Issue{Index: -1, Component: "route", Error: e.Error()})
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

func resolveSelections(desired []rules.Rule, selected map[netip.Prefix][]route.Route, failed map[string]bool) {
	for _, r := range desired {
		keep := []route.Route{}
		for _, v := range selected[r.Prefix] {
			winner := true
			for _, other := range desired {
				if other.Prefix.Bits() > r.Prefix.Bits() && !protected(other, failed) && (other.Table != nil || other.Interface != "") && route.Matches(v, other) {
					winner = false
					break
				}
			}
			if winner {
				keep = append(keep, v)
			}
		}
		selected[r.Prefix] = keep
	}
}
func routeSources(v route.Route, desired []rules.Rule, failed map[string]bool) []string {
	out := []string{}
	for _, r := range desired {
		if !protected(r, failed) && (r.Table != nil || r.Interface != "") && route.Matches(v, r) {
			out = mergeSources(out, r.Sources)
		}
	}
	return out
}
