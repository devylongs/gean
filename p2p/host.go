package p2p

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// HostConfig holds configuration for creating a libp2p host.
type HostConfig struct {
	PrivateKey  crypto.PrivKey
	ListenAddrs []string
}

// NewHost creates a new libp2p host with the given configuration.
func NewHost(ctx context.Context, cfg HostConfig) (host.Host, error) {
	var privKey crypto.PrivKey
	var err error

	if cfg.PrivateKey != nil {
		privKey = cfg.PrivateKey
	} else {
		privKey, _, err = crypto.GenerateKeyPairWithReader(crypto.Secp256k1, 256, rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}
	}

	listenAddrs := cfg.ListenAddrs
	if len(listenAddrs) == 0 {
		// Devnet 0 uses QUIC transport
		listenAddrs = []string{
			"/ip4/0.0.0.0/udp/9000/quic-v1",
		}
	}

	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings(listenAddrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("create host: %w", err)
	}

	return h, nil
}

// ParseBootnodes parses a list of multiaddr strings into peer.AddrInfo.
// Skips ENR records (enr:-...) which require separate decoding.
func ParseBootnodes(addrs []string) ([]peer.AddrInfo, error) {
	var peers []peer.AddrInfo
	for _, addr := range addrs {
		// Skip ENR records for now - they need special decoding
		if len(addr) > 4 && addr[:4] == "enr:" {
			continue
		}
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			continue // Skip unparseable addresses
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			continue
		}
		peers = append(peers, *pi)
	}
	return peers, nil
}
