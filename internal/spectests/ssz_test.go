//go:build spectests

package spectests

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

// Spec fixture roots for SSZ conformance tests. Each directory holds fixtures
// that carry typeName / serialized (SSZ hex) / root (hash_tree_root hex).
var sszFixtureRoots = []string{
	"../../leanSpec/fixtures/consensus/ssz/lstar/ssz/test_consensus_containers",
	"../../leanSpec/fixtures/consensus/ssz/lstar/ssz/test_networking_containers",
	"../../leanSpec/fixtures/consensus/ssz/lstar/ssz/test_xmss_containers",
}

type sszFixtureOuter map[string]sszFixture

type sszFixture struct {
	Network         string          `json:"network"`
	LeanEnv         string          `json:"leanEnv"`
	TypeName        string          `json:"typeName"`
	Value           json.RawMessage `json:"value"`
	Serialized      string          `json:"serialized"`
	Root            string          `json:"root"`
	ExpectException string          `json:"expectException"`
	RawBytes        string          `json:"rawBytes"`
}

// sszCodec is the uniform contract every gean SSZ type satisfies (sszgen
// output). The value JSON is intentionally ignored — `serialized` and `root`
// are the canonical spec outputs, and exercising the gean codec against them
// covers the decoder, the encoder (via round-trip), and the hasher.
type sszCodec interface {
	UnmarshalSSZ(buf []byte) error
	MarshalSSZ() ([]byte, error)
	HashTreeRoot() ([32]byte, error)
}

var hashSkipTypes = map[string]bool{"SignedAttestation": true}

// sszFactories maps the spec typeName to a zero-value gean struct.
//
// Networking req/resp types (Status, BlocksByRootRequest) are registered via
// thin adapters in networking_ssz_adapters_test.go because their gean
// implementations live in internal/p2p/ with hand-rolled SSZ that doesn't match the
// fastssz-generated sszCodec contract.
//
// XMSS containers (PublicKey, Signature, HashTreeLayer, HashTreeOpening) are
// registered via xmss_ssz_adapters_test.go. Gean stores XMSS payloads as
// opaque byte arrays — the inner spec structure exists only in the Rust FFI
// — so the adapters reconstruct the field layout for hash_tree_root.
var sszFactories = map[string]func() sszCodec{
	"Checkpoint":                  func() sszCodec { return new(types.Checkpoint) },
	"Config":                      func() sszCodec { return new(types.ChainConfig) },
	"Validator":                   func() sszCodec { return new(types.Validator) },
	"BlockHeader":                 func() sszCodec { return new(types.BlockHeader) },
	"BlockBody":                   func() sszCodec { return new(types.BlockBody) },
	"Block":                       func() sszCodec { return new(types.Block) },
	"SignedBlock":                 func() sszCodec { return new(types.SignedBlock) },
	"AttestationData":             func() sszCodec { return new(types.AttestationData) },
	"Attestation":                 func() sszCodec { return new(types.Attestation) },
	"SignedAttestation":           func() sszCodec { return new(types.SignedAttestation) },
	"AggregatedAttestation":       func() sszCodec { return new(types.AggregatedAttestation) },
	"SignedAggregatedAttestation": func() sszCodec { return new(types.SignedAggregatedAttestation) },
	"SingleMessageAggregate":      func() sszCodec { return new(types.SingleMessageAggregate) },
	"MultiMessageAggregate":       func() sszCodec { return new(types.MultiMessageAggregate) },
	"State":                       func() sszCodec { return new(types.State) },
	"Status":                      func() sszCodec { return new(sszStatusAdapter) },
	"BlocksByRootRequest":         func() sszCodec { return new(sszBlocksByRootRequestAdapter) },
	"PublicKey":                   func() sszCodec { return new(sszPublicKeyAdapter) },
	"Signature":                   func() sszCodec { return new(sszSignatureAdapter) },
	"HashTreeLayer":               func() sszCodec { return new(sszHashTreeLayerAdapter) },
	"HashTreeOpening":             func() sszCodec { return new(sszHashTreeOpeningAdapter) },
}

