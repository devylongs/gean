package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

var errInvalidOptions = errors.New("invalid keygen options")

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, stderr io.Writer) error {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		return err
	}

	keysDir := filepath.Join(opts.OutputDir, "hash-sig-keys")
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := os.MkdirAll(keysDir, 0o755); err != nil {
		return fmt.Errorf("create key dir: %w", err)
	}

	manifestPath := filepath.Join(opts.OutputDir, "manifest.json")
	m, reused, err := loadOrGenerate(opts, keysDir, manifestPath)
	if err != nil {
		return err
	}
	if reused {
		log.Printf("keys already exist (%d validators, %d nodes) - skipping generation",
			len(m.Validators), len(m.Nodes))
	}

	genesisTime := opts.GenesisTime
	if genesisTime == 0 {
		genesisTime = uint64(time.Now().Unix()) + uint64(opts.GenesisDelay)
	}
	if err := writeConfigYAML(opts.OutputDir, genesisTime, m.Validators); err != nil {
		return err
	}
	if err := writeAnnotatedValidatorsYAML(opts.OutputDir, m.Validators, opts.Nodes); err != nil {
		return err
	}
	if err := writeNodesYAML(opts.OutputDir, m.Nodes, opts.BasePort); err != nil {
		return err
	}

	logSummary(opts, genesisTime, m)
	return nil
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	opts := options{}
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.IntVar(&opts.Validators, "validators", 5, "Number of validators to generate")
	fs.IntVar(&opts.Nodes, "nodes", 3, "Number of nodes")
	fs.StringVar(&opts.OutputDir, "output", "testnet", "Output directory")
	fs.IntVar(&opts.BasePort, "base-port", 9000, "Base P2P port (incremented per node)")
	fs.Uint64Var(&opts.GenesisTime, "genesis-time", 0, "Absolute genesis Unix time; 0 uses now + --genesis-delay")
	fs.IntVar(&opts.GenesisDelay, "genesis-delay", 30, "Seconds from now until genesis when --genesis-time is unset")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.Validators < 1 || opts.Nodes < 1 {
		return opts, fmt.Errorf("%w: need at least 1 validator and 1 node", errInvalidOptions)
	}
	if opts.OutputDir == "" {
		return opts, fmt.Errorf("%w: output directory is required", errInvalidOptions)
	}
	if opts.BasePort < 1 || opts.BasePort > 65535 {
		return opts, fmt.Errorf("%w: base port range exceeds 1..65535", errInvalidOptions)
	}
	if opts.Nodes > 65535-opts.BasePort+1 {
		return opts, fmt.Errorf("%w: base port range exceeds 1..65535", errInvalidOptions)
	}
	if opts.GenesisDelay < 0 {
		return opts, fmt.Errorf("%w: genesis delay must be >= 0", errInvalidOptions)
	}
	return opts, nil
}

func logSummary(opts options, genesisTime uint64, m manifest) {
	log.Println("---")
	log.Printf("output: %s", opts.OutputDir)
	log.Printf("genesis time: %d (%s)", genesisTime,
		time.Unix(int64(genesisTime), 0).Format(time.RFC3339))
	log.Printf("validators: %d, nodes: %d", len(m.Validators), len(m.Nodes))
	log.Println("")
	log.Println("run immediately:")
	log.Printf("  bin/gean --custom-network-config-dir %s --node-key %s/node0.key --node-id node0 --is-aggregator --data-dir data/node0",
		opts.OutputDir, opts.OutputDir)
}
