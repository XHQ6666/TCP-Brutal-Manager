# TCP Brutal Manager

TCP Brutal Manager **1.0.0** manages Linux TCP Brutal rules from a JSON configuration. It uses official `brutalctl` for rule operations and ordinary routes, and Netlink for existing policy-route attributes. Run mode watches configuration and route/link events, polls rules, and refreshes domain records on an independent DNS timer.

## Requirements

- Linux with TCP Brutal **v2.0.1**, the supported/tested baseline.
- Official `brutalctl`, iproute2 and permissions to read or modify rules/routes as appropriate.
- For source builds, the Go version specified in `go.mod`.

Linux packages are provided for amd64, arm64, 386, armv5, armv6 and armv7. Each archive contains the binary `tcp-brutal-config`, example configuration, systemd template, release notes and user documentation. The CLI help name follows the executable's invoked filename.

## Quick start

Verify the archive against `SHA256SUMS`, extract it, and edit `config.example.json` with your own destinations and rates. The example destination is a documentation address, not a production endpoint.

```sh
manager_bin=./tcp-brutal-config
manager_config=./config.example.json
"$manager_bin" version
"$manager_bin" check -c "$manager_config"
"$manager_bin" apply -c "$manager_config" -n
# After reviewing the plan, apply once or run continuously:
"$manager_bin" apply -c "$manager_config"
"$manager_bin" run -c "$manager_config"
```

`check`, `diff` and `status` query DNS and inspect the current plan without modifying rules. Exit **3** means a difference; **7** means partial/degraded results. SIGTERM stops Run without clearing rules.

**Flush clears all Brutal rules, including external rules.** Stop Run first if you want them to remain cleared. A damaged/missing management record with an initialization marker causes Apply to rebuild globally and remove `brutal` route attributes across tables. Read the recovery procedure before deleting or restoring state files.

## Documentation

- [Configuration reference](docs/CONFIGURATION.md): rates, CIDR, resolvers, DNS selection and settings.
- [CLI reference](docs/CLI.md): commands, options, outputs and exit codes.
- [Operations guide](docs/SOP.md): installation, Tailscale, continuous maintenance, Flush and recovery.
- [Release notes](RELEASE_NOTES.md): changes, validation and limits.

Default configuration is `/etc/tcp-brutal-config/config.json`, state is `/var/lib/tcp-brutal-config`, and the file log is `/var/log/tcp-brutal-config.log`. Keep a stable configuration path: source identity includes that path.

## Behavior and validation

Rules are ordered IPv4 first, then by normalized network address and CIDR length. Identical rules retain all sources; conflicting parameters for the same Prefix are isolated. Normal Apply preserves external rules and does not use destructive Flush to repair order. Ordinary routes are delegated to brutalctl; policy rules maintain all existing routes fully contained in their CIDR scope. Set `route_strict: true` to match only exact prefixes. Missing routes report degradation while the Brutal rule remains maintained.

Tests include unit/fake-controller integration, race checks and Linux Netlink tests. Linux amd64 live tests with TCP Brutal 2.0.1 covered DNS System/DoH/DoT/DoQ, rule updates, failure isolation, configuration reload, route/link repair, ownership, Flush and state recovery. The external-rule ordering fix has dedicated regressions and Linux CI coverage. Other architectures were cross-compiled, not live-kernel tested. Throughput, every crash point and concurrent external route mutations are not exhaustively verified; Netlink read/modify/write is not atomic.

## Build

```sh
go test -race ./...
go vet ./...
bash scripts/build.sh
# One architecture, with an explicit output directory:
bash scripts/build.sh dist amd64
```

Use a clean Git checkout. All builds embed VERSION (default `1.0.0`), the source commit and UTC build time. CI uses `GITHUB_SHA`; exported source requires `BUILD_COMMIT`. `BUILD_TIME` can override the UTC timestamp. Do not label changed source with an older commit. Validation artifacts with synthetic commits are not release artifacts.
