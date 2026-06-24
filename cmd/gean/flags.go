package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/geanlabs/gean/internal/shadowcost"
)

var errInvalidConfig = errors.New("invalid gean configuration")

type config struct {
	ConfigDir          string
	GossipPort         int
	HTTPAddr           string
	APIPort            int
	MetricsPort        int
	NodeKey            string
	NodeID             string
	CheckpointURL      string
	IsAggregator       bool
	CommitteeCount     uint64
	AggregateSubnetIDs []uint64
	DataDir            string
	ShadowAggregateMs  int
	ShadowVerifyMs     int
}

type configPaths struct {
	config     string
	bootnodes  string
	validators string
	keysDir    string
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("gean", flag.ContinueOnError)
	fs.SetOutput(stderr)

	aggregateSubnetIDs := ""
	fs.StringVar(&cfg.ConfigDir, "custom-network-config-dir", "", "Config directory (required)")
	fs.IntVar(&cfg.GossipPort, "gossipsub-port", 9000, "P2P listen port (QUIC/UDP)")
	fs.StringVar(&cfg.HTTPAddr, "http-address", "127.0.0.1", "Bind address for API + metrics")
	fs.IntVar(&cfg.APIPort, "api-port", 5052, "API server port")
	fs.IntVar(&cfg.MetricsPort, "metrics-port", 5054, "Metrics server port")
	fs.StringVar(&cfg.NodeKey, "node-key", "", "Path to hex-encoded secp256k1 private key (required)")
	fs.StringVar(&cfg.NodeID, "node-id", "", "Node identifier, e.g. gean_0 (required)")
	fs.StringVar(&cfg.CheckpointURL, "checkpoint-sync-url", "", "URL for checkpoint sync (optional)")
	fs.BoolVar(&cfg.IsAggregator, "is-aggregator", false, "Enable attestation aggregation")
	fs.Uint64Var(&cfg.CommitteeCount, "attestation-committee-count", 1, "Number of attestation subnets")
	fs.StringVar(&aggregateSubnetIDs, "aggregate-subnet-ids", "", "Comma-separated subnet IDs (requires --is-aggregator)")
	fs.StringVar(&cfg.DataDir, "data-dir", "./data", "Pebble database directory")
	fs.IntVar(&cfg.ShadowAggregateMs, "shadow-aggregate-cost-ms", 0,
		"Simulated aggregation prover delay in ms (network-simulator runs only; 0 disables)")
	fs.IntVar(&cfg.ShadowVerifyMs, "shadow-verify-cost-ms", 0,
		"Simulated attestation-verify delay in ms (network-simulator runs only; 0 disables)")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}

	if cfg.ConfigDir == "" || cfg.NodeKey == "" || cfg.NodeID == "" {
		fmt.Fprintln(stderr, "required flags: --custom-network-config-dir, --node-key, --node-id")
		fs.Usage()
		return cfg, errInvalidConfig
	}
	if err := validatePort("gossipsub-port", cfg.GossipPort, stderr); err != nil {
		return cfg, err
	}
	if err := validatePort("api-port", cfg.APIPort, stderr); err != nil {
		return cfg, err
	}
	if err := validatePort("metrics-port", cfg.MetricsPort, stderr); err != nil {
		return cfg, err
	}
	if cfg.CommitteeCount < 1 {
		fmt.Fprintln(stderr, "--attestation-committee-count must be >= 1")
		return cfg, errInvalidConfig
	}
	if cfg.ShadowAggregateMs < 0 || cfg.ShadowVerifyMs < 0 {
		fmt.Fprintln(stderr, "--shadow-*-cost-ms must be >= 0")
		return cfg, errInvalidConfig
	}
	if !cfg.IsAggregator && aggregateSubnetIDs != "" {
		fmt.Fprintln(stderr, "--aggregate-subnet-ids requires --is-aggregator")
		return cfg, errInvalidConfig
	}

	subnetIDs, err := parseAggregateSubnetIDs(aggregateSubnetIDs, stderr)
	if err != nil {
		return cfg, err
	}
	if err := validateAggregateSubnetIDs(subnetIDs, cfg.CommitteeCount, stderr); err != nil {
		return cfg, err
	}
	cfg.AggregateSubnetIDs = subnetIDs
	return cfg, nil
}

func validatePort(name string, port int, stderr io.Writer) error {
	if port < 0 || port > 65535 {
		fmt.Fprintf(stderr, "--%s must be between 0 and 65535\n", name)
		return errInvalidConfig
	}
	return nil
}

func parseAggregateSubnetIDs(raw string, stderr io.Writer) ([]uint64, error) {
	if raw == "" {
		return nil, nil
	}
	var ids []uint64
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			fmt.Fprintf(stderr, "invalid aggregate-subnet-id %q: %v\n", part, err)
			return nil, errInvalidConfig
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func validateAggregateSubnetIDs(ids []uint64, committeeCount uint64, stderr io.Writer) error {
	for _, id := range ids {
		if id >= committeeCount {
			fmt.Fprintf(stderr, "--aggregate-subnet-ids contains %d, want < --attestation-committee-count (%d)\n",
				id, committeeCount)
			return errInvalidConfig
		}
	}
	return nil
}

func (c config) paths() configPaths {
	return configPaths{
		config:     filepath.Join(c.ConfigDir, "config.yaml"),
		bootnodes:  filepath.Join(c.ConfigDir, "nodes.yaml"),
		validators: filepath.Join(c.ConfigDir, "annotated_validators.yaml"),
		keysDir:    filepath.Join(c.ConfigDir, "hash-sig-keys"),
	}
}

func (c config) shadowCosts() shadowcost.Costs {
	return shadowcost.Costs{
		Aggregate: time.Duration(c.ShadowAggregateMs) * time.Millisecond,
		Verify:    time.Duration(c.ShadowVerifyMs) * time.Millisecond,
	}
}

func (c config) apiAddress() string {
	return net.JoinHostPort(c.HTTPAddr, strconv.Itoa(c.APIPort))
}

func (c config) metricsAddress() string {
	return net.JoinHostPort(c.HTTPAddr, strconv.Itoa(c.MetricsPort))
}
