package xmss

// #cgo CFLAGS: -I.
// #cgo linux LDFLAGS: -L${SRCDIR}/rust/target/multisig-release -lcgo_glue -lm -ldl -lpthread
// #cgo darwin LDFLAGS: -L${SRCDIR}/rust/target/multisig-release -lcgo_glue -lm -ldl -lpthread -framework CoreFoundation -framework SystemConfiguration -framework Security
//
// #include <stdint.h>
// #include <stdlib.h>
// #include <stdbool.h>
//
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
//
// PublicKey* hashsig_public_key_from_ssz(const uint8_t* public_key_ptr, size_t public_key_len);
// void hashsig_public_key_free(PublicKey* public_key);
//
// Signature* hashsig_sign(const PrivateKey* private_key, const uint8_t* message_ptr, uint32_t epoch);
// void hashsig_signature_free(Signature* signature);
// size_t hashsig_signature_to_bytes(const Signature* signature, uint8_t* buffer, size_t buffer_len);
// Signature* hashsig_signature_from_ssz(const uint8_t* signature_ptr, size_t signature_len);
//
// int hashsig_verify(const PublicKey* public_key, const uint8_t* message_ptr,
//     uint32_t epoch, const Signature* signature);
// int hashsig_verify_ssz(const uint8_t* pubkey_bytes, size_t pubkey_len,
//     const uint8_t* message, uint32_t epoch,
//     const uint8_t* signature_bytes, size_t signature_len);
//
// size_t hashsig_public_key_to_bytes(const PublicKey* public_key, uint8_t* buffer, size_t buffer_len);
// size_t hashsig_private_key_to_bytes(const PrivateKey* private_key, uint8_t* buffer, size_t buffer_len);
// size_t hashsig_message_length();
//
// KeyPair* hashsig_keypair_generate(const char* seed_phrase,
//     size_t activation_epoch, size_t num_active_epochs);
//
// int32_t xmss_setup_prover();
// int32_t xmss_setup_verifier();
//
// int32_t xmss_aggregate_type_1(
//     const PublicKey* const* raw_pub_keys,
//     const Signature* const* raw_signatures,
//     size_t num_raw,
//     const PublicKey* const* child_all_pub_keys,
//     const size_t* child_num_keys,
//     const uint8_t* const* child_proof_ptrs,
//     const size_t* child_proof_lens,
//     size_t num_children,
//     const uint8_t* message_hash_ptr,
//     uint32_t slot,
//     size_t log_inv_rate,
//     uint8_t* out_buf, size_t out_cap, size_t* out_written);
//
// bool xmss_verify_type_1(
//     const PublicKey* const* public_keys, size_t num_keys,
//     const uint8_t* message_hash_ptr,
//     uint32_t slot,
//     const uint8_t* proof, size_t proof_len);
//
// int32_t xmss_merge_type_1_to_type_2(
//     const uint8_t* const* proof_ptrs, const size_t* proof_lens,
//     const PublicKey* const* pubkeys, const size_t* pubkey_counts,
//     size_t count, size_t log_inv_rate,
//     uint8_t* out_buf, size_t out_cap, size_t* out_written);
//
// int32_t xmss_split_type_2_by_message(
//     const uint8_t* proof, size_t proof_len,
//     const PublicKey* const* pubkeys, const size_t* pubkey_counts,
//     size_t count, const uint8_t* target_message, size_t log_inv_rate,
//     uint8_t* out_buf, size_t out_cap, size_t* out_written);
//
// bool xmss_verify_type_2(
//     const uint8_t* proof, size_t proof_len,
//     const PublicKey* const* pubkeys, const size_t* pubkey_counts,
//     size_t count, const uint8_t* message_hashes, const uint32_t* message_slots);
//
// int32_t poseidon_permute_kb16(uint32_t* state, size_t len);
// int32_t poseidon_permute_kb24(uint32_t* state, size_t len);
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/geanlabs/gean/internal/types"
)

const (
	MessageLength    = 32
	MaxProofSize     = 1 << 19
	SignatureBuffer  = 4000
	PubkeyBuffer     = 256
	PrivateKeyBuffer = 10 << 20
)

