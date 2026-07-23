package xmss

// #include <stdint.h>
// #include <stdlib.h>
// typedef struct KeyPair KeyPair;
// typedef struct PublicKey PublicKey;
// typedef struct PrivateKey PrivateKey;
// typedef struct Signature Signature;
//
// KeyPair* hashsig_keypair_from_ssz(
//     const uint8_t* private_key_ptr, size_t private_key_len,
//     const uint8_t* public_key_ptr, size_t public_key_len);
// void hashsig_keypair_free(KeyPair* keypair);
// const PublicKey* hashsig_keypair_get_public_key(const KeyPair* keypair);
// const PrivateKey* hashsig_keypair_get_private_key(const KeyPair* keypair);
// Signature* hashsig_sign(const PrivateKey* private_key, const uint8_t* message_ptr, uint32_t epoch);
// void hashsig_signature_free(Signature* signature);
// size_t hashsig_signature_to_bytes(const Signature* signature, uint8_t* buffer, size_t buffer_len);
import "C"

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"

	"github.com/geanlabs/gean/internal/types"
	"gopkg.in/yaml.v3"
)

type ValidatorKeyPair struct {
	handle *C.KeyPair
	Index  uint64
}

func (kp *ValidatorKeyPair) PublicKeyPtr() *C.PublicKey {
	if kp == nil || kp.handle == nil {
		return nil
	}
	return C.hashsig_keypair_get_public_key(kp.handle)
}

func (kp *ValidatorKeyPair) PublicKey() CPubKey {
	return unsafe.Pointer(kp.PublicKeyPtr())
}

func (kp *ValidatorKeyPair) PrivateKeyPtr() *C.PrivateKey {
	if kp == nil || kp.handle == nil {
		return nil
	}
	return C.hashsig_keypair_get_private_key(kp.handle)
}

func (kp *ValidatorKeyPair) Sign(slot uint32, message [32]byte) ([types.SignatureSize]byte, error) {
	var result [types.SignatureSize]byte
	privateKey := kp.PrivateKeyPtr()
	if privateKey == nil {
		return result, fmt.Errorf("%w: keypair is nil or closed", ErrSigningFailed)
	}

	sigPtr := C.hashsig_sign(
		privateKey,
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		C.uint32_t(slot),
	)
	if sigPtr == nil {
		return result, fmt.Errorf("%w: validator %d slot %d", ErrSigningFailed, kp.Index, slot)
	}
	defer C.hashsig_signature_free(sigPtr)

	buf := make([]byte, SignatureBuffer)
	n := C.hashsig_signature_to_bytes(
		sigPtr,
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
	)
	if n == 0 || int(n) != types.SignatureSize {
		return result, fmt.Errorf("signature serialization failed: wrote %d bytes, expected %d", n, types.SignatureSize)
	}

	copy(result[:], buf[:n])
	return result, nil
}

func (kp *ValidatorKeyPair) Close() {
	if kp != nil && kp.handle != nil {
		C.hashsig_keypair_free(kp.handle)
		kp.handle = nil
	}
}

type KeyManager struct {
	attestationKeys map[uint64]*ValidatorKeyPair
	proposalKeys    map[uint64]*ValidatorKeyPair
}

func NewKeyManager(attestationKeys, proposalKeys map[uint64]*ValidatorKeyPair) *KeyManager {
	return &KeyManager{
		attestationKeys: attestationKeys,
		proposalKeys:    proposalKeys,
	}
}

func (km *KeyManager) ValidatorIDs() []uint64 {
	if km == nil {
		return nil
	}
	ids := make([]uint64, 0, len(km.attestationKeys))
	for id := range km.attestationKeys {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}

func (km *KeyManager) GetAttestationKey(validatorID uint64) *ValidatorKeyPair {
	if km == nil {
		return nil
	}
	return km.attestationKeys[validatorID]
}

func (km *KeyManager) GetProposalKey(validatorID uint64) *ValidatorKeyPair {
	if km == nil {
		return nil
	}
	return km.proposalKeys[validatorID]
}

func (km *KeyManager) SignAttestation(validatorID uint64, data *types.AttestationData) ([types.SignatureSize]byte, error) {
	if km == nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("key manager is nil")
	}
	kp := km.GetAttestationKey(validatorID)
	if kp == nil || kp.handle == nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("attestation key for validator %d not found", validatorID)
	}
	if data == nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("attestation data is nil")
	}

	msgRoot, err := data.HashTreeRoot()
	if err != nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("hash tree root failed: %w", err)
	}

	slot := uint32(data.Slot)
	if uint64(slot) != data.Slot {
		return [types.SignatureSize]byte{}, fmt.Errorf("slot %d overflows uint32", data.Slot)
	}

	return kp.Sign(slot, msgRoot)
}

func (km *KeyManager) SignBlock(validatorID uint64, slot uint64, blockRoot [32]byte) ([types.SignatureSize]byte, error) {
	if km == nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("key manager is nil")
	}
	kp := km.GetProposalKey(validatorID)
	if kp == nil || kp.handle == nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("proposal key for validator %d not found", validatorID)
	}

	s := uint32(slot)
	if uint64(s) != slot {
		return [types.SignatureSize]byte{}, fmt.Errorf("slot %d overflows uint32", slot)
	}

	return kp.Sign(s, blockRoot)
}

