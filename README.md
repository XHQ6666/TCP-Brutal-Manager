# TCP Brutal Manager

Linux TCP Brutal rule manager — **1.0.0-Beta (Pre-Release)**.

Configuration-driven management through official `brutalctl` (baseline v2.0.1), live System/DoH/DoT/DoQ resolution, IPv4-first numeric ordering, partial-failure isolation, managed-source tracking, and continuous maintenance.

## Requirements

Linux, TCP Brutal v2.0.1 module, `brutalctl`, iproute2, appropriate permissions. Build with the Go version in go.mod. No kernel module or congestion-control change is made by installing this binary.

## Usage

```sh
./tcp-brutal-config -h
./tcp-brutal-config check -c config.json
./tcp-brutal-config apply -c config.json -n
./tcp-brutal-config apply -c config.json
./tcp-brutal-config run -c config.json
./tcp-brutal-config list
./tcp-brutal-config flush -n
```

Run shutdown preserves state. Stop run before flushing if you want to keep the rules cleared. **Flush clears all Brutal rules, including rules from other tools.** State-record corruption with an existing initialization marker triggers the configured global recovery policy: clear rules and brutal attributes across routing tables, then rebuild valid configuration.

Rate accepts a number (Mbps) or a bit-rate string: 200, "200Mbps", "2.5Gbps". It must convert exactly to a positive integer Mbps. The Beta input limit is uint32 Mbps; no byte-unit rates are accepted.

`cidr` defaults to IPv4 /32 or IPv6 /128. Domain rules also accept `{ "v4": 24, "v6": 64 }`. Ordinary routes are delegated to brutalctl; explicit `table` and/or `interface` target an existing unique exact route, never create a peer route.

Configuration events and Netlink route/link events are watched. DNS uses `settings.dns.query_interval` (60s default). Brutal rules use independent polling. In-process successful DNS results support non-DNS reconciliation; no cross-run DNS cache is reused.

State is stored in `/var/lib/tcp-brutal-config`, logs default to `/var/log/tcp-brutal-config.log`, configuration defaults to `/etc/tcp-brutal-config/config.json`. Help uses the executable's invoked filename.

## Validation and Beta limits

Unit and fake-controller integration tests verify defaults, matching, CIDR, rates, ordering, deduplication, parser behavior, state checksums, dry-run and local-failure isolation. Linux-specific Netlink metric preservation tests run in Linux CI. Local macOS checks do not validate a live TCP Brutal kernel, real Tailscale route rebuilding, or interoperability with public encrypted DNS servers.

The Beta must be tested on an isolated Linux host before production use. Full kernel-order repair is deferred if it would require destructive rebuilding. Source identity uses config path, source value and duplicate occurrence, so invalid edits that cannot identify an old source do not protect it. Route read/modify/write is not atomic against other route managers.

Detailed confirmed behavior is in [PLAN](docs/PLAN.md); operational procedure in [SOP](docs/SOP.md). CLI status currently returns the same live synchronization plan as diff, with the same exit semantics.

## Build and test

```sh
go test -race ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o tcp-brutal-config ./cmd/tcp-brutal-config
```

Exit codes: 0 success, 1 runtime error, 2 global configuration/CLI error, 3 difference (read-only commands), 4 verification error, 5 permission error, 6 kernel/dependency error, 7 partial success/degraded. Apply dry-run returns 3 for ordinary differences. Run's normal signal shutdown returns 0, not a health verdict for previous rounds.
