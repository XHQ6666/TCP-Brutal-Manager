# 1.0.0-Beta — Pre-Release

First TCP Brutal Manager Beta for Linux. Configuration and rule maintenance through official brutalctl v2.0.1, System/DoH/DoT/DoQ, numeric CIDR ordering, managed-source records, per-rule error isolation, run mode, route/link watches, and matching cleanup of managed policy-route attributes.

Artifacts: static Linux amd64 and arm64 binaries, configuration example, systemd unit, and SHA-256 checksums.

Validation: local unit/fake-controller integration tests and race checks; Linux CI executes Linux Netlink codec preservation tests and builds. This does **not** claim live TCP Brutal module, real Tailscale rebuilding or public DNS service interoperability validation. Run on an isolated Linux test host first.

Important behavior: flush clears all Brutal rules; stop run first to prevent reapplication. Recorded-state corruption with an initialization marker invokes configured global rebuild and removes brutal attributes across tables. Ordinary routes are delegated to brutalctl. Full destructive reorder is deferred in normal apply.
