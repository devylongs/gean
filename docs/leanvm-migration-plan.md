# leanVM XMSS migration plan (experimental)

Branch: `experimental/leanvm-track-main`

Goal: move gean off the split **leanSig (scheme) + leanVM (aggregation)** crypto
stack onto the **single internalized-XMSS leanVM** stack that the live devnet
clients have adopted (the "sub-MTU" variant). This mirrors what ethlambda did on
`build/leanvm-track-main`, but mapped onto gean's own FFI/workspace layout.

## Status gate (do not merge past this line without the group)

This branch is **experimental prep**, not a merge candidate. Two upstream facts
must land before gean adopts the variant on `main`:

1. **leanSpec adopts the sub-MTU scheme.** Today the pinned spec (`eca701e`)
   `PROD_CONFIG` is Dim46 aborting — the large variant gean already implements.
   gean pins the spec; the trigger is a new `LEAN_SPEC_COMMIT_HASH` whose
   `PROD_CONFIG` is the sub-MTU scheme.
2. **A frozen leanVM rev.** Clients currently *track* leanVM `main` (it moved
   `a73ab11 → c83b40f → 380da82` in three days). gean pins a SHA; we need the
   one the devnet freezes on, not "main".

Until both land this branch builds and self-tests but does not interop-gate.

## Current gean crypto layout (what changes)

Three-crate Rust workspace under `xmss/rust/`, funnelled through `cgo-glue`
(staticlib shim) into `xmss/ffi.go`:

| Crate | Upstream today | Responsibility | Fate |
|---|---|---|---|
| `hashsig-glue` | **leanSig** `devnet4` (`SIGAbortingTargetSumLifetime32Dim46Base8`) | raw XMSS: keygen, sign, verify-single, pubkey/sig parse | **rewire to leanVM `xmss`** |
| `multisig-glue` | **leanVM** `e2592df` (`lean-multisig`, `rec_aggregation`, `backend`) | aggregation: setup/aggregate/verify type-1, merge/split type-2, Poseidon | **bump to frozen leanVM rev** |
| `cgo-glue` | — | staticlib shim re-exporting both | unchanged (verify symbols still dedup) |

End state: **both glue crates source from one pinned leanVM rev; the `leansig`
dependency is deleted.** leanVM `main` internalized XMSS, so the scheme now lives
in leanVM's `xmss` crate alongside aggregation — the two-upstream split is gone.

## Work breakdown

### 1. Rust — `hashsig-glue` (the real work)
- Delete `leansig = { git = ..., branch = "devnet4" }` from
  `hashsig-glue/Cargo.toml`. Add the leanVM `xmss` crate at the frozen rev
  (same rev as `multisig-glue` — keep them identical).
- `hashsig-glue/src/lib.rs`: replace the `leansig::signature::generalized_xmss::
  instantiations_aborting` imports (`SIGAbortingTargetSumLifetime32Dim46Base8`,
  and the `lifetime_2_to_the_8` test variant behind `test-config`) with leanVM's
  internalized XMSS scheme + its test-lifetime variant.
- Adapt to the reworked #262 API across the C exports (lines 108–363):
  - keygen: `xmss_key_gen(seed, activation_slot, num_active_slots)` — maps onto
    gean's existing `GenerateKeyPair(seedPhrase, activationEpoch, numActiveEpochs)`.
  - sign: **no RNG argument** now; `sign(&sk, slot, &[u8;32])`. gean's
    `hashsig_*` sign export drops the rng plumbing.
  - verify + SSZ parse: new pubkey/sig byte lengths (32 / 1208).
  - `hashsig_message_length()` stays 32.
- Preserve a fast **test scheme** (short lifetime) behind the existing
  `test-config` feature so `make test-ffi` stays quick.

### 2. Rust — `multisig-glue`
- Bump `lean-multisig`, `rec_aggregation`, `backend` from `e2592df` to the frozen
  leanVM rev in `multisig-glue/Cargo.toml`.
- Re-check the six aggregation C exports against #262's "reworked aggregation":
  `xmss_setup_prover/verifier`, `xmss_aggregate_type_1`, `xmss_verify_type_1`,
  `xmss_merge_type_1_to_type_2`, `xmss_split_type_2_by_message`,
  `xmss_verify_type_2`. `LogInvRate=2` stays (spec prod value).
- Adopt `rec_aggregation`'s **serialize-aggregate-without-pubkeys** option once
  present upstream (author flagged it missing on `main`, fixing) — this is the
  block/aggregate size lever.
- **Do not touch `[profile.multisig-release]`** (opt-level 3, codegen-units 1).
  `"z"`/`"s"` miscompile `xmss_aggregate` on x86_64 — documented in the workspace
  `Cargo.toml`.

### 3. Rust — workspace lock
- Regenerate `xmss/rust/Cargo.lock`; confirm **no `leanSig` source lines remain**
  and every `leanVM.git` entry is the single frozen rev (mirror of ethlambda's
  track-main lock, which is uniform on one SHA with zero leansig entries).

