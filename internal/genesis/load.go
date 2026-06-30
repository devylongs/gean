package genesis

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/geanlabs/gean/internal/types"
	"gopkg.in/yaml.v3"
)

func LoadGenesisConfig(path string) (*GenesisConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}

	var config GenesisConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("parse config.yaml: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("parse config.yaml: %w", err)
		}
		return nil, fmt.Errorf("parse config.yaml: expected a single YAML document")
	}
	// GENESIS_TIME is a running-node config requirement (the slot clock needs a
	// real anchor), enforced only here on the config-file boundary. It is NOT
	// part of validate(): GenesisState()/Validators() are pure constructors that
	// must accept GenesisTime==0 for deterministic spec/test states.
	if config.GenesisTime == 0 {
		return nil, fmt.Errorf("GENESIS_TIME is 0 or missing")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &config, nil
}

func (gc *GenesisConfig) validate() error {
	if gc == nil {
		return fmt.Errorf("genesis config is nil")
	}
	if len(gc.GenesisValidators) == 0 {
		return fmt.Errorf("GENESIS_VALIDATORS is empty")
	}
	if len(gc.GenesisValidators) > types.ValidatorRegistryLimit {
		return fmt.Errorf("GENESIS_VALIDATORS has %d validators (max %d)",
			len(gc.GenesisValidators), types.ValidatorRegistryLimit)
	}
	if gc.ValidatorCount != nil && *gc.ValidatorCount != uint64(len(gc.GenesisValidators)) {
		return fmt.Errorf("VALIDATOR_COUNT=%d disagrees with len(GENESIS_VALIDATORS)=%d",
			*gc.ValidatorCount, len(gc.GenesisValidators))
	}
	// ATTESTATION_COMMITTEE_COUNT is a shared network parameter, not a gean
	// constant: every node in a devnet must agree on it, so it is sourced from
	// the common config rather than pinned per client. Accept any positive
	// value (multi-subnet devnets run >1); reject 0, which would divide by zero
	// when mapping a validator to its subnet (validator_index % count).
	if gc.AttestationCommitteeCount != nil && *gc.AttestationCommitteeCount == 0 {
		return fmt.Errorf("ATTESTATION_COMMITTEE_COUNT must be >= 1")
	}

	attestationPubkeys := make(map[[types.PubkeySize]byte]int, len(gc.GenesisValidators))
	proposalPubkeys := make(map[[types.PubkeySize]byte]int, len(gc.GenesisValidators))
	rolePubkeys := make(map[[types.PubkeySize]byte]string, len(gc.GenesisValidators)*2)
	for i, entry := range gc.GenesisValidators {
		attestationPubkey, err := decodePubkey(entry.AttestationPubkey, i, "attestation")
		if err != nil {
			return err
		}
		proposalPubkey, err := decodePubkey(entry.ProposalPubkey, i, "proposal")
		if err != nil {
			return err
		}
		if prev, ok := attestationPubkeys[attestationPubkey]; ok {
			return fmt.Errorf("GENESIS_VALIDATORS[%d] duplicate attestation pubkey first seen at index %d", i, prev)
		}
		if prev, ok := proposalPubkeys[proposalPubkey]; ok {
			return fmt.Errorf("GENESIS_VALIDATORS[%d] duplicate proposal pubkey first seen at index %d", i, prev)
		}
		if prev, ok := rolePubkeys[attestationPubkey]; ok {
			return fmt.Errorf("GENESIS_VALIDATORS[%d] duplicate attestation pubkey already used as %s", i, prev)
		}
		rolePubkeys[attestationPubkey] = fmt.Sprintf("attestation pubkey at index %d", i)
		if prev, ok := rolePubkeys[proposalPubkey]; ok {
			return fmt.Errorf("GENESIS_VALIDATORS[%d] duplicate proposal pubkey already used as %s", i, prev)
		}
		rolePubkeys[proposalPubkey] = fmt.Sprintf("proposal pubkey at index %d", i)
		attestationPubkeys[attestationPubkey] = i
		proposalPubkeys[proposalPubkey] = i
	}
	return nil
}
