//go:build spectests

package spectests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/geanlabs/gean/xmss"
)

// Single-message (Type-1) proof fixtures.
type verifyProofsSingleCase struct {
	PublicKeys      []string    `json:"publicKeys"`
	Message         string      `json:"message"`
	Slot            uint32      `json:"slot"`
	Proof           fcProofData `json:"proof"`
	RejectionReason *string     `json:"rejectionReason"`
}

// Multi-message (Type-2) proof fixtures.
type verifyProofsMultiCase struct {
	PublicKeysPerMessage [][]string  `json:"publicKeysPerMessage"`
	Messages             []string    `json:"messages"`
	Slots                []uint32    `json:"slots"`
	Proof                fcProofData `json:"proof"`
	RejectionReason      *string     `json:"rejectionReason"`
}

func parsePubkeyHandles(t *testing.T, hexKeys []string) []xmss.CPubKey {
	t.Helper()
	keys := make([]xmss.CPubKey, 0, len(hexKeys))
	for _, h := range hexKeys {
		pk, err := xmss.ParsePublicKey(parseHexPubkey(h))
		if err != nil {
			t.Fatalf("parse pubkey: %v", err)
		}
		keys = append(keys, pk)
	}
	return keys
}

func walkFixtures(t *testing.T, root string, run func(t *testing.T, raw []byte)) {
	t.Helper()
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skipf("fixtures not present at %s; skipping", root)
	}
	var files []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if len(files) == 0 {
		t.Skipf("no fixtures in %s; skipping", root)
	}
	for _, file := range files {
		file := file
		rel, _ := filepath.Rel(root, file)
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("reading %s: %v", file, err)
			}
			run(t, raw)
		})
	}
}

func TestSpecVerifySingleMessageProofs(t *testing.T) {
	walkFixtures(t, "../../leanSpec/fixtures/consensus/verify_single_message_proofs", func(t *testing.T, raw []byte) {
		var fixture map[string]verifyProofsSingleCase
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		for name, tc := range fixture {
			tc := tc
			t.Run(name, func(t *testing.T) {
				keys := parsePubkeyHandles(t, tc.PublicKeys)
				defer func() {
					for _, k := range keys {
						xmss.FreePublicKey(k)
					}
				}()
				err := xmss.VerifyAggregatedSignature(parseHexBytes(tc.Proof.Data), keys, parseHexRoot(tc.Message), tc.Slot)
				assertProofOutcome(t, err, tc.RejectionReason)
			})
		}
	})
}

func TestSpecVerifyMultiMessageProofs(t *testing.T) {
	walkFixtures(t, "../../leanSpec/fixtures/consensus/verify_multi_message_proofs", func(t *testing.T, raw []byte) {
		var fixture map[string]verifyProofsMultiCase
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		for name, tc := range fixture {
			tc := tc
			t.Run(name, func(t *testing.T) {
				// Component counts may intentionally disagree in rejection fixtures
				// (e.g. a missing binding); let the verifier reject rather than guarding here.
				groups := make([][]xmss.CPubKey, len(tc.PublicKeysPerMessage))
				defer func() {
					for _, g := range groups {
						for _, k := range g {
							xmss.FreePublicKey(k)
						}
					}
				}()
				for i := range tc.PublicKeysPerMessage {
					groups[i] = parsePubkeyHandles(t, tc.PublicKeysPerMessage[i])
				}
				bindings := make([]xmss.MessageBinding, len(tc.Messages))
				for i := range tc.Messages {
					var msg [xmss.MessageLength]byte
					copy(msg[:], parseHexBytes(tc.Messages[i]))
					var slot uint32
					if i < len(tc.Slots) {
						slot = tc.Slots[i]
					}
					bindings[i] = xmss.MessageBinding{Message: msg, Slot: slot}
				}
				err := xmss.VerifyType2Proof(parseHexBytes(tc.Proof.Data), groups, bindings)
				assertProofOutcome(t, err, tc.RejectionReason)
			})
		}
	})
}

func assertProofOutcome(t *testing.T, err error, rejection *string) {
	t.Helper()
	if rejection != nil {
		if err == nil {
			t.Fatalf("expected rejection %q, got valid", *rejection)
		}
		return
	}
	if err != nil {
		t.Fatalf("expected valid proof, got error: %v", err)
	}
}