var (
	ErrEmptyInput          = errors.New("empty input")
	ErrCountMismatch       = errors.New("public key count does not match signature count")
	ErrAggregationFailed   = errors.New("signature aggregation failed")
	ErrSerializationFailed = errors.New("proof serialization failed")
	ErrProofTooBig         = errors.New("aggregated proof exceeds 512 KiB")
	ErrVerificationFailed  = errors.New("aggregated signature verification failed")
	ErrDeserializeFailed   = errors.New("proof deserialization failed")
	ErrSigningFailed       = errors.New("signing failed")
	ErrInvalidSignature    = errors.New("signature verification returned invalid")
	ErrSignatureError      = errors.New("signature verification error (malformed data)")
	ErrPubkeyParseFailed   = errors.New("public key parsing failed")
	ErrKeypairParseFailed  = errors.New("keypair parsing failed")
	ErrMalformedChildProof = errors.New("malformed child proof")
	ErrMalformedRawInput   = errors.New("malformed raw signature input")
	ErrSetupFailed         = errors.New("XMSS setup failed")
	ErrPoseidonWidth       = errors.New("poseidon permutation width must be 16 or 24")
	ErrPoseidonPermute     = errors.New("poseidon permutation failed")
)

// Poseidon2Permute applies the KoalaBear Poseidon permutation in place. The
// state holds canonical field-element values; width must be 16 or 24.
func Poseidon2Permute(state []uint32) error {
	if len(state) != 16 && len(state) != 24 {
		return ErrPoseidonWidth
	}
	ptr := (*C.uint32_t)(unsafe.Pointer(&state[0]))
	n := C.size_t(len(state))
	var status C.int32_t
	if len(state) == 16 {
		status = C.poseidon_permute_kb16(ptr, n)
	} else {
		status = C.poseidon_permute_kb24(ptr, n)
	}
	if status != 0 {
		return ErrPoseidonPermute
	}
	return nil
}

var (
	proverOnce   sync.Once
	verifierOnce sync.Once
)

var proverErr, verifierErr error

func EnsureProverReady() error {
	proverOnce.Do(func() {
		if C.xmss_setup_prover() != 0 {
			proverErr = ErrSetupFailed
		}
	})
	return proverErr
}

func EnsureVerifierReady() error {
	verifierOnce.Do(func() {
		if C.xmss_setup_verifier() != 0 {
			verifierErr = ErrSetupFailed
		}
	})
	return verifierErr
}

func VerifySignatureSSZ(pubkey [types.PubkeySize]byte, slot uint32, message [32]byte, signature [types.SignatureSize]byte) (bool, error) {
	result := C.hashsig_verify_ssz(
		(*C.uint8_t)(unsafe.Pointer(&pubkey[0])), C.size_t(types.PubkeySize),
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		C.uint32_t(slot),
		(*C.uint8_t)(unsafe.Pointer(&signature[0])), C.size_t(types.SignatureSize),
	)

	switch result {
	case 1:
		return true, nil
	case 0:
		return false, nil
	default:
		return false, ErrSignatureError
	}
}

type ChildProof struct {
	Pubkeys []CPubKey
	Proof   []byte
}

const LogInvRate = 2

func AggregateSignatures(
	pubkeys []CPubKey,
	sigs []CSig,
	message [32]byte,
	slot uint32,
) ([]byte, error) {
	return AggregateWithChildren(pubkeys, sigs, nil, message, slot)
}

