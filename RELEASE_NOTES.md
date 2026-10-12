# 1.0.0

First stable TCP Brutal Manager release for Linux, with TCP Brutal v2.0.1 as the supported/tested baseline.

## Features

- JSON rules for IPs and domains, integer-Mbps bit-rate conversion and per-family CIDR.
- System, DoH, DoT and DoQ resolvers, named DNS selection rules and optional fallback.
- Per-rule failure isolation, shared-source ownership, safe preservation of external rules.
- Run mode with configuration and Netlink route/link events, independent DNS/rule/route timers.
- Existing policy-route attribute management, matching cleanup, state integrity and resumable recovery.

## Fixes since Beta

Unclaimed external rules are excluded from managed order comparison. Regression tests retain detection of real managed-order differences. A common build script embeds version, source commit and UTC build time for every target and refuses missing provenance.

## Packages and docs

Linux amd64, arm64, 386, armv5, armv6 and armv7 archives, SHA256SUMS, example configuration, systemd template and formal configuration/CLI/operations documentation.

## Validation and limits

Unit/fake-controller integration, race checks, vet and Linux Netlink tests run in CI. Linux amd64 live tests with TCP Brutal 2.0.1 covered public System/DoH/DoT/DoQ, rule updates and isolation, reload, route/link repair, external/shared ownership, Flush and corrupt-record recovery. The ordering patch was validated by targeted regressions and Linux CI; it has not yet replaced the live test host binary.

Other architectures are cross-compiled, not live-kernel validated. No throughput claim is made. Crash/disk-full/concurrent external-route failure cases are not exhaustive. Route read/modify/write is not atomic. Destructive reordering remains deferred during normal Apply. Flush clears all rules; marked corrupt state triggers global rebuild. The systemd template was not installed in the live tests.