func (km *KeyManager) Close() {
	if km == nil {
		return
	}
	for _, kp := range km.attestationKeys {
		kp.Close()
	}
	for _, kp := range km.proposalKeys {
		kp.Close()
	}
}

type annotatedValidator struct {
	Index             uint64 `yaml:"index"`
	PubkeyHex         string `yaml:"pubkey_hex"`
	PrivkeyFile       string `yaml:"privkey_file"`
	AttestationPubkey string `yaml:"attestation_pubkey_hex"`
	ProposalPubkey    string `yaml:"proposal_pubkey_hex"`
	AttestationSkFile string `yaml:"attestation_sk_file"`
	ProposalSkFile    string `yaml:"proposal_sk_file"`
}

func LoadValidatorKeys(annotatedPath, keysDir, nodeID string) (*KeyManager, error) {
	data, err := os.ReadFile(annotatedPath)
	if err != nil {
		return nil, fmt.Errorf("read annotated validators: %w", err)
	}

	var allValidators map[string][]annotatedValidator
	if err := yaml.Unmarshal(data, &allValidators); err != nil {
		return nil, fmt.Errorf("parse annotated validators: %w", err)
	}

	validators, ok := allValidators[nodeID]
	if !ok {
		return nil, fmt.Errorf("node ID %q not found in annotated validators", nodeID)
	}

	attestationKeys := make(map[uint64]*ValidatorKeyPair)
	proposalKeys := make(map[uint64]*ValidatorKeyPair)

	for _, v := range validators {
		if v.PrivkeyFile != "" {
			kp, err := loadKeypair(keysDir, v.PrivkeyFile, v.PubkeyHex, v.Index)
			if err != nil {
				return nil, fmt.Errorf("load key for validator %d (%s): %w", v.Index, v.PrivkeyFile, err)
			}
			if strings.Contains(v.PrivkeyFile, "attester") || strings.Contains(v.PrivkeyFile, "attestation") {
				attestationKeys[v.Index] = kp
			} else if strings.Contains(v.PrivkeyFile, "proposer") || strings.Contains(v.PrivkeyFile, "proposal") {
				proposalKeys[v.Index] = kp
			} else {
				if attestationKeys[v.Index] == nil {
					attestationKeys[v.Index] = kp
				}
				if proposalKeys[v.Index] == nil {
					proposalKeys[v.Index] = kp
				}
			}
		} else if v.AttestationSkFile != "" || v.ProposalSkFile != "" {
			if v.AttestationSkFile == "" {
				return nil, fmt.Errorf("attestation key file missing for validator %d", v.Index)
			}
			if v.ProposalSkFile == "" {
				return nil, fmt.Errorf("proposal key file missing for validator %d", v.Index)
			}
			attKp, err := loadKeypair(keysDir, v.AttestationSkFile, v.AttestationPubkey, v.Index)
			if err != nil {
				return nil, fmt.Errorf("load attestation key for validator %d: %w", v.Index, err)
			}
			attestationKeys[v.Index] = attKp

			propKp, err := loadKeypair(keysDir, v.ProposalSkFile, v.ProposalPubkey, v.Index)
			if err != nil {
				return nil, fmt.Errorf("load proposal key for validator %d: %w", v.Index, err)
			}
			proposalKeys[v.Index] = propKp
		}
	}

	return NewKeyManager(attestationKeys, proposalKeys), nil
}

func loadKeypair(keysDir, skFile, pubkeyHex string, index uint64) (*ValidatorKeyPair, error) {
	skPath := skFile
	if !filepath.IsAbs(skPath) {
		skPath = filepath.Join(keysDir, skFile)
	}

	skBytes, err := os.ReadFile(skPath)
	if err != nil {
		return nil, fmt.Errorf("read secret key: %w", err)
	}
	if len(skBytes) == 0 {
		return nil, fmt.Errorf("secret key is empty")
	}

	pkHex := strings.TrimSpace(pubkeyHex)
	if len(pkHex) >= 2 && pkHex[0] == '0' && (pkHex[1] == 'x' || pkHex[1] == 'X') {
		pkHex = pkHex[2:]
	}
	pkBytes, err := hex.DecodeString(pkHex)
	if err != nil {
		return nil, fmt.Errorf("decode pubkey hex: %w", err)
	}
	if len(pkBytes) != types.PubkeySize {
		return nil, fmt.Errorf("pubkey has %d bytes, expected %d", len(pkBytes), types.PubkeySize)
	}

	handle := C.hashsig_keypair_from_ssz(
		(*C.uint8_t)(unsafe.Pointer(&skBytes[0])), C.size_t(len(skBytes)),
		(*C.uint8_t)(unsafe.Pointer(&pkBytes[0])), C.size_t(len(pkBytes)),
	)
	if handle == nil {
		return nil, fmt.Errorf("%w: validator %d", ErrKeypairParseFailed, index)
	}

	return &ValidatorKeyPair{handle: handle, Index: index}, nil
}