func AggregateWithChildren(
	pubkeys []CPubKey,
	sigs []CSig,
	children []ChildProof,
	message [32]byte,
	slot uint32,
) ([]byte, error) {
	numRaw := len(pubkeys)
	numChildren := len(children)

	if numRaw == 0 && numChildren == 0 {
		return nil, ErrEmptyInput
	}
	if numRaw > 0 && len(sigs) != numRaw {
		return nil, fmt.Errorf("%w: %d pubkeys, %d sigs", ErrCountMismatch, numRaw, len(sigs))
	}
	if err := validateRawInputs(pubkeys, sigs); err != nil {
		return nil, err
	}
	if numRaw == 0 && numChildren < 2 {
		return nil, fmt.Errorf("at least 2 children required when no raw sigs provided")
	}
	if err := validateChildProofs(children); err != nil {
		return nil, err
	}

	if err := EnsureProverReady(); err != nil {
		return nil, err
	}

	// Pin Go memory passed to C. Go 1.21+ cgo checks reject passing slices
	// containing Go pointers unless the backing memory is pinned.
	var pinner runtime.Pinner
	defer pinner.Unpin()

	var rawPkPtr **C.PublicKey
	var rawSigPtr **C.Signature
	if numRaw > 0 {
		cPubkeys := make([]*C.PublicKey, numRaw)
		for i, pk := range pubkeys {
			cPubkeys[i] = (*C.PublicKey)(pk)
		}
		cSigs := make([]*C.Signature, numRaw)
		for i, s := range sigs {
			cSigs[i] = (*C.Signature)(s)
		}
		pinner.Pin(&cPubkeys[0])
		pinner.Pin(&cSigs[0])
		rawPkPtr = (**C.PublicKey)(unsafe.Pointer(&cPubkeys[0]))
		rawSigPtr = (**C.Signature)(unsafe.Pointer(&cSigs[0]))
	}

	var childAllPkPtr **C.PublicKey
	var childNumKeysPtr *C.size_t
	var childProofPtrsPtr **C.uint8_t
	var childProofLensPtr *C.size_t

	if numChildren > 0 {
		var allChildPks []*C.PublicKey
		childNumKeys := make([]C.size_t, numChildren)
		childProofPtrs := make([]*C.uint8_t, numChildren)
		childProofLens := make([]C.size_t, numChildren)

		for i, child := range children {
			childNumKeys[i] = C.size_t(len(child.Pubkeys))
			for _, pk := range child.Pubkeys {
				allChildPks = append(allChildPks, (*C.PublicKey)(pk))
			}
			pinner.Pin(&child.Proof[0])
			childProofPtrs[i] = (*C.uint8_t)(unsafe.Pointer(&child.Proof[0]))
			childProofLens[i] = C.size_t(len(child.Proof))
		}

		if len(allChildPks) > 0 {
			pinner.Pin(&allChildPks[0])
			childAllPkPtr = (**C.PublicKey)(unsafe.Pointer(&allChildPks[0]))
		}
		pinner.Pin(&childProofPtrs[0])
		childNumKeysPtr = (*C.size_t)(unsafe.Pointer(&childNumKeys[0]))
		childProofPtrsPtr = (**C.uint8_t)(unsafe.Pointer(&childProofPtrs[0]))
		childProofLensPtr = (*C.size_t)(unsafe.Pointer(&childProofLens[0]))
	}

	bufPtr := getProofBuf()
	defer putProofBuf(bufPtr)
	buf := *bufPtr
	var written C.size_t
	status := C.xmss_aggregate_type_1(
		rawPkPtr,
		rawSigPtr,
		C.size_t(numRaw),
		childAllPkPtr,
		childNumKeysPtr,
		childProofPtrsPtr,
		childProofLensPtr,
		C.size_t(numChildren),
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		C.uint32_t(slot),
		C.size_t(LogInvRate),
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
		&written,
	)
	if status != 0 {
		if status == -2 || int(written) > MaxProofSize {
			return nil, ErrProofTooBig
		}
		return nil, ErrAggregationFailed
	}
	if written == 0 {
		return nil, ErrSerializationFailed
	}
	if int(written) > MaxProofSize {
		return nil, ErrProofTooBig
	}

	result := make([]byte, int(written))
	copy(result, buf[:written])
	return result, nil
}

func validateRawInputs(pubkeys []CPubKey, sigs []CSig) error {
	if err := validatePublicKeys(pubkeys); err != nil {
		return err
	}
	for i, sig := range sigs {
		if sig == nil {
			return fmt.Errorf("%w: signature %d is nil", ErrMalformedRawInput, i)
		}
	}
	return nil
}

func validatePublicKeys(pubkeys []CPubKey) error {
	for i, pk := range pubkeys {
		if pk == nil {
			return fmt.Errorf("%w: pubkey %d is nil", ErrMalformedRawInput, i)
		}
	}
	return nil
}

func validateChildProofs(children []ChildProof) error {
	for i, child := range children {
		if len(child.Pubkeys) == 0 {
			return fmt.Errorf("%w: child %d has no pubkeys", ErrMalformedChildProof, i)
		}
		if len(child.Proof) == 0 {
			return fmt.Errorf("%w: child %d proof data is empty", ErrMalformedChildProof, i)
		}
		for j, pk := range child.Pubkeys {
			if pk == nil {
				return fmt.Errorf("%w: child %d pubkey %d is nil", ErrMalformedChildProof, i, j)
			}
		}
	}
	return nil
}

func VerifyAggregatedSignature(
	proofData []byte,
	pubkeys []CPubKey,
	message [32]byte,
	slot uint32,
) error {
	if len(proofData) == 0 || len(pubkeys) == 0 {
		return ErrEmptyInput
	}
	if err := validatePublicKeys(pubkeys); err != nil {
		return err
	}

	if len(proofData) > MaxProofSize {
		return ErrProofTooBig
	}
	if err := EnsureVerifierReady(); err != nil {
		return err
	}

	cPubkeys := make([]*C.PublicKey, len(pubkeys))
	for i, pk := range pubkeys {
		cPubkeys[i] = (*C.PublicKey)(pk)
	}

	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(&cPubkeys[0])

	valid := C.xmss_verify_type_1(
		(**C.PublicKey)(unsafe.Pointer(&cPubkeys[0])),
		C.size_t(len(cPubkeys)),
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		C.uint32_t(slot),
		(*C.uint8_t)(unsafe.Pointer(&proofData[0])),
		C.size_t(len(proofData)),
	)
	if !valid {
		return ErrVerificationFailed
	}

	return nil
}

