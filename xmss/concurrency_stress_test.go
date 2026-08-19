package xmss

import (
	"fmt"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/types"
)

// prepareStressInputs generates numSigs keypairs, signs one message with each,
// and returns the serialized public keys and signatures. Handles are parsed
// per-call in the workers so concurrent aggregations never share a handle —
// the realistic model for parallel group proving, where each group owns its
// signatures.
func prepareStressInputs(t *testing.T, numSigs int) ([][types.PubkeySize]byte, [][types.SignatureSize]byte, [32]byte) {
	t.Helper()
	var message [32]byte
	message[0] = 0x7a
	pkList := make([][types.PubkeySize]byte, numSigs)
	sigList := make([][types.SignatureSize]byte, numSigs)
	for i := range numSigs {
		kp, err := GenerateKeyPair(fmt.Sprintf("stress-%d", i), 0, 1<<16)
		if err != nil {
			t.Fatalf("keygen %d: %v", i, err)
		}
		sig, err := kp.Sign(0, message)
		if err != nil {
			kp.Close()
			t.Fatalf("sign %d: %v", i, err)
		}
		pk, err := kp.PublicKeyBytes()
		kp.Close()
		if err != nil {
			t.Fatalf("pubkey %d: %v", i, err)
		}
		pkList[i] = pk
		sigList[i] = sig
	}
	return pkList, sigList, message
}

// aggregateFresh parses its own handles, aggregates, frees them, and returns the
// proof. Distinct handles per call isolate prover reentrancy from handle sharing.
func aggregateFresh(pkList [][types.PubkeySize]byte, sigList [][types.SignatureSize]byte, message [32]byte) ([]byte, error) {
	n := len(pkList)
	pks := make([]CPubKey, 0, n)
	sigs := make([]CSig, 0, n)
	defer func() {
		for _, p := range pks {
			FreePublicKey(p)
		}
		for _, s := range sigs {
			FreeSignature(s)
		}
	}()
	for i := range n {
		cpk, err := ParsePublicKey(pkList[i])
		if err != nil {
			return nil, err
		}
		pks = append(pks, cpk)
		csig, err := ParseSignature(sigList[i][:])
		if err != nil {
			return nil, err
		}
		sigs = append(sigs, csig)
	}
	return AggregateSignatures(pks, sigs, message, 0)
}

func maxRSSBytes() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	// darwin reports bytes, linux reports kilobytes.
	return int64(ru.Maxrss)
}

// TestFFIConcurrentAggregation answers whether parallel group proving is viable:
// is the aggregate FFI reentrant (no crash, all proofs valid) and does the prover
// leave idle cores (so concurrency would raise throughput)? It bypasses the
// proving gate exactly as parallel proving would.
func TestFFIConcurrentAggregation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow FFI proving stress test in -short")
	}
	const numSigs = 8
	const proofs = 6
	const parallelism = 4

	pkList, sigList, message := prepareStressInputs(t, numSigs)
	EnsureProverReady()
	EnsureVerifierReady()

	// Warm one proof so the comparison excludes any first-call lazy init.
	if _, err := aggregateFresh(pkList, sigList, message); err != nil {
		t.Fatalf("warmup aggregate: %v", err)
	}

	verify := func(proof []byte) error {
		pks := make([]CPubKey, 0, numSigs)
		defer func() {
			for _, p := range pks {
				FreePublicKey(p)
			}
		}()
		for i := range numSigs {
			cpk, err := ParsePublicKey(pkList[i])
			if err != nil {
				return err
			}
			pks = append(pks, cpk)
		}
		return VerifyAggregatedSignature(proof, pks, message, 0)
	}

	rssBefore := maxRSSBytes()

	// Serial baseline.
	serialStart := time.Now()
	for i := range proofs {
		proof, err := aggregateFresh(pkList, sigList, message)
		if err != nil {
			t.Fatalf("serial aggregate %d: %v", i, err)
		}
		if err := verify(proof); err != nil {
			t.Fatalf("serial proof %d invalid: %v", i, err)
		}
	}
	serialDur := time.Since(serialStart)

	// Parallel: same number of proofs across `parallelism` goroutines, distinct
	// handles each. A crash here means the FFI is not reentrant; an invalid proof
	// means it is not correct under concurrency.
	rssMidRSS := maxRSSBytes()
	var wg sync.WaitGroup
	errCh := make(chan error, proofs)
	sem := make(chan struct{}, parallelism)
	parStart := time.Now()
	for i := range proofs {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			proof, err := aggregateFresh(pkList, sigList, message)
			if err != nil {
				errCh <- fmt.Errorf("parallel aggregate %d: %w", idx, err)
				return
			}
			if err := verify(proof); err != nil {
				errCh <- fmt.Errorf("parallel proof %d invalid: %w", idx, err)
			}
		}(i)
	}
	wg.Wait()
	parDur := time.Since(parStart)
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrency failure: %v", err)
		}
	}

	rssAfter := maxRSSBytes()
	speedup := serialDur.Seconds() / parDur.Seconds()

	t.Logf("FFI concurrency: %d sigs/proof, %d proofs, parallelism=%d", numSigs, proofs, parallelism)
	t.Logf("  serial   total=%v  per-proof=%v", serialDur, serialDur/proofs)
	t.Logf("  parallel total=%v  per-proof=%v", parDur, parDur/proofs)
	t.Logf("  speedup (serial/parallel) = %.2fx  (≈%d ⇒ prover single-threaded, ≈1 ⇒ already core-saturated)", speedup, parallelism)
	t.Logf("  maxRSS: before=%dMB midSerial=%dMB after=%dMB (darwin bytes / linux KB)",
		rssBefore>>20, rssMidRSS>>20, rssAfter>>20)
}
