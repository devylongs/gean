package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func validFlagArgs() []string {
	return []string{
		"--custom-network-config-dir", "/config",
		"--node-key", "/config/node0.key",
		"--node-id", "node0",
	}
}

func TestParseConfig_ValidDefaults(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseConfig(validFlagArgs(), &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}

	if cfg.ConfigDir != "/config" || cfg.NodeKey != "/config/node0.key" || cfg.NodeID != "node0" {
		t.Fatalf("required fields not parsed: %+v", cfg)
	}
	if cfg.GossipPort != 9000 || cfg.HTTPAddr != "127.0.0.1" || cfg.APIPort != 5052 || cfg.MetricsPort != 5054 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.IsAggregator || cfg.CommitteeCount != 1 || cfg.DataDir != "./data" || len(cfg.AggregateSubnetIDs) != 0 {
		t.Fatalf("unexpected role/storage defaults: %+v", cfg)
	}
	if cfg.ShadowAggregateSignaturesRate != 0 || cfg.ShadowVerifySignatureRate != 0 || cfg.ShadowVerifyAggregatedSignaturesRate != 0 {
		t.Fatalf("shadow rates must default to disabled: %+v", cfg)
	}
}

func TestParseConfig_ShadowRates(t *testing.T) {
	args := append(validFlagArgs(),
		"--shadow-xmss-aggregate-signatures-rate", "5",
		"--shadow-xmss-verify-signature-rate", "2",
		"--shadow-xmss-verify-aggregated-signatures-rate", "100",
	)
	var stderr bytes.Buffer
	cfg, err := parseConfig(args, &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}
	if cfg.ShadowAggregateSignaturesRate != 5 ||
		cfg.ShadowVerifySignatureRate != 2 ||
		cfg.ShadowVerifyAggregatedSignaturesRate != 100 {
		t.Fatalf("unexpected shadow rates: %+v", cfg)
	}
}

func TestParseConfig_ShadowRatesFromEnv(t *testing.T) {
	t.Setenv("GEAN_SHADOW_XMSS_AGGREGATE_SIGNATURES_RATE", "7.5")
	t.Setenv("GEAN_SHADOW_XMSS_VERIFY_SIGNATURE_RATE", "3")
	t.Setenv("GEAN_SHADOW_XMSS_VERIFY_AGGREGATED_SIGNATURES_RATE", "9")

	var stderr bytes.Buffer
	cfg, err := parseConfig(validFlagArgs(), &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}
	if cfg.ShadowAggregateSignaturesRate != 7.5 ||
		cfg.ShadowVerifySignatureRate != 3 ||
		cfg.ShadowVerifyAggregatedSignaturesRate != 9 {
		t.Fatalf("env rates not applied: %+v", cfg)
	}
}

func TestParseConfig_ShadowRateFlagBeatsEnv(t *testing.T) {
	t.Setenv("GEAN_SHADOW_XMSS_AGGREGATE_SIGNATURES_RATE", "7.5")

	args := append(validFlagArgs(), "--shadow-xmss-aggregate-signatures-rate", "2")
	var stderr bytes.Buffer
	cfg, err := parseConfig(args, &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}
	if cfg.ShadowAggregateSignaturesRate != 2 {
		t.Fatalf("flag should win over env, got %v", cfg.ShadowAggregateSignaturesRate)
	}
}

func TestParseConfig_InvalidShadowEnv(t *testing.T) {
	t.Setenv("GEAN_SHADOW_XMSS_VERIFY_SIGNATURE_RATE", "not-a-number")

	var stderr bytes.Buffer
	_, err := parseConfig(validFlagArgs(), &stderr)
	if err == nil {
		t.Fatal("expected unparseable env rate to fail")
	}
	if !strings.Contains(stderr.String(), "GEAN_SHADOW_XMSS_VERIFY_SIGNATURE_RATE") {
		t.Fatalf("env error message not found:\n%s", stderr.String())
	}
}

func TestParseConfig_NegativeShadowRate(t *testing.T) {
	for _, flag := range []string{
		"--shadow-xmss-aggregate-signatures-rate",
		"--shadow-xmss-verify-signature-rate",
		"--shadow-xmss-verify-aggregated-signatures-rate",
	} {
		t.Run(flag, func(t *testing.T) {
			args := append(validFlagArgs(), flag, "-1")
			var stderr bytes.Buffer
			_, err := parseConfig(args, &stderr)
			if err == nil {
				t.Fatal("expected negative shadow rate to fail")
			}
			if !strings.Contains(stderr.String(), flag+" must not be negative") {
				t.Fatalf("negative rate message not found:\n%s", stderr.String())
			}
		})
	}
}

func TestParseConfig_MissingRequiredFlags(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseConfig(nil, &stderr)
	if err == nil {
		t.Fatal("expected missing required flags to fail")
	}
	if !strings.Contains(stderr.String(), "required flags: --custom-network-config-dir, --node-key, --node-id") {
		t.Fatalf("missing required flags message not found:\n%s", stderr.String())
	}
}

