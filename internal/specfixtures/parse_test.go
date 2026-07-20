package specfixtures

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestParseHexRootRejectsTooLongInput(t *testing.T) {
	tooLong := "0x" + hex.EncodeToString(make([]byte, 33))
	if _, err := ParseHexRoot(tooLong); err == nil {
		t.Fatal("expected overlong root error")
	}
}

func TestParseHexRootRejectsEmptyInput(t *testing.T) {
	for _, input := range []string{"", "0x", " 0X "} {
		if _, err := ParseHexRoot(input); err == nil || !strings.Contains(err.Error(), "empty hex root") {
			t.Fatalf("ParseHexRoot(%q) error=%v, want empty root rejection", input, err)
		}
	}
}

func TestParseHexRootPadsShortInput(t *testing.T) {
	root, err := ParseHexRoot("0x0102")
	if err != nil {
		t.Fatalf("parse root: %v", err)
	}
	if root[0] != 0x01 || root[1] != 0x02 {
		t.Fatalf("root prefix=%x, want 0102", root[:2])
	}
}

func TestParseHexAcceptsUppercasePrefixAndWhitespace(t *testing.T) {
	root, err := ParseHexRoot(" 0X0A0b ")
	if err != nil {
		t.Fatalf("parse root: %v", err)
	}
	if root[0] != 0x0a || root[1] != 0x0b {
		t.Fatalf("root prefix=%x, want 0a0b", root[:2])
	}

	got, err := ParseHexBytes(" 0XDEad ")
	if err != nil {
		t.Fatalf("parse bytes: %v", err)
	}
	if hex.EncodeToString(got) != "dead" {
		t.Fatalf("bytes=%x, want dead", got)
	}
}

func TestParseFixedHexRejectsEmptyInput(t *testing.T) {
	if _, err := ParseHexPubkey("0x"); err == nil || !strings.Contains(err.Error(), "empty pubkey") {
		t.Fatalf("ParseHexPubkey error=%v, want empty pubkey rejection", err)
	}
	if _, err := ParseHexSignature(" "); err == nil || !strings.Contains(err.Error(), "empty signature") {
		t.Fatalf("ParseHexSignature error=%v, want empty signature rejection", err)
	}
}

func TestParseBoolBitlistAcceptsBoolAndInt(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage("true"),
		json.RawMessage("0"),
		json.RawMessage("1"),
	}
	bits, err := ParseBoolBitlist(raw)
	if err != nil {
		t.Fatalf("parse bitlist: %v", err)
	}
	if !types.BitlistGet(bits, 0) || types.BitlistGet(bits, 1) || !types.BitlistGet(bits, 2) {
		t.Fatalf("unexpected bitlist bits: %08b", bits)
	}
}

func TestParseBoolBitlistRejectsNull(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage("null"),
	}
	if _, err := ParseBoolBitlist(raw); err == nil || !strings.Contains(err.Error(), "null value") {
		t.Fatalf("ParseBoolBitlist error=%v, want null rejection", err)
	}
}

func TestParseBoolBitlistRejectsNonBinaryInt(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage("2"),
	}
	if _, err := ParseBoolBitlist(raw); err == nil || !strings.Contains(err.Error(), "not 0 or 1") {
		t.Fatalf("ParseBoolBitlist error=%v, want non-binary int rejection", err)
	}
}

func TestParseRootListRejectsNull(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage("null"),
	}
	if _, err := parseRootList("roots", raw); err == nil || !strings.Contains(err.Error(), "empty hex root") {
		t.Fatalf("parseRootList error=%v, want null root rejection", err)
	}
}

func TestToStateRequiresCompleteValidatorKeys(t *testing.T) {
	state := minimalTestState()
	state.Validators.Data = []TestValidator{{AttestationPubkey: hexOfLen(types.PubkeySize)}}
	if _, err := state.ToState(); err == nil || !strings.Contains(err.Error(), "proposalPubkey: missing") {
		t.Fatalf("ToState error=%v, want missing proposal pubkey", err)
	}

	state = minimalTestState()
	state.Validators.Data = []TestValidator{{}}
	if _, err := state.ToState(); err == nil || !strings.Contains(err.Error(), "pubkey: missing") {
		t.Fatalf("ToState error=%v, want missing legacy pubkey", err)
	}
}

func TestConvertersRejectNilFixtures(t *testing.T) {
	var state *TestState
	if _, err := state.ToState(); err == nil || !strings.Contains(err.Error(), "state fixture is nil") {
		t.Fatalf("ToState error=%v, want nil fixture rejection", err)
	}

	var block *TestBlock
	if _, err := block.ToBlock(); err == nil || !strings.Contains(err.Error(), "block fixture is nil") {
		t.Fatalf("ToBlock error=%v, want nil fixture rejection", err)
	}

	var att *TestAggregatedAttestation
	if _, err := att.ToAggregatedAttestation(); err == nil || !strings.Contains(err.Error(), "aggregated attestation fixture is nil") {
		t.Fatalf("ToAggregatedAttestation error=%v, want nil fixture rejection", err)
	}

	var attData *TestAttData
	if _, err := attData.ToAttestationData(); err == nil || !strings.Contains(err.Error(), "attestation data fixture is nil") {
		t.Fatalf("ToAttestationData error=%v, want nil fixture rejection", err)
	}

	var signedBlock *FixtureSignedBlock
	if _, err := signedBlock.ToSignedBlock(); err == nil || !strings.Contains(err.Error(), "signed block fixture is nil") {
		t.Fatalf("ToSignedBlock error=%v, want nil fixture rejection", err)
	}
}

func TestToStateAcceptsLegacyValidatorKey(t *testing.T) {
	state := minimalTestState()
	state.Validators.Data = []TestValidator{{Pubkey: hexOfLen(types.PubkeySize), Index: 7}}

	got, err := state.ToState()
	if err != nil {
		t.Fatalf("ToState: %v", err)
	}
	if len(got.Validators) != 1 || got.Validators[0].Index != 7 {
		t.Fatalf("validators=%v, want one validator with index 7", got.Validators)
	}
	if got.Validators[0].AttestationPubkey != got.Validators[0].ProposalPubkey {
		t.Fatal("legacy pubkey should seed both attestation and proposal keys")
	}
}

func minimalTestState() TestState {
	return TestState{
		Config:            TestConfig{GenesisTime: 1},
		LatestBlockHeader: TestBlockHeader{ParentRoot: "0x01", StateRoot: "0x02", BodyRoot: "0x03"},
		LatestJustified:   TestCheckpoint{Root: "0x04"},
		LatestFinalized:   TestCheckpoint{Root: "0x05"},
	}
}

func hexOfLen(n int) string {
	return "0x" + hex.EncodeToString(make([]byte, n))
}

// TestValidator must map the fixture's public-key fields, which are serialized as
// attestationPublicKey / proposalPublicKey. A tag mismatch here leaves both empty,
// so ToState falls through to the single-key path and rejects every anchor with
// "pubkey: missing" — silently failing the entire hive spec-test driver at init.
func TestTestValidatorMapsCanonicalPublicKeyFields(t *testing.T) {
	const raw = `{"attestationPublicKey":"0xaa","proposalPublicKey":"0xbb","index":7}`
	var v TestValidator
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v.AttestationPubkey != "0xaa" {
		t.Fatalf("attestationPublicKey not mapped: got %q", v.AttestationPubkey)
	}
	if v.ProposalPubkey != "0xbb" {
		t.Fatalf("proposalPublicKey not mapped: got %q", v.ProposalPubkey)
	}
	if v.Index != 7 {
		t.Fatalf("index not mapped: got %d", v.Index)
	}
}