### 4. Go — sizes and SSZ (regen, never hand-edit `*_encoding.go`)
- `internal/types/constants.go`: `PubkeySize` 52 → 32, `SignatureSize` 2536 → 1208.
- Fix the **literal** ssz-size tags (they carry numbers, not the const):
  - `attestation.go:18` `ssz-size:"2536"` → `"1208"`.
  - `validator.go` pubkey fields `"52"` → `"32"` (Attestation/Proposal pubkey).
- `make sszgen` — regenerates `attestation_encoding.go`, `validator_encoding.go`,
  `state_encoding.go` (embeds Validator), `block_encoding.go` (embeds
  attestations). If sszgen errors, STOP and report — no hand-writing.

### 5. Go — FFI binding (`xmss/ffi.go`, `xmss/keys.go`)
- Array types keyed on the constants (`[types.SignatureSize]byte`,
  `[types.PubkeySize]byte`) resize automatically, but verify the C call shapes:
  - `VerifySignatureSSZ` (171), `GenerateKeyPair` (635), `ParsePublicKey` (598),
    `ParseSignature` (615), `PublicKeyBytes`/`PrivateKeyBytes` (646/666).
  - `keys.go` `Sign`/`SignAttestation`/`SignBlock` return the resized sig — check
    the underlying export dropped the RNG arg.
- `pubkey_cache.go` / `proof_pool.go`: size-driven, no logic change; confirm buffer
  sizings still hold.

### 6. Go — keygen + fixtures + tests
- `cmd/keygen/manifest.go:100` `len(s) != PubkeySize*2` now 64 not 104; update the
  `104`/`[52]byte` literals in `cmd/keygen/main_test.go` and
  `internal/types/ssz_test.go` / `ssz_compliance_test.go`.
- Regenerate testnet keys (`make run-setup` path via `cmd/keygen`).
- Refresh spec fixtures once the spec pin bumps (`make test-spec`).

### 7. Architecture invariants to preserve (from the leanVM author's notes)
- **Proving is single-proof-at-a-time upstream.** gean already proves one Type-2
  block proof per slot on the off-tick proposal worker and splits on the recovery
  worker — keep it. Do **not** add a proving pool.
- **Verification is parallel-safe.** gean's per-attestation verify goroutines are
  fine; keep `AttestationSignatureMap` mutex-guarded.
- Proving stays **off** the 800ms tick loop so the clock doesn't drift.

## Sequencing / commits (each its own commit, revertible)
1. `hashsig-glue` rewire (leanSig → leanVM xmss) + `multisig-glue` rev bump + lock.
2. `constants.go` + ssz tags + `make sszgen` (regenerated files in the same commit).
3. `ffi.go` / `keys.go` API adaptation.
4. keygen + test-literal updates; regenerate keys.
5. spec-pin bump (**separate commit**) + `make test-spec` fixtures.

## Verification gate
`make build` → `make test-ffi` → `make test` → `make test-spec` (after spec pin) →
`make lint` → devnet interop against the frozen rev (`devnet-runner`).

## Step 1 progress (spike against placeholder rev `380da82`)

Done on this branch (Rust only; Go untouched, so `make ffi` builds but `make test-ffi`
won't until step 4):

- **`hashsig-glue`**: dropped `leansig`; now sources the scheme from leanVM's `xmss`
  crate. `SignatureScheme::{key_gen,sign,verify}` → free fns `xmss_key_gen_from_seed`
  / `xmss_sign` / `xmss_verify`; `MESSAGE_LENGTH` → `MESSAGE_LEN_BYTES`. Secret key is
  **serde-only upstream** → persist with **postcard** (was SSZ); pubkey/sig stay SSZ.
  All C symbols preserved (Go binding unchanged). Compiles clean.
- **`multisig-glue`**: rev `e2592df` → `380da82`. Aggregate wire API renamed:
  `compress_without_pubkeys()`→`to_bytes_without_pubkeys()`,
  `decompress_without_pubkeys()`→`from_bytes_without_pubkeys()`,
  `info.without_pubkeys.{message,slot}`→`info.core.{message,slot}`. Imports/functions
  otherwise stable.
- **Confirmed sizes** from `xmss` crate constants: `PUB_KEY_SSZ_LEN=32`,
  `SIGNATURE_SSZ_LEN=1208` (scheme is V=42, base 8, log-lifetime 32).
- **Lockfile**: regenerated — **zero leanSig entries**, single leanVM rev.
- **FOLLOW-UP (flagged):** leanVM removed the **width-24 Poseidon** permutation
  (scheme now hashes width-16 only). `poseidon_permute_kb24` FFI is stubbed to return
  `-1` (loud failure, ABI preserved). Only consumer is the `poseidon_permutation`
  spec-vector test — revisit when the spec pin bumps (vectors expected to drop width-24).

Not yet done (later steps): Go constants + ssz tags + `make sszgen` (step 4), FFI
call-shape/Go review (step 5), keygen + fixtures (step 6).

## Open questions for the call
1. Sub-MTU adopted for devnet5 (breaking) — confirmed yes/no?
2. Exact frozen leanVM rev to pin (not "main").
3. leanSpec commit that lands the new `PROD_CONFIG` (our go signal).
4. Confirmed scheme identifier + key/sig sizes (32 / 1208) so fixtures line up
   cross-client.