func TestParseConfig_InvalidCommitteeCount(t *testing.T) {
	args := append(validFlagArgs(), "--attestation-committee-count", "0")
	var stderr bytes.Buffer
	_, err := parseConfig(args, &stderr)
	if err == nil {
		t.Fatal("expected committee count validation to fail")
	}
	if !strings.Contains(stderr.String(), "--attestation-committee-count must be >= 1") {
		t.Fatalf("committee count message not found:\n%s", stderr.String())
	}
}

func TestParseConfig_CommitteeCountSetTracking(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseConfig(validFlagArgs(), &stderr)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.committeeCountSet {
		t.Fatal("committeeCountSet should be false when flag omitted")
	}

	args := append(validFlagArgs(), "--attestation-committee-count", "4")
	cfg, err = parseConfig(args, &stderr)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !cfg.committeeCountSet || cfg.CommitteeCount != 4 {
		t.Fatalf("explicit flag not tracked: set=%v count=%d", cfg.committeeCountSet, cfg.CommitteeCount)
	}
}

func TestResolveCommitteeCount(t *testing.T) {
	count := func(v uint64) *uint64 { return &v }
	tests := []struct {
		name      string
		flagCount uint64
		flagSet   bool
		config    *uint64
		want      uint64
	}{
		{"flag overrides config", 4, true, count(8), 4},
		{"config when flag unset", 1, false, count(8), 8},
		{"default when neither set", 1, false, nil, 1},
		{"flag wins even when equal to default", 1, true, count(8), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveCommitteeCount(tt.flagCount, tt.flagSet, tt.config); got != tt.want {
				t.Fatalf("resolveCommitteeCount = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseConfig_AggregateSubnetsRequireAggregator(t *testing.T) {
	args := append(validFlagArgs(), "--aggregate-subnet-ids", "1,2")
	var stderr bytes.Buffer
	_, err := parseConfig(args, &stderr)
	if err == nil {
		t.Fatal("expected aggregate subnets without aggregator to fail")
	}
	if !strings.Contains(stderr.String(), "--aggregate-subnet-ids requires --is-aggregator") {
		t.Fatalf("aggregate subnet dependency message not found:\n%s", stderr.String())
	}
}

func TestParseConfig_InvalidAggregateSubnetID(t *testing.T) {
	args := append(validFlagArgs(), "--is-aggregator", "--aggregate-subnet-ids", "1,nope")
	var stderr bytes.Buffer
	_, err := parseConfig(args, &stderr)
	if err == nil {
		t.Fatal("expected invalid aggregate subnet ID to fail")
	}
	if !strings.Contains(stderr.String(), `invalid aggregate-subnet-id "nope"`) {
		t.Fatalf("invalid subnet message not found:\n%s", stderr.String())
	}
}

func TestParseConfig_AggregateSubnetIDs(t *testing.T) {
	args := append(validFlagArgs(), "--is-aggregator", "--attestation-committee-count", "4", "--aggregate-subnet-ids", "1, 2,,3")
	var stderr bytes.Buffer
	cfg, err := parseConfig(args, &stderr)
	if err != nil {
		t.Fatalf("parseConfig returned error: %v\nstderr:\n%s", err, stderr.String())
	}
	if !cfg.IsAggregator {
		t.Fatal("expected aggregator flag to be true")
	}
	if !reflect.DeepEqual(cfg.AggregateSubnetIDs, []uint64{1, 2, 3}) {
		t.Fatalf("unexpected aggregate subnet IDs: %v", cfg.AggregateSubnetIDs)
	}
}

func TestParseConfig_InvalidPorts(t *testing.T) {
	tests := []struct {
		name string
		flag string
		port string
	}{
		{name: "gossip negative", flag: "--gossipsub-port", port: "-1"},
		{name: "api too high", flag: "--api-port", port: "65536"},
		{name: "metrics too high", flag: "--metrics-port", port: "65536"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append(validFlagArgs(), tt.flag, tt.port)
			var stderr bytes.Buffer
			_, err := parseConfig(args, &stderr)
			if err == nil {
				t.Fatal("expected invalid port to fail")
			}
			if !strings.Contains(stderr.String(), tt.flag+" must be between 0 and 65535") {
				t.Fatalf("port validation message not found:\n%s", stderr.String())
			}
		})
	}
}

func TestParseConfig_AggregateSubnetIDsMustBeInRange(t *testing.T) {
	args := append(validFlagArgs(), "--is-aggregator", "--attestation-committee-count", "2", "--aggregate-subnet-ids", "0,2")
	var stderr bytes.Buffer
	_, err := parseConfig(args, &stderr)
	if err == nil {
		t.Fatal("expected out-of-range aggregate subnet to fail")
	}
	if !strings.Contains(stderr.String(), "--aggregate-subnet-ids contains 2") {
		t.Fatalf("subnet range message not found:\n%s", stderr.String())
	}
}

func TestConfigAddressesUseJoinHostPort(t *testing.T) {
	cfg := config{HTTPAddr: "::1", APIPort: 5052, MetricsPort: 5054}
	if got := cfg.apiAddress(); got != "[::1]:5052" {
		t.Fatalf("apiAddress=%q, want [::1]:5052", got)
	}
	if got := cfg.metricsAddress(); got != "[::1]:5054" {
		t.Fatalf("metricsAddress=%q, want [::1]:5054", got)
	}
}