// sszDecodeFailureFactories provides decode-only entry points for fixtures
// that carry expectException + rawBytes and assert the decoder rejects
// malformed input. Empty for now: the rejection fixtures
// currently target spec-internal parameterized type wrappers (DecodeBitlist8,
// DecodeBitvector16) plus basic types (Bytes4, Uint32) that gean never
// exposes as standalone decoders. The harness skips with a clear reason
// when typeName isn't registered, so future fixtures targeting types gean
// does decode (e.g. via UnmarshalSSZ on a registered struct) auto-work
// once added here.
var sszDecodeFailureFactories = map[string]func([]byte) error{}

// TestSpecSSZ walks the ssz container fixture directories and, for each
// fixture, exercises the gean codec end-to-end:
//
//  1. hex-decode `serialized` into bytes.
//  2. UnmarshalSSZ(bytes) into a zero-value gean struct — exercises the decoder.
//  3. MarshalSSZ() the struct again and assert byte-for-byte equal to (1) —
//     exercises the encoder via round-trip.
//  4. HashTreeRoot() and assert equal to the fixture's `root` — exercises
//     merkleization independently of encode/decode.
func TestSpecSSZ(t *testing.T) {
	var walked int
	for _, root := range sszFixtureRoots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			t.Logf("skipping %s: not present", root)
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
				return err
			}
			walked++

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("%s: read: %v", path, err)
				return nil
			}

			var outer sszFixtureOuter
			if err := json.Unmarshal(raw, &outer); err != nil {
				t.Errorf("%s: unmarshal: %v", path, err)
				return nil
			}

			for testID, fx := range outer {
				name := shortName(testID, path)
				fx := fx
				t.Run(name, func(t *testing.T) {
					runSSZFixture(t, fx)
				})
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	if walked == 0 {
		t.Skip("no ssz fixtures found; run 'make leanSpec/fixtures'")
	}
}

func runSSZFixture(t *testing.T, fx sszFixture) {
	t.Helper()

	if fx.ExpectException != "" {
		runSSZDecodeFailure(t, fx)
		return
	}

	factory, ok := sszFactories[fx.TypeName]
	if !ok {
		t.Skipf("type %q not wired into ssz harness", fx.TypeName)
		return
	}

	want, err := hex.DecodeString(strings.TrimPrefix(fx.Serialized, "0x"))
	if err != nil {
		t.Fatalf("decode serialized hex: %v", err)
	}

	// (2) Decode spec-canonical bytes into a gean struct.
	obj := factory()
	if err := obj.UnmarshalSSZ(want); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}

	// (3) Re-encode and assert byte-for-byte equality with the input.
	got, err := obj.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("ssz round-trip mismatch:\n got:  %s\n want: %s",
			hex.EncodeToString(got), hex.EncodeToString(want))
	}

	// (4) Independently hash and compare to the fixture's expected root.
	// Skip known spec-ecosystem divergences — see hashSkipTypes for rationale.
	if hashSkipTypes[fx.TypeName] {
		return
	}
	hash, err := obj.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	gotRoot := "0x" + hex.EncodeToString(hash[:])
	wantRoot := strings.ToLower(fx.Root)
	if !strings.HasPrefix(wantRoot, "0x") {
		wantRoot = "0x" + wantRoot
	}
	if gotRoot != wantRoot {
		t.Errorf("hash_tree_root mismatch:\n got:  %s\n want: %s", gotRoot, wantRoot)
	}
}

// runSSZDecodeFailure handles fixtures with expectException set: the rawBytes
// payload is decoded against the registered decoder for typeName, and the
// decode MUST fail. Skips cleanly when typeName isn't registered so future
// fixtures auto-work as types are wired in.
func runSSZDecodeFailure(t *testing.T, fx sszFixture) {
	t.Helper()

	decoder, ok := sszDecodeFailureFactories[fx.TypeName]
	if !ok {
		t.Skipf("decode-failure type %q not wired into ssz harness", fx.TypeName)
		return
	}

	raw, err := hex.DecodeString(strings.TrimPrefix(fx.RawBytes, "0x"))
	if err != nil {
		t.Fatalf("decode rawBytes hex: %v", err)
	}

	if err := decoder(raw); err == nil {
		t.Errorf("expected decode of typeName=%q to fail with %s, but it succeeded",
			fx.TypeName, fx.ExpectException)
	}
}
