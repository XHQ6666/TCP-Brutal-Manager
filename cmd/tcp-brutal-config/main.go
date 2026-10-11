package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/brutal"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/config"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/manager"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/state"
	"github.com/fsnotify/fsnotify"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var version = "1.0.0-Beta"
var commit = "dev"
var buildTime = "unknown"

func main() { os.Exit(execute(os.Args[1:])) }
func help(command string) {
	p := filepath.Base(os.Args[0])
	fmt.Printf("Usage: %s <command> [options]\n\nCommands:\n  run      Continuously maintain DNS, rules and policy routes\n  apply    Synchronize managed rules\n  check    Validate configuration and inspect differences\n  diff     Show differences and skipped rules\n  list     Show current kernel rules\n  status   Show configuration and synchronization status\n  flush    Clear ALL Brutal rules and matching managed route attributes\n  version  Show version and build information\n  help     Show help for a command\n\nOptions:\n  -c, --config <path>  Default /etc/tcp-brutal-config/config.json\n  -h, --help           Show help\n  -v, --version        Show version\n  -n, --dry-run        apply/flush only; no system or state writes\n\nRun DNS query interval: JSON settings.dns.query_interval (default 60s).\nRun shutdown preserves rules. Stop run BEFORE flush to keep rules cleared.\n", p)
	if command != "" {
		fmt.Printf("\nCommand: %s\n", command)
	}
}
func execute(args []string) int {
	command := ""
	path := "/etc/tcp-brutal-config/config.json"
	dry := false
	configSeen := false
	for i := 0; i < len(args); i++ {
		s := args[i]
		switch s {
		case "-h", "--help":
			help(command)
			return 0
		case "-v", "--version":
			fmt.Printf("%s %s commit=%s built=%s\n", filepath.Base(os.Args[0]), version, commit, buildTime)
			return 0
		case "-c", "--config":
			if configSeen || i+1 == len(args) {
				fmt.Fprintln(os.Stderr, "config requires one path")
				return 2
			}
			i++
			path = args[i]
			configSeen = true
		case "-n", "--dry-run":
			dry = true
		default:
			if strings.HasPrefix(s, "--config=") {
				if configSeen {
					return 2
				}
				path = strings.TrimPrefix(s, "--config=")
				configSeen = true
			} else if command == "help" {
				help(s)
				return 0
			} else if command == "" && !strings.HasPrefix(s, "-") {
				command = s
			} else {
				fmt.Fprintln(os.Stderr, "unknown argument", s)
				return 2
			}
		}
	}
	if command == "" || command == "help" {
		help("")
		return 0
	}
	if command == "version" {
		fmt.Printf("%s commit=%s built=%s\n", version, commit, buildTime)
		return 0
	}
	switch command {
	case "run", "apply", "check", "diff", "list", "status", "flush":
	default:
		fmt.Fprintln(os.Stderr, "unknown command", command)
		return 2
	}
	if dry && command != "apply" && command != "flush" {
		fmt.Fprintln(os.Stderr, "dry-run only supported for apply/flush")
		return 2
	}
	if runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "Linux required")
		return 6
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if command == "list" {
		r, e := brutal.Read("/proc/net/tcp_brutal/rules")
		if e != nil {
			return fail(e, 6)
		}
		printJSON(r)
		return 0
	}
	var c *config.Config
	var e error
	if command == "flush" {
		c, e = config.Parse([]byte(`{"log":{"enabled":false},"rules":[{"ip":"192.0.2.1","rate":1}]}`), path)
	} else {
		c, e = config.Load(path)
	}
	if e != nil {
		return fail(e, 2)
	}
	logger, e := openLog(c)
	if e != nil {
		return fail(e, 1)
	}
	defer logger.Close()
	m := manager.New(c)
	m.Before = func(msg string) error { return logger.Write("INFO", "apply", msg) }
	if command == "run" {
		return run(ctx, m, logger)
	}
	var p manager.Plan
	var code int
	if command == "flush" {
		p, code, e = m.Flush(ctx, dry)
	} else {
		p, code, e = m.Sync(ctx, command == "apply" && !dry, true)
	}
	printJSON(p)
	if e != nil {
		logger.Write("ERROR", "apply", e.Error())
		return fail(e, code)
	}
	if e = logger.Write("INFO", "apply", fmt.Sprintf("command=%s exit=%d issues=%d", command, code, len(p.Issues))); e != nil {
		return fail(e, 1)
	}
	return code
}
func fail(e error, code int) int {
	fmt.Fprintln(os.Stderr, e)
	if errors.Is(e, os.ErrPermission) {
		return 5
	}
	if code == 0 {
		return 1
	}
	return code
}
func printJSON(v any) { enc := json.NewEncoder(os.Stdout); enc.SetIndent("", "  "); enc.Encode(v) }

type Logger struct {
	file  *os.File
	level string
}

