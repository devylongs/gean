package genesis

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

const testConfigYAML = `GENESIS_TIME: 1770407233
GENESIS_VALIDATORS:
  - attestation_pubkey: "cd323f232b34ab26d6db7402c886e74ca81cfd3a0c659d2fe022356f25592f7d"
    proposal_pubkey: "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1"
  - attestation_pubkey: "b7b0f72e24801b02bda64073cb4de6699a416b37dfead227d7ca3922647c940f"
    proposal_pubkey: "b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1"
  - attestation_pubkey: "8d9cbc508b20ef43e165f8559c1bdd18aaeda805ef565a4f9ffd6e4fbed01c05"
    proposal_pubkey: "c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1"
`

func pubkeyHex(value byte) string {
	return strings.Repeat(fmt.Sprintf("%02x", value), types.PubkeySize)
}

func TestLoadGenesisConfig(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, testConfigYAML)

	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if config.GenesisTime != 1770407233 {
		t.Fatalf("genesis time: expected 1770407233, got %d", config.GenesisTime)
	}
	if len(config.GenesisValidators) != 3 {
		t.Fatalf("validators: expected 3, got %d", len(config.GenesisValidators))
	}
}

func TestValidators(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, testConfigYAML)

	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	validators, err := config.Validators()
	if err != nil {
		t.Fatalf("validators: %v", err)
	}

	if len(validators) != 3 {
		t.Fatalf("expected 3 validators, got %d", len(validators))
	}
	for i, v := range validators {
		if v.Index != uint64(i) {
			t.Fatalf("validator %d index: expected %d, got %d", i, i, v.Index)
		}
		if v.AttestationPubkey == [types.PubkeySize]byte{} {
			t.Fatalf("validator %d has zero attestation pubkey", i)
		}
		if v.ProposalPubkey == [types.PubkeySize]byte{} {
			t.Fatalf("validator %d has zero proposal pubkey", i)
		}
		if v.AttestationPubkey == v.ProposalPubkey {
			t.Fatalf("validator %d: attestation and proposal pubkeys should be different", i)
		}
	}
}

func TestGenesisState(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, testConfigYAML)

	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	state, err := config.GenesisState()
	if err != nil {
		t.Fatalf("genesis state: %v", err)
	}

	if state.Slot != 0 {
		t.Fatalf("genesis slot should be 0, got %d", state.Slot)
	}
	if state.Config.GenesisTime != 1770407233 {
		t.Fatal("genesis time mismatch")
	}
	if len(state.Validators) != 3 {
		t.Fatalf("expected 3 validators, got %d", len(state.Validators))
	}
	if !types.IsZeroRoot(state.LatestJustified.Root) {
		t.Fatal("justified root should be zero at genesis")
	}
	if !types.IsZeroRoot(state.LatestFinalized.Root) {
		t.Fatal("finalized root should be zero at genesis")
	}
}

func TestLoadGenesisConfigMissing(t *testing.T) {
	_, err := LoadGenesisConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("should error on missing file")
	}
}

func TestLoadGenesisConfigInvalidPubkey(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	data := `GENESIS_TIME: 1770407233
GENESIS_VALIDATORS:
  - attestation_pubkey: "not-hex"
	    proposal_pubkey: "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1"
`
	writeGenesisConfig(t, tmpFile, data)

	_, err := LoadGenesisConfig(tmpFile)
	if err == nil {
		t.Fatal("expected invalid pubkey error")
	}
}

func TestLoadGenesisConfigValidatorCountConsistent(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, "VALIDATOR_COUNT: 3\n"+testConfigYAML)

	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if config.ValidatorCount == nil {
		t.Fatal("ValidatorCount should be parsed when VALIDATOR_COUNT is set")
	}
	if *config.ValidatorCount != 3 {
		t.Fatalf("ValidatorCount: expected 3, got %d", *config.ValidatorCount)
	}
}

func TestLoadGenesisConfigValidatorCountMismatch(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, "VALIDATOR_COUNT: 5\n"+testConfigYAML)

	_, err := LoadGenesisConfig(tmpFile)
	if err == nil {
		t.Fatal("expected error when VALIDATOR_COUNT disagrees with len(GENESIS_VALIDATORS)")
	}
}

// TestLoadGenesisConfigAcceptsLeanchainFields covers the shared leanchain
// config.yaml schema other clients also consume (committee count, key lifetime,
// validator count) — these must parse, not trip strict field checking.
func TestLoadGenesisConfigAcceptsLeanchainFields(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile,
		"ATTESTATION_COMMITTEE_COUNT: 1\nACTIVE_EPOCH: 18\nVALIDATOR_COUNT: 3\n"+testConfigYAML)

	config, err := LoadGenesisConfig(tmpFile)
	if err != nil {
		t.Fatalf("load leanchain config: %v", err)
	}
	if config.ActiveEpoch != 18 {
		t.Fatalf("ACTIVE_EPOCH not parsed: %d", config.ActiveEpoch)
	}
	if config.AttestationCommitteeCount == nil || *config.AttestationCommitteeCount != types.AttestationCommitteeCount {
		t.Fatalf("ATTESTATION_COMMITTEE_COUNT not parsed: %v", config.AttestationCommitteeCount)
	}
}

