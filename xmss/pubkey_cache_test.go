package xmss

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestPubKeyCacheGetAndReuse(t *testing.T) {
	cache := NewPubKeyCache()
	defer cache.Close()

	var pubkey [types.PubkeySize]byte
	pubkey[0] = 0x01
	pubkey[1] = 0xFF

	pk1, err := cache.Get(pubkey)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if pk1 == nil {
		t.Fatal("first Get returned nil")
	}

	pk2, err := cache.Get(pubkey)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if pk1 != pk2 {
		t.Fatal("expected same pointer from cache")
	}

	if cache.Len() != 1 {
		t.Fatalf("expected 1 cached entry, got %d", cache.Len())
	}
}

func TestPubKeyCacheMultipleKeys(t *testing.T) {
	cache := NewPubKeyCache()
	defer cache.Close()

	var pk1, pk2 [types.PubkeySize]byte
	pk1[0] = 0x01
	pk2[0] = 0x02

	h1, err := cache.Get(pk1)
	if err != nil {
		t.Fatalf("get pk1: %v", err)
	}
	h2, err := cache.Get(pk2)
	if err != nil {
		t.Fatalf("get pk2: %v", err)
	}

	if h1 == h2 {
		t.Fatal("different keys should produce different handles")
	}
	if cache.Len() != 2 {
		t.Fatalf("expected 2 cached entries, got %d", cache.Len())
	}
}

func TestPubKeyCacheClose(t *testing.T) {
	cache := NewPubKeyCache()

	var pubkey [types.PubkeySize]byte
	pubkey[0] = 0xAA

	_, err := cache.Get(pubkey)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	cache.Close()

	if cache.Len() != 0 {
		t.Fatalf("expected 0 entries after Close, got %d", cache.Len())
	}
}