func openLog(c *config.Config) (*Logger, error) {
	l := &Logger{level: c.Log.Level}
	if *c.Log.Enabled {
		f, e := os.OpenFile(c.Log.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if e != nil {
			return nil, e
		}
		l.file = f
		if e = l.Write("INFO", "log", "path="+c.Log.Path); e != nil {
			f.Close()
			return nil, e
		}
	}
	return l, nil
}
func (l *Logger) Write(level, component, msg string) error {
	if l.file == nil {
		return nil
	}
	if l.level == "error" && level != "ERROR" {
		return nil
	}
	if l.level == "warning" && level == "INFO" {
		return nil
	}
	_, e := fmt.Fprintf(l.file, "%s [%s] [%s] %s\n", time.Now().Format(time.RFC3339), level, component, strings.ReplaceAll(msg, "\n", " "))
	return e
}
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
func run(ctx context.Context, m *manager.Manager, l *Logger) int {
	m.Before = func(msg string) error { return l.Write("INFO", "apply", msg) }
	release, e := state.Lock("/run/tcp-brutal-config-run.lock")
	if e != nil {
		return fail(e, 1)
	}
	defer release()
	watcher, e := fsnotify.NewWatcher()
	if e != nil {
		return fail(e, 1)
	}
	defer watcher.Close()
	watcher.Add(filepath.Dir(m.Config.Path))
	routeEvents := make(chan struct{}, 1)
	watchRoutes := m.Routes
	watchInterval := m.Config.Durations.Resync
	go func() {
		for ctx.Err() == nil {
			e := watchRoutes.Watch(ctx, routeEvents)
			if e != nil && ctx.Err() == nil {
				select {
				case routeEvents <- struct{}{}:
				default:
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(watchInterval):
			}
		}
	}()
	query := time.NewTimer(0)
	defer query.Stop()
	poll := time.NewTimer(m.Config.Durations.Poll)
	defer poll.Stop()
	resync := time.NewTimer(m.Config.Durations.Resync)
	defer resync.Stop()
	rescan := time.NewTimer(m.Config.Durations.Rescan)
	defer rescan.Stop()
	configDebounce := time.NewTimer(time.Hour)
	configDebounce.Stop()
	defer configDebounce.Stop()
	routeDebounce := time.NewTimer(time.Hour)
	routeDebounce.Stop()
	defer routeDebounce.Stop()
	content, _ := os.ReadFile(m.Config.Path)
	generation := state.Generation(content)
	sync := func(refresh bool) {
		if e := l.Write("INFO", "apply", fmt.Sprintf("start refresh_dns=%t", refresh)); e != nil {
			fmt.Fprintln(os.Stderr, e)
			return
		}
		p, code, e := m.Sync(ctx, true, refresh)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			l.Write("ERROR", "apply", e.Error())
		} else {
			l.Write("INFO", "apply", fmt.Sprintf("exit=%d issues=%d", code, len(p.Issues)))
			for _, issue := range p.Issues {
				l.Write("WARNING", issue.Component, issue.Error)
			}
		}
	}
	reload := func() {
		b, e := os.ReadFile(m.Config.Path)
		if e != nil {
			l.Write("ERROR", "config", e.Error())
			return
		}
		gen := state.Generation(b)
		if gen == generation {
			return
		}
		next, e := config.Parse(b, m.Config.Path)
		if e != nil {
			l.Write("ERROR", "config", e.Error())
			return
		}
		newLog, e := openLog(next)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return
		}
		old := m.Config
		nextM := manager.New(next)
		dnsChanged := fmt.Sprint(old.DNS, old.DNSRules) != fmt.Sprint(next.DNS, next.DNSRules)
		nextM.Cache = m.Cache
		for _, raw := range next.Rules {
			var rule config.Rule
			if config.Strict(raw, &rule) == nil && rule.Domain != "" {
				if _, ok := nextM.Cache[rule.Domain]; !ok {
					dnsChanged = true
				}
			}
		}
		m = nextM
		l.Close()
		l = newLog
		m.Before = func(msg string) error { return l.Write("INFO", "apply", msg) }
		generation = gen
		query.Reset(next.Durations.Query)
		poll.Reset(next.Durations.Poll)
		resync.Reset(next.Durations.Resync)
		rescan.Reset(next.Durations.Rescan)
		sync(dnsChanged)
	}
	for {
		select {
		case <-ctx.Done():
			l.Close()
			return 0
		case <-query.C:
			sync(true)
			query.Reset(m.Config.Durations.Query)
		case <-poll.C:
			sync(false)
			poll.Reset(m.Config.Durations.Poll)
		case <-resync.C:
			sync(false)
			resync.Reset(m.Config.Durations.Resync)
		case <-rescan.C:
			reload()
			rescan.Reset(m.Config.Durations.Rescan)
		case ev := <-watcher.Events:
			if filepath.Clean(ev.Name) == m.Config.Path {
				configDebounce.Reset(m.Config.Durations.ConfigDebounce)
			}
		case er := <-watcher.Errors:
			if er != nil {
				l.Write("WARNING", "config", er.Error())
				reload()
			}
		case <-configDebounce.C:
			reload()
		case <-routeEvents:
			routeDebounce.Reset(m.Config.Durations.RouteDebounce)
		case <-routeDebounce.C:
			sync(false)
		}
	}
}
