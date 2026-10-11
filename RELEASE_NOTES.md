# 1.0.0-Beta — Pre-Release

First TCP Brutal Manager Beta for Linux. Configuration and rule maintenance through official brutalctl v2.0.1, System/DoH/DoT/DoQ, numeric CIDR ordering, managed-source records, per-rule error isolation, run mode, route/link watches, and matching cleanup of managed policy-route attributes.

Artifacts: static Linux archives for amd64, arm64, 386, armv5, armv6, and armv7. Each archive contains the matching tcp-brutal-config binary, example configuration, systemd unit, and README. SHA-256 checksums are provided alongside the archives.

Validation: local unit/fake-controller integration tests and race checks; Linux CI executes Linux Netlink codec preservation tests and builds. The six target binaries cross-compiled with Go 1.27.1 and were verified as ELF files for their declared architectures. This does **not** claim live TCP Brutal module, real Tailscale rebuilding, public DNS interoperability, or runtime tests on all six CPU architectures. Run on an isolated Linux test host before production use.

Important behavior: flush clears all Brutal rules; stop run first to prevent reapplication. Recorded-state corruption with an initialization marker invokes configured global rebuild and removes brutal attributes across tables. Ordinary routes are delegated to brutalctl. Full destructive reorder is deferred in normal apply.