// Committee count is a shared network parameter; a multi-subnet devnet
// config.yaml must load rather than be rejected for differing from the default.
func TestLoadGenesisConfigAcceptsMultiSubnetCommitteeCount(t *testing.T) {
	for _, count := range []uint64{2, 8} {
		tmpFile := t.TempDir() + "/config.yaml"
		writeGenesisConfig(t, tmpFile, fmt.Sprintf("ATTESTATION_COMMITTEE_COUNT: %d\n", count)+testConfigYAML)

		config, err := LoadGenesisConfig(tmpFile)
		if err != nil {
			t.Fatalf("committee count %d: unexpected error: %v", count, err)
		}
		if config.AttestationCommitteeCount == nil || *config.AttestationCommitteeCount != count {
			t.Fatalf("committee count %d not parsed: %v", count, config.AttestationCommitteeCount)
		}
	}
}

func TestLoadGenesisConfigRejectsZeroCommitteeCount(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, "ATTESTATION_COMMITTEE_COUNT: 0\n"+testConfigYAML)

	if _, err := LoadGenesisConfig(tmpFile); err == nil {
		t.Fatal("expected error when ATTESTATION_COMMITTEE_COUNT is 0")
	}
}

func TestValidatorsReturnErrorsForDirectMalformedConfig(t *testing.T) {
	config := &GenesisConfig{
		GenesisTime: 1,
		GenesisValidators: []GenesisValidatorEntry{{
			AttestationPubkey: "not-hex",
			ProposalPubkey:    "also-not-hex",
		}},
	}

	validators, err := config.Validators()
	if err == nil {
		t.Fatalf("expected validator error, got validators=%v", validators)
	}
}

func TestGenesisStateReturnsValidatorErrors(t *testing.T) {
	config := &GenesisConfig{
		GenesisTime: 1,
		GenesisValidators: []GenesisValidatorEntry{{
			AttestationPubkey: "not-hex",
			ProposalPubkey:    "also-not-hex",
		}},
	}

	state, err := config.GenesisState()
	if err == nil {
		t.Fatalf("expected genesis state error, got state=%v", state)
	}
}

func TestLoadGenesisConfigRejectsUnknownFields(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, "UNKNOWN_FIELD: true\n"+testConfigYAML)

	config, err := LoadGenesisConfig(tmpFile)
	if err == nil {
		t.Fatalf("expected unknown field error, got config=%v", config)
	}
	if !strings.Contains(err.Error(), "UNKNOWN_FIELD") {
		t.Fatalf("error %q does not mention unknown field", err.Error())
	}
}

func TestLoadGenesisConfigRejectsMultipleDocuments(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	writeGenesisConfig(t, tmpFile, testConfigYAML+"\n---\nGENESIS_TIME: 1\n")

	config, err := LoadGenesisConfig(tmpFile)
	if err == nil {
		t.Fatalf("expected multiple document error, got config=%v", config)
	}
	if !strings.Contains(err.Error(), "single YAML document") {
		t.Fatalf("error %q does not mention single document", err.Error())
	}
}

func TestLoadGenesisConfigRejectsZeroPubkey(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	data := fmt.Sprintf(`GENESIS_TIME: 1770407233
GENESIS_VALIDATORS:
  - attestation_pubkey: "%s"
    proposal_pubkey: "%s"
`, pubkeyHex(0), pubkeyHex(1))
	writeGenesisConfig(t, tmpFile, data)

	config, err := LoadGenesisConfig(tmpFile)
	if err == nil {
		t.Fatalf("expected zero pubkey error, got config=%v", config)
	}
	if !strings.Contains(err.Error(), "zero pubkey") {
		t.Fatalf("error %q does not mention zero pubkey", err.Error())
	}
}

func TestLoadGenesisConfigRejectsDuplicatePubkeys(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	data := fmt.Sprintf(`GENESIS_TIME: 1770407233
GENESIS_VALIDATORS:
  - attestation_pubkey: "%s"
    proposal_pubkey: "%s"
  - attestation_pubkey: "%s"
    proposal_pubkey: "%s"
`, pubkeyHex(1), pubkeyHex(2), pubkeyHex(1), pubkeyHex(3))
	writeGenesisConfig(t, tmpFile, data)

	config, err := LoadGenesisConfig(tmpFile)
	if err == nil {
		t.Fatalf("expected duplicate pubkey error, got config=%v", config)
	}
	if !strings.Contains(err.Error(), "duplicate attestation pubkey") {
		t.Fatalf("error %q does not mention duplicate attestation pubkey", err.Error())
	}
}

