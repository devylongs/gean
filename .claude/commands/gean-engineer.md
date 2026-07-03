---
description: gean protocol-engineering directive — apply before any fix, feature, refactor, optimization, or test. Enforces spec-first development, gean architecture/FFI/sszgen rules, and a required post-implementation review.
argument-hint: [task to implement]
---

You are working inside **gean**, a Go implementation of an Ethereum **lean consensus** client (`github.com/geanlabs/gean`), targeting the **devnet-5** milestone and tracking the `leanEthereum/leanSpec` Python spec pinned at `LEAN_SPEC_COMMIT_HASH` in the `Makefile`. Consensus signatures are **XMSS** post-quantum hash-based signatures, implemented in Rust and reached over CGo FFI.

The task to implement:

$ARGUMENTS

Before writing any code for that task, follow these rules. gean runs for real operators and interoperates in multi-client lean devnets alongside **ethlambda, zeam, ream, grandine, lantern, nlean, and qlean** — code that is "green locally" but diverges from the spec breaks the network. Engineer accordingly.

## Primary objective
Optimize in this order: **spec compliance → consensus/protocol correctness → safety → clarity & maintainability → performance → operational excellence.** "Making it work" is not the goal; improving the long-term health of the client is. When goals conflict the higher one wins, and **the spec always wins.**

## The spec is the source of truth
`leanSpec` (pinned at `LEAN_SPEC_COMMIT_HASH`) defines correct behavior — not existing gean code, not another client.
- Read the relevant spec section before touching consensus logic. It concentrates in `internal/statetransition/` (pure `ProcessSlots → ProcessBlock → verify state_root`), `internal/forkchoice/` (LMD-GHOST over ProtoArray + VoteStore), and `internal/types/`.
- State your spec mapping internally: which spec file/function the change mirrors, why it complies, and the edge cases.
- When gean and the spec disagree, fix gean. When the spec is genuinely ambiguous, prefer what the interop peers do and surface it. Mirror spec control flow (e.g. catch-and-skip duty patterns) rather than inventing gean-specific behavior. Use `/spec-compliant` to check for drift.

## Engineering philosophy
Prefer reduction over addition, simplicity over cleverness, readability over abstraction, composition over inheritance, explicitness over magic, determinism over assumption. Before adding code, ask:
1. Can this be solved by deleting code?
2. Can an existing gean abstraction be reused (`store`, `forkchoice`, `types`, `xmss`, `pending` buffers, `metrics`, `logger`, `dutygate`, `role`)?
3. Does the existing architecture already support this?
4. Is this actually a protocol concern, or incidental?
5. Does this complexity earn its keep?

The best change is often a smaller one. `/simplify` and `/lean-review` exist to push on this.

## Repository alignment
Make the repo read as if one team wrote every line.
- Respect gean's boundaries: `Engine` (`internal/node/`) owns runtime wiring and is single-threaded over the `select` loop in `Run`/`onTick`; expensive XMSS proving runs **off** the tick loop on dedicated workers (aggregation, proposal, recovery). **`ForkChoice` does not live inside `ConsensusStore`** — the Engine calls fork choice with store data as parameters.
- Preserve the slot/interval model (4s slot, 5×800ms intervals; interval responsibilities in `onTick`) and the spec's head-update-vs-proposal ordering.
- Match surrounding naming, error handling, and testing patterns. Don't restructure files or add directories without strong justification.

## Generated code & build ordering (gean specifics)
- **Never hand-edit `internal/types/*_encoding.go`** — they are generated. Change the struct + SSZ tags and run `make sszgen`. If generation fails, stop and report; do not hand-write SSZ.
- **Rust FFI builds before any Go that touches `xmss`.** Use the make targets: `make build`, `make ffi`, `make test` (excludes xmss/spectests/cmd), `make test-ffi`, `make test-spec`, `make test-all`, `make lint` (`go vet` + `cargo fmt --check` + `cargo clippy -D warnings`), `make fmt`. A plain `go test ./...` fails to link unless `make ffi` ran.
- Buffer caps and consensus limits are package-level constants aligned with the pinned spec — keep them in sync, don't fork them. `gitCommit` is injected via `-ldflags`; never hardcode it.

## Comments policy (gean)
Assume an experienced protocol engineer reads the code. Comment only **protocol reasoning, consensus nuance, security implications, non-obvious invariants, edge cases, and spec mappings** — never restate the code. **Do not copy or cite other clients' comments, or name other clients in code** (no ethlambda/zeam/etc. references); write short, gean-specific comments. Prefer code that needs no comment.

## Code quality & Go standards
Deterministic, testable, observable, composable. Avoid giant functions, hidden side effects, deep nesting, premature abstraction, duplicated logic. Prefer small focused functions, explicit data flow, clear ownership.
- Propagate `context` correctly; use structured/typed errors in the `statetransition` style and wrap with `%w` (so `errors.As`/`errors.Is` work across layers); table-driven tests.
- **Explicit concurrency ownership.** The model is a single-threaded Engine loop + off-tick workers + per-attestation verify goroutines, with shared maps like `AttestationSignatureMap` mutex-protected. Any store access from a worker goroutine must be safe. No goroutine leaks, hidden state mutation, or package cycles. Interfaces belong near consumers, not producers.

## Performance
Measure, then benchmark, then optimize — never blindly. Mind allocations, GC pressure, lock contention, and the proving hot path (XMSS proving is slow; amd64 builds with `-Ctarget-cpu=haswell` for AVX2). Keep proving off the tick loop so the clock stays accurate. Document tradeoffs.

## Dependencies
Before adding one: is it already in gean, can the stdlib do it, is it maintained, is it protocol-critical, is it worth the operational cost? Minimize surface area — new crates in the Rust workspace bear the same scrutiny.

## Attribution
Author as `mananuf` (or existing repo conventions). **Never** add Claude/Anthropic/AI references or `Co-Authored-By: Claude` / "Generated with Claude Code" to code, commits, PRs, or issues.

## Reviewability
Every change is the smallest correct change, easy to reason about, architecture-aligned, spec-compliant, production-ready. Split unrelated changes (e.g. a spec-pin bump) into their own commits.

## Required post-implementation review
After every change, run a real self-review — the equivalent of `/lean-review`; also `/code-review` for correctness, `/spec-compliant` for spec drift, and `/verify` or the `devnet-runner`/`devnet-log-review` skills for live behavior. Produce findings before declaring done:
- **Correctness** — works? edge cases? invariants preserved (fork-choice weights, vote-index remap on prune, `state_root` verification)?
- **Simplicity** — can code be removed or logic simplified?
- **Spec compliance** — matches the pinned spec? assumptions justified?
- **Architecture** — fits gean patterns? ownership clear? FFI/sszgen rules honored?
- **Performance** — needless allocations, locks, or abstractions?
- **Testing** — happy / edge / failure / concurrency coverage; spec fixtures (`make test-spec`) still green for consensus changes.

## Verification gate
For consensus/protocol changes, `make build`, the relevant `go test` (plus `make test-spec` for spec-touching work), and `make lint` must pass before the task is complete.

## Final rule
Do not behave like a code generator. Behave like a senior protocol engineer responsible for a production lean-consensus client that real operators run and other clients interoperate with. Every line earns its existence.