type Type1Input struct {
	Pubkeys []CPubKey
	Proof   []byte
}

type MessageBinding struct {
	Message [MessageLength]byte
	Slot    uint32
}

func MergeType1Proofs(inputs []Type1Input) ([]byte, error) {
	if len(inputs) == 0 {
		return nil, ErrEmptyInput
	}
	if err := EnsureProverReady(); err != nil {
		return nil, err
	}

	var pinner runtime.Pinner
	defer pinner.Unpin()

	proofPtrs := make([]*C.uint8_t, len(inputs))
	proofLens := make([]C.size_t, len(inputs))
	keyCounts := make([]C.size_t, len(inputs))
	var keys []*C.PublicKey
	for i, input := range inputs {
		if len(input.Proof) == 0 || len(input.Proof) > MaxProofSize {
			return nil, ErrMalformedChildProof
		}
		if err := validatePublicKeys(input.Pubkeys); err != nil {
			return nil, err
		}
		pinner.Pin(&input.Proof[0])
		proofPtrs[i] = (*C.uint8_t)(unsafe.Pointer(&input.Proof[0]))
		proofLens[i] = C.size_t(len(input.Proof))
		keyCounts[i] = C.size_t(len(input.Pubkeys))
		for _, key := range input.Pubkeys {
			keys = append(keys, (*C.PublicKey)(key))
		}
	}
	if len(keys) == 0 {
		return nil, ErrEmptyInput
	}
	pinner.Pin(&proofPtrs[0])
	pinner.Pin(&keys[0])

	bufPtr := getProofBuf()
	defer putProofBuf(bufPtr)
	buf := *bufPtr
	var written C.size_t
	status := C.xmss_merge_type_1_to_type_2(
		(**C.uint8_t)(unsafe.Pointer(&proofPtrs[0])),
		(*C.size_t)(unsafe.Pointer(&proofLens[0])),
		(**C.PublicKey)(unsafe.Pointer(&keys[0])),
		(*C.size_t)(unsafe.Pointer(&keyCounts[0])),
		C.size_t(len(inputs)),
		C.size_t(LogInvRate),
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
		&written,
	)
	return proofResult(status, written, buf)
}

func SplitType2Proof(
	proof []byte,
	pubkeys [][]CPubKey,
	target [MessageLength]byte,
) ([]byte, error) {
	if len(proof) == 0 || len(pubkeys) == 0 {
		return nil, ErrEmptyInput
	}
	if len(proof) > MaxProofSize {
		return nil, ErrProofTooBig
	}
	if err := EnsureProverReady(); err != nil {
		return nil, err
	}

	keys, counts, err := flattenPublicKeys(pubkeys)
	if err != nil {
		return nil, err
	}
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(&proof[0])
	pinner.Pin(&keys[0])

	bufPtr := getProofBuf()
	defer putProofBuf(bufPtr)
	buf := *bufPtr
	var written C.size_t
	status := C.xmss_split_type_2_by_message(
		(*C.uint8_t)(unsafe.Pointer(&proof[0])),
		C.size_t(len(proof)),
		(**C.PublicKey)(unsafe.Pointer(&keys[0])),
		(*C.size_t)(unsafe.Pointer(&counts[0])),
		C.size_t(len(pubkeys)),
		(*C.uint8_t)(unsafe.Pointer(&target[0])),
		C.size_t(LogInvRate),
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
		&written,
	)
	return proofResult(status, written, buf)
}

func VerifyType2Proof(
	proof []byte,
	pubkeys [][]CPubKey,
	bindings []MessageBinding,
) error {
	if len(proof) == 0 || len(pubkeys) == 0 || len(pubkeys) != len(bindings) {
		return ErrCountMismatch
	}
	if len(proof) > MaxProofSize {
		return ErrProofTooBig
	}
	if err := EnsureVerifierReady(); err != nil {
		return err
	}
	keys, counts, err := flattenPublicKeys(pubkeys)
	if err != nil {
		return err
	}

	hashes := make([]byte, 0, len(bindings)*MessageLength)
	slots := make([]C.uint32_t, len(bindings))
	for i, binding := range bindings {
		hashes = append(hashes, binding.Message[:]...)
		slots[i] = C.uint32_t(binding.Slot)
	}

	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(&proof[0])
	pinner.Pin(&keys[0])
	pinner.Pin(&hashes[0])

	if !C.xmss_verify_type_2(
		(*C.uint8_t)(unsafe.Pointer(&proof[0])),
		C.size_t(len(proof)),
		(**C.PublicKey)(unsafe.Pointer(&keys[0])),
		(*C.size_t)(unsafe.Pointer(&counts[0])),
		C.size_t(len(pubkeys)),
		(*C.uint8_t)(unsafe.Pointer(&hashes[0])),
		(*C.uint32_t)(unsafe.Pointer(&slots[0])),
	) {
		return ErrVerificationFailed
	}
	return nil
}