func TestLoadGenesisConfigRejectsSameValidatorRolePubkey(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	key := pubkeyHex(1)
	data := fmt.Sprintf(`GENESIS_TIME: 1770407233
GENESIS_VALIDATORS:
  - attestation_pubkey: "%s"
    proposal_pubkey: "%s"
`, key, key)
	writeGenesisConfig(t, tmpFile, data)

	config, err := LoadGenesisConfig(tmpFile)
	if err == nil {
		t.Fatalf("expected duplicate role pubkey error, got config=%v", config)
	}
	if !strings.Contains(err.Error(), "duplicate proposal pubkey already used as attestation pubkey") {
		t.Fatalf("error %q does not mention cross-role duplicate", err.Error())
	}
}

func TestLoadGenesisConfigRejectsCrossValidatorRolePubkey(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	data := fmt.Sprintf(`GENESIS_TIME: 1770407233
GENESIS_VALIDATORS:
  - attestation_pubkey: "%s"
    proposal_pubkey: "%s"
  - attestation_pubkey: "%s"
    proposal_pubkey: "%s"
`, pubkeyHex(1), pubkeyHex(2), pubkeyHex(3), pubkeyHex(1))
	writeGenesisConfig(t, tmpFile, data)

	config, err := LoadGenesisConfig(tmpFile)
	if err == nil {
		t.Fatalf("expected cross-validator role pubkey error, got config=%v", config)
	}
	if !strings.Contains(err.Error(), "duplicate proposal pubkey already used as attestation pubkey at index 0") {
		t.Fatalf("error %q does not mention cross-validator role duplicate", err.Error())
	}
}

func TestGenesisConfigRejectsTooManyValidators(t *testing.T) {
	config := &GenesisConfig{
		GenesisTime:       1,
		GenesisValidators: make([]GenesisValidatorEntry, types.ValidatorRegistryLimit+1),
	}

	validators, err := config.Validators()
	if err == nil {
		t.Fatalf("expected validator limit error, got validators=%v", validators)
	}
	if !strings.Contains(err.Error(), "max") {
		t.Fatalf("error %q does not mention max validators", err.Error())
	}
}

func TestValidatorsAcceptUppercaseHexPrefix(t *testing.T) {
	config := &GenesisConfig{
		GenesisTime: 1,
		GenesisValidators: []GenesisValidatorEntry{{
			AttestationPubkey: "0X" + pubkeyHex(1),
			ProposalPubkey:    "0X" + pubkeyHex(2),
		}},
	}

	validators, err := config.Validators()
	if err != nil {
		t.Fatalf("validators: %v", err)
	}
	if len(validators) != 1 {
		t.Fatalf("validators=%d, want 1", len(validators))
	}
}

// GenesisState/Validators are pure constructors and must accept GenesisTime==0:
// leanSpec's API/test fixtures use genesisTime=0 for deterministic states. The
// time!=0 requirement is enforced on the config-file boundary only (see
// TestLoadGenesisConfigRejectsZeroGenesisTime).
func TestGenesisStateAcceptsZeroGenesisTime(t *testing.T) {
	config := &GenesisConfig{
		GenesisValidators: []GenesisValidatorEntry{{
			AttestationPubkey: pubkeyHex(1),
			ProposalPubkey:    pubkeyHex(2),
		}},
	}

	state, err := config.GenesisState()
	if err != nil {
		t.Fatalf("GenesisState with GenesisTime=0 and valid validators should succeed, got %v", err)
	}
	if state.Config.GenesisTime != 0 {
		t.Fatalf("expected GenesisTime=0 in state, got %d", state.Config.GenesisTime)
	}
}

// The slot clock needs a real anchor, so a running node's config.yaml with
// GENESIS_TIME=0 must be rejected at load time.
func TestLoadGenesisConfigRejectsZeroGenesisTime(t *testing.T) {
	tmpFile := t.TempDir() + "/config.yaml"
	zeroTimeYAML := "GENESIS_TIME: 0\n" + strings.SplitN(testConfigYAML, "\n", 2)[1]
	writeGenesisConfig(t, tmpFile, zeroTimeYAML)

	if _, err := LoadGenesisConfig(tmpFile); err == nil || !strings.Contains(err.Error(), "GENESIS_TIME") {
		t.Fatalf("expected GENESIS_TIME rejection at load, got %v", err)
	}
}

func writeGenesisConfig(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("write genesis config: %v", err)
	}
}
