package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/devylongs/gean/consensus"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

// Service manages p2p networking for the consensus client.
type Service struct {
	host     host.Host
	pubsub   *pubsub.PubSub
	handlers *MessageHandlers
	logger   *slog.Logger

	blockTopic *pubsub.Topic
	blockSub   *pubsub.Subscription
	attTopic   *pubsub.Topic
	attSub     *pubsub.Subscription

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ServiceConfig holds configuration for the p2p service.
type ServiceConfig struct {
	Host      host.Host
	Handlers  *MessageHandlers
	Bootnodes []peer.AddrInfo
	Logger    *slog.Logger
}

// NewService creates a new p2p service.
func NewService(ctx context.Context, cfg ServiceConfig) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Create gossipsub
	ps, err := NewGossipSub(ctx, cfg.Host)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create gossipsub: %w", err)
	}

	// Join topics
	blockTopic, err := ps.Join(BlockTopic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("join block topic: %w", err)
	}

	attTopic, err := ps.Join(AttestationTopic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("join attestation topic: %w", err)
	}

	// Subscribe to topics
	blockSub, err := blockTopic.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe block topic: %w", err)
	}

	attSub, err := attTopic.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe attestation topic: %w", err)
	}

	svc := &Service{
		host:       cfg.Host,
		pubsub:     ps,
		handlers:   cfg.Handlers,
		logger:     logger,
		blockTopic: blockTopic,
		blockSub:   blockSub,
		attTopic:   attTopic,
		attSub:     attSub,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Connect to bootnodes
	for _, pi := range cfg.Bootnodes {
		if err := cfg.Host.Connect(ctx, pi); err != nil {
			logger.Warn("failed to connect to bootnode",
				"peer", pi.ID,
				"error", err,
			)
		} else {
			logger.Info("connected to bootnode", "peer", pi.ID)
		}
	}

	return svc, nil
}

// Start begins processing incoming messages.
func (s *Service) Start() {
	s.wg.Add(2)
	go s.processBlocks()
	go s.processAttestations()
	s.logger.Info("p2p service started",
		"peer_id", s.host.ID(),
		"addrs", s.host.Addrs(),
	)
}

// Stop shuts down the p2p service.
func (s *Service) Stop() {
	s.cancel()
	s.blockSub.Cancel()
	s.attSub.Cancel()
	s.wg.Wait()
	s.host.Close()
	s.logger.Info("p2p service stopped")
}

// PublishBlock publishes a signed block to the network.
func (s *Service) PublishBlock(ctx context.Context, block *consensus.SignedBlock) error {
	data, err := block.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal block: %w", err)
	}
	compressed := CompressMessage(data)
	return s.blockTopic.Publish(ctx, compressed)
}

// PublishAttestation publishes a signed vote to the network.
func (s *Service) PublishAttestation(ctx context.Context, vote *consensus.SignedVote) error {
	data, err := vote.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal attestation: %w", err)
	}
	compressed := CompressMessage(data)
	return s.attTopic.Publish(ctx, compressed)
}

// PeerCount returns the number of connected peers.
func (s *Service) PeerCount() int {
	return len(s.host.Network().Peers())
}

// processBlocks handles incoming block messages.
func (s *Service) processBlocks() {
	defer s.wg.Done()

	for {
		msg, err := s.blockSub.Next(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				return // context cancelled
			}
			s.logger.Error("block subscription error", "error", err)
			continue
		}

		// Skip self-published messages
		if msg.ReceivedFrom == s.host.ID() {
			continue
		}

		if s.handlers != nil {
			if err := s.handlers.HandleBlockMessage(s.ctx, msg.Data); err != nil {
				s.logger.Error("handle block error", "error", err)
			}
		}
	}
}

// processAttestations handles incoming attestation messages.
func (s *Service) processAttestations() {
	defer s.wg.Done()

	for {
		msg, err := s.attSub.Next(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				return // context cancelled
			}
			s.logger.Error("attestation subscription error", "error", err)
			continue
		}

		// Skip self-published messages
		if msg.ReceivedFrom == s.host.ID() {
			continue
		}

		if s.handlers != nil {
			if err := s.handlers.HandleAttestationMessage(s.ctx, msg.Data); err != nil {
				s.logger.Error("handle attestation error", "error", err)
			}
		}
	}
}
