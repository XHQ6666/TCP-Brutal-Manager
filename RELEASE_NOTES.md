# 1.0.1 (Pre-Release)

The binary, service, default configuration/state/log paths and documentation now consistently use tcp-brutal-manager.

Policy routes now default to CIDR containment: one Brutal group can serve all matching existing routes in the configured table/interface, including more specific node routes. No aggregate route or per-node Brutal group is created.

- Add route_strict (default false) for exact-prefix routing policies.
- Maintain multiple routes with per-route failure isolation and most-specific policy lock precedence.
- Preserve route identity, unrelated attributes, metrics and ECMP nexthops; interface-constrained ECMP requires all nexthops to match.
- Continue maintaining the Brutal group when routes are temporarily absent; route/link events and the existing 60-second route resync maintain attributes.
- Migrate valid v1 management records to v2 with a backup, without Flush; retain ambiguous legacy identities for review.
- Add per-route plan details and matched/succeeded/failed summaries.

Validation: local race/vet and six Linux cross-builds passed. Linux amd64 tests, Netlink attribute/ECMP preservation and an isolated real-route integration passed. Production Linux amd64 verification maintained five rules and seventeen policy routes without errors across a periodic maintenance cycle. Identifiers in reports are redacted. Other architectures remain compile-tested only; exhaustive crash/concurrency cases are not covered.

Downgrade caution: older releases do not understand state v2. Do not restart an old binary against v2 records without reviewing route attributes and arranging restoration from deployment backups.

Packages: Linux amd64, arm64, 386, armv5, armv6, armv7 and SHA256SUMS. This release is a pre-release.

---

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
