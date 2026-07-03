# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately via [GitHub Security Advisories](https://github.com/geanlabs/gean/security/advisories/new). Do not open a public issue for anything security-sensitive.

Include what you can: affected component, reproduction steps or a failing input, and impact as you understand it. We will acknowledge reports as quickly as we can and keep you informed as we triage and fix.

Gean is pre-production devnet software with no bug bounty program yet. We still want every report — consensus bugs found now are far cheaper than consensus bugs found on a live network.

## Scope

The areas where bugs are most likely to be consensus- or security-critical:

- State transition (`internal/statetransition/`) and consensus types/SSZ (`internal/types/`)
- Fork choice (`internal/forkchoice/`)
- XMSS signatures and the Rust FFI boundary (`xmss/`)
- Networking and wire decoding (`internal/p2p/`)
- Block import and attestation validation (`internal/blockprocessor/`, `internal/attestation/`, `internal/aggregation/`)

Divergence from the pinned [leanSpec](https://github.com/leanEthereum/leanSpec) reference is a bug even when nothing crashes — cross-client consensus splits are the failure mode we care most about.

## Supported versions

Only the latest commit on the current devnet branch is supported. Older devnet branches are kept for reference and do not receive fixes.

## Official sources

The only official distribution channels for Gean are:

- Source code: [github.com/geanlabs/gean](https://github.com/geanlabs/gean)
- Website: [geanlabs.com](https://geanlabs.com)

Gean Labs does not publish prebuilt binaries, installers, or release archives. Build from source using the instructions in the [README](README.md). Any repository, website, or download claiming to offer Gean binaries is unofficial and should be treated as malicious.

If you find a site or repository impersonating Gean, report it via [GitHub Security Advisories](https://github.com/geanlabs/gean/security/advisories/new) or by opening an issue, and report it to GitHub at [github.com/contact/report-abuse](https://github.com/contact/report-abuse).