func flattenPublicKeys(groups [][]CPubKey) ([]*C.PublicKey, []C.size_t, error) {
	counts := make([]C.size_t, len(groups))
	var keys []*C.PublicKey
	for i, group := range groups {
		if len(group) == 0 {
			return nil, nil, ErrEmptyInput
		}
		if err := validatePublicKeys(group); err != nil {
			return nil, nil, err
		}
		counts[i] = C.size_t(len(group))
		for _, key := range group {
			keys = append(keys, (*C.PublicKey)(key))
		}
	}
	return keys, counts, nil
}

func proofResult(status C.int32_t, written C.size_t, buf []byte) ([]byte, error) {
	if status != 0 {
		if status == -2 || int(written) > MaxProofSize {
			return nil, ErrProofTooBig
		}
		return nil, ErrAggregationFailed
	}
	if written == 0 {
		return nil, ErrSerializationFailed
	}
	result := make([]byte, int(written))
	copy(result, buf[:written])
	return result, nil
}

type CPubKey = unsafe.Pointer

type CSig = unsafe.Pointer

func ParsePublicKey(pubkeyBytes [types.PubkeySize]byte) (CPubKey, error) {
	pk := C.hashsig_public_key_from_ssz(
		(*C.uint8_t)(unsafe.Pointer(&pubkeyBytes[0])),
		C.size_t(types.PubkeySize),
	)
	if pk == nil {
		return nil, ErrPubkeyParseFailed
	}
	return unsafe.Pointer(pk), nil
}

func FreePublicKey(pk CPubKey) {
	if pk != nil {
		C.hashsig_public_key_free((*C.PublicKey)(pk))
	}
}

func ParseSignature(sigBytes []byte) (CSig, error) {
	if len(sigBytes) == 0 {
		return nil, ErrInvalidSignature
	}
	sig := C.hashsig_signature_from_ssz(
		(*C.uint8_t)(unsafe.Pointer(&sigBytes[0])),
		C.size_t(len(sigBytes)),
	)
	if sig == nil {
		return nil, ErrInvalidSignature
	}
	return unsafe.Pointer(sig), nil
}

func FreeSignature(sig CSig) {
	if sig != nil {
		C.hashsig_signature_free((*C.Signature)(sig))
	}
}

func GenerateKeyPair(seedPhrase string, activationEpoch, numActiveEpochs uint64) (*ValidatorKeyPair, error) {
	cSeed := C.CString(seedPhrase)
	defer C.free(unsafe.Pointer(cSeed))

	kp := C.hashsig_keypair_generate(cSeed, C.size_t(activationEpoch), C.size_t(numActiveEpochs))
	if kp == nil {
		return nil, ErrKeypairParseFailed
	}
	return &ValidatorKeyPair{handle: kp, Index: 0}, nil
}

func (kp *ValidatorKeyPair) PublicKeyBytes() ([types.PubkeySize]byte, error) {
	var result [types.PubkeySize]byte
	publicKey := kp.PublicKeyPtr()
	if publicKey == nil {
		return result, fmt.Errorf("keypair is nil or closed")
	}
	var buf [PubkeyBuffer]byte

	n := C.hashsig_public_key_to_bytes(
		publicKey,
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
	)
	if n == 0 || int(n) != types.PubkeySize {
		return result, fmt.Errorf("pubkey serialization failed: got %d bytes", n)
	}
	copy(result[:], buf[:n])
	return result, nil
}

func (kp *ValidatorKeyPair) PrivateKeyBytes() ([]byte, error) {
	privateKey := kp.PrivateKeyPtr()
	if privateKey == nil {
		return nil, fmt.Errorf("keypair is nil or closed")
	}
	buf := make([]byte, PrivateKeyBuffer)
	n := C.hashsig_private_key_to_bytes(
		privateKey,
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
	)
	if n == 0 {
		return nil, fmt.Errorf("private key serialization failed")
	}
	return buf[:n], nil
}
