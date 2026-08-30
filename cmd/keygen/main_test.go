package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOptionsDefaults(t *testing.T) {
	var stderr bytes.Buffer
	opts, err := parseOptions(nil, &stderr)
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if opts.Validators != 5 || opts.Nodes != 3 || opts.OutputDir != "testnet" || opts.BasePort != 9000 {
		t.Fatalf("unexpected defaults: %+v", opts)
	}
}

func TestParseOptionsRejectsInvalidValues(t *testing.T) {
	tests := [][]string{
		{"--validators", "0"},
		{"--nodes", "0"},
		{"--output", ""},
		{"--base-port", "0"},
		{"--base-port", "65535", "--nodes", "2"},
	}
	for _, args := range tests {
		var stderr bytes.Buffer
		_, err := parseOptions(args, &stderr)
		if !errors.Is(err, errInvalidOptions) {
			t.Fatalf("args=%v error=%v, want errInvalidOptions", args, err)
		}
	}
}

func TestManifestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	want := &manifest{
		Validators: []validatorInfo{{
			Index:                2,
			AttestationPubkeyHex: "aa",
			ProposalPubkeyHex:    "bb",
			AttestationSkFile:    "att.ssz",
			ProposalSkFile:       "prop.ssz",
		}},
		Nodes: []nodeInfo{{KeyFile: "node0.key", PeerID: "peer"}},
	}

	if err := saveManifest(path, want); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	got, err := loadManifest(path)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if got.Validators[0] != want.Validators[0] || got.Nodes[0] != want.Nodes[0] {
		t.Fatalf("manifest=%+v, want %+v", got, want)
	}
}

func TestManifestUsableRejectsMissingFileNames(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "hash-sig-keys")
	if err := os.Mkdir(keysDir, 0o755); err != nil {
		t.Fatalf("mkdir keys: %v", err)
	}

	opts := options{Validators: 1, Nodes: 1, OutputDir: dir, BasePort: 9000}
	m := &manifest{
		Validators: []validatorInfo{{}},
		Nodes:      []nodeInfo{{}},
	}
	if manifestUsable(m, opts, keysDir) {
		t.Fatal("expected manifest with empty file names to be unusable")
	}
}

func TestManifestUsableAcceptsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "hash-sig-keys")
	if err := os.Mkdir(keysDir, 0o755); err != nil {
		t.Fatalf("mkdir keys: %v", err)
	}
	writeTestFile(t, filepath.Join(keysDir, "att.ssz"))
	writeTestFile(t, filepath.Join(keysDir, "prop.ssz"))
	writeTestFile(t, filepath.Join(dir, "node0.key"))

	opts := options{Validators: 1, Nodes: 1, OutputDir: dir, BasePort: 9000}
	m := &manifest{
		Validators: []validatorInfo{{
			Index:                0,
			AttestationPubkeyHex: strings.Repeat("a", 64),
			ProposalPubkeyHex:    strings.Repeat("b", 64),
			AttestationSkFile:    "att.ssz",
			ProposalSkFile:       "prop.ssz",
		}},
		Nodes: []nodeInfo{{KeyFile: "node0.key", PeerID: "peer0"}},
	}
	if !manifestUsable(m, opts, keysDir) {
		t.Fatal("expected manifest with existing files to be usable")
	}
}

func TestManifestUsableRejectsMalformedMetadata(t *testing.T) {
	dir := t.TempDir()
	keysDir := filepath.Join(dir, "hash-sig-keys")
	if err := os.Mkdir(keysDir, 0o755); err != nil {
		t.Fatalf("mkdir keys: %v", err)
	}
	writeTestFile(t, filepath.Join(keysDir, "att.ssz"))
	writeTestFile(t, filepath.Join(keysDir, "prop.ssz"))
	writeTestFile(t, filepath.Join(dir, "node0.key"))

	validValidator := validatorInfo{
		Index:                0,
		AttestationPubkeyHex: strings.Repeat("a", 64),
		ProposalPubkeyHex:    strings.Repeat("b", 64),
		AttestationSkFile:    "att.ssz",
		ProposalSkFile:       "prop.ssz",
	}
	validNode := nodeInfo{KeyFile: "node0.key", PeerID: "peer0"}
	opts := options{Validators: 1, Nodes: 1, OutputDir: dir, BasePort: 9000}

	tests := []struct {
		name     string
		manifest manifest
	}{
		{
			name:     "bad validator index",
			manifest: manifest{Validators: []validatorInfo{withValidatorIndex(validValidator, 1)}, Nodes: []nodeInfo{validNode}},
		},
		{
			name:     "bad pubkey hex",
			manifest: manifest{Validators: []validatorInfo{withAttestationPubkey(validValidator, "not-hex")}, Nodes: []nodeInfo{validNode}},
		},
		{
			name:     "missing peer id",
			manifest: manifest{Validators: []validatorInfo{validValidator}, Nodes: []nodeInfo{{KeyFile: "node0.key"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if manifestUsable(&tt.manifest, opts, keysDir) {
				t.Fatal("expected malformed manifest to be unusable")
			}
		})
	}
}

func TestRenderOutputs(t *testing.T) {
	validators := []validatorInfo{{
		Index:                1,
		AttestationPubkeyHex: "att",
		ProposalPubkeyHex:    "prop",
		AttestationSkFile:    "att.ssz",
		ProposalSkFile:       "prop.ssz",
	}}
	config := renderConfigYAML(123, validators)
	if !strings.Contains(config, "GENESIS_TIME: 123") || !strings.Contains(config, "attestation_pubkey: \"att\"") {
		t.Fatalf("config yaml missing fields:\n%s", config)
	}

	annotated := renderAnnotatedValidatorsYAML(validators, 2)
	if !strings.Contains(annotated, "node0:") || !strings.Contains(annotated, "node1:") ||
		!strings.Contains(annotated, "proposal_sk_file: prop.ssz") {
		t.Fatalf("annotated validators yaml missing fields:\n%s", annotated)
	}

	nodes := renderNodesYAML([]nodeInfo{{PeerID: "peer0"}, {PeerID: "peer1"}}, 9000)
	if !strings.Contains(nodes, "/udp/9000/") || !strings.Contains(nodes, "/udp/9001/") {
		t.Fatalf("nodes yaml missing ports:\n%s", nodes)
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func withValidatorIndex(v validatorInfo, index int) validatorInfo {
	v.Index = index
	return v
}

func withAttestationPubkey(v validatorInfo, pubkey string) validatorInfo {
	v.AttestationPubkeyHex = pubkey
	return v
}
