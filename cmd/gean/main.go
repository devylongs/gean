package main

import (
	"context"
	"errors"
	"flag"
	"os"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/node"
	"github.com/geanlabs/gean/internal/role"
)

func main() {
	cfg, err := parseConfig(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(1)
	}
	if err := run(cfg); err != nil {
		logger.Error(logger.Node, "fatal: %v", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	logger.Info(logger.Node, "gean consensus client starting")

	inputs, err := loadStartupInputs(cfg)
	if err != nil {
		return err
	}
	defer inputs.keyManager.Close()

	backend, s, err := openStore(cfg.DataDir)
	if err != nil {
		logger.Error(logger.Node, "open pebble: %v", err)
		return err
	}
	defer backend.Close()

	if err := bootstrapStore(s, inputs.genesisConfig, cfg.CheckpointURL); err != nil {
		return err
	}

	if err := recoverStoreTime(s, inputs.genesisConfig.GenesisTime); err != nil {
		return err
	}

	headSlot, headRoot, parentRoot, err := forkChoiceAnchor(s)
	if err != nil {
		return err
	}
	fc := forkchoice.New(headSlot, headRoot, parentRoot)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p2pHost, err := setupP2P(ctx, cfg, inputs.keyManager)
	if err != nil {
		logger.Error(logger.Network, "create p2p host: %v", err)
		return err
	}
	defer p2pHost.Close()

	preinitializeXMSS(cfg.IsAggregator)

	aggCtl := role.NewWithHook(cfg.IsAggregator, metrics.SetIsAggregator)
	n := node.New(s, fc, p2pHost, inputs.keyManager, aggCtl, cfg.CommitteeCount, cfg.shadowCosts())

	registerReqRespHandlers(p2pHost, s)
	startNodeNetworking(ctx, n, s, p2pHost, inputs.bootnodes)

	apiAddr, metricsAddr := startHTTPServers(cfg, s, fc, aggCtl)
	logger.Info(logger.Node, "gean started: api=%s metrics=%s aggregator=%v", apiAddr, metricsAddr, cfg.IsAggregator)

	waitForShutdown(cancel)
	return nil
}
