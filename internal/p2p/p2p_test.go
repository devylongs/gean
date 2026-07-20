package p2p

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
	"github.com/golang/snappy"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// stubStream is a minimal network.Stream for exercising writeResponse: it wires
// Write and SetWriteDeadline (the only methods writeResponse touches) and leaves the
// rest to the embedded nil interface, which panics if unexpectedly called.
type stubStream struct {
	network.Stream
	w        io.Writer
	writeErr error
}

func (s stubStream) Write(p []byte) (int, error) {
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	return s.w.Write(p)
}

func (stubStream) SetWriteDeadline(time.Time) error { return nil }

type captureHandler struct {
	block *types.SignedBlock
}

func (h *captureHandler) OnBlock(block *types.SignedBlock) { h.block = block }
func (h *captureHandler) OnGossipAttestation(att *types.SignedAttestation) {
}
func (h *captureHandler) OnGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) {
}

func TestLogPublishIncludesDevnetDiagnostics(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetQuiet(false)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		SetClientGitCommit(unknownBuildValue)
	})

	SetClientGitCommit("abc123")
	sszData := []byte("signed block ssz")
	compressed := SnappyRawEncode(sszData)
	root := [32]byte{0xaa}
	logPublish(BlockTopic(), sszData, compressed, publishLogInfo{
		slot:         7,
		proposer:     "3",
		blockRoot:    root,
		hasBlockRoot: true,
	})

	out := buf.String()
	wantParts := []string{
		"publish gossip",
		"topic=" + BlockTopic(),
		"slot=7",
		"proposer=3",
		"block_root=0xaa",
		"sha256_ssz=0x",
		"sha256_compressed=0x",
		"compressed_len=",
		"snappy_self_decode_ok=true",
		"message_id=0x",
		"client_git_sha=abc123",
		"snappy=github.com/golang/snappy@",
	}
	for _, part := range wantParts {
		if !strings.Contains(out, part) {
			t.Fatalf("publish log missing %q:\n%s", part, out)
		}
	}
}

func TestPublishBlockLogsCanonicalBlockRoot(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetQuiet(false)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
	})

	signed := &types.SignedBlock{
		Block: &types.Block{
			Slot:          7,
			ProposerIndex: 3,
			Body:          &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{Proof: []byte{0x42}},
	}
	blockRoot, err := signed.Block.HashTreeRoot()
	if err != nil {
		t.Fatalf("block root: %v", err)
	}
	signedRoot, err := signed.HashTreeRoot()
	if err != nil {
		t.Fatalf("signed block root: %v", err)
	}
	if blockRoot == signedRoot {
		t.Fatal("test setup failed: signed block root unexpectedly equals canonical block root")
	}

	h := &Host{}
	err = h.PublishBlock(context.Background(), signed)
	if err == nil || !strings.Contains(err.Error(), "not subscribed to topic") {
		t.Fatalf("PublishBlock error=%v, want missing topic after log", err)
	}

	out := buf.String()
	want := "block_root=0x" + hex.EncodeToString(blockRoot[:])
	if !strings.Contains(out, want) {
		t.Fatalf("publish log missing canonical root %s:\n%s", want, out)
	}
	wrong := "block_root=0x" + hex.EncodeToString(signedRoot[:])
	if strings.Contains(out, wrong) {
		t.Fatalf("publish log used signed-block root %s:\n%s", wrong, out)
	}
}

func TestSnappyRawRoundtrip(t *testing.T) {
	data := []byte("hello lean consensus world")
	compressed := SnappyRawEncode(data)
	decompressed, err := SnappyRawDecode(compressed)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(data, decompressed) {
		t.Fatal("roundtrip mismatch")
	}
}

func TestVarintRoundtrip(t *testing.T) {
	tests := []uint32{0, 1, 127, 128, 255, 256, 16383, 16384, 1<<20 - 1}
	for _, v := range tests {
		encoded := EncodeVarint(v)
		decoded, rest, err := DecodeVarint(encoded)
		if err != nil {
			t.Fatalf("varint %d: %v", v, err)
		}
		if decoded != v {
			t.Fatalf("varint %d: got %d", v, decoded)
		}
		if len(rest) != 0 {
			t.Fatalf("varint %d: %d trailing bytes", v, len(rest))
		}
	}
}

func TestReqRespPayloadRoundtrip(t *testing.T) {
	data := []byte("state transition function data")
	encoded := EncodeReqRespPayload(data)
	decoded, err := DecodeReqRespPayload(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(data, decoded) {
		t.Fatal("payload roundtrip mismatch")
	}
}

func TestResponseEncoding(t *testing.T) {
	data := []byte("response payload")
	encoded := EncodeResponse(RespSuccess, data)
	if encoded[0] != RespSuccess {
		t.Fatalf("expected code 0x00, got 0x%02x", encoded[0])
	}

	reader := bytes.NewReader(encoded)
	code, decoded, err := DecodeResponse(reader)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if code != RespSuccess {
		t.Fatalf("code: expected 0, got %d", code)
	}
	if !bytes.Equal(data, decoded) {
		t.Fatal("response roundtrip mismatch")
	}
}

func TestWriteResponseReportsWriteFailure(t *testing.T) {
	if writeResponse(stubStream{writeErr: os.ErrClosed}, "test", RespSuccess, []byte("payload")) {
		t.Fatal("expected writeResponse to report failure")
	}
}

func TestWriteResponseWritesEncodedResponse(t *testing.T) {
	var buf bytes.Buffer
	if !writeResponse(stubStream{w: &buf}, "test", RespSuccess, []byte("payload")) {
		t.Fatal("expected writeResponse success")
	}
	code, payload, err := DecodeResponse(&buf)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if code != RespSuccess || !bytes.Equal(payload, []byte("payload")) {
		t.Fatalf("code=%d payload=%q, want success/payload", code, payload)
	}
}

func TestErrorMessageTruncatedOnEncode(t *testing.T) {
	oversized := bytes.Repeat([]byte("A"), MaxErrorMessageSize+100)
	encoded := EncodeResponse(RespInvalidRequest, oversized)
	code, decoded, err := DecodeResponse(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if code != RespInvalidRequest {
		t.Fatalf("code: expected %d, got %d", RespInvalidRequest, code)
	}
	if len(decoded) != MaxErrorMessageSize {
		t.Fatalf("decoded length: expected %d, got %d", MaxErrorMessageSize, len(decoded))
	}
}

func TestErrorMessageAtBoundaryRoundtrips(t *testing.T) {
	data := bytes.Repeat([]byte("B"), MaxErrorMessageSize)
	encoded := EncodeResponse(RespServerError, data)
	code, decoded, err := DecodeResponse(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if code != RespServerError {
		t.Fatalf("code: expected %d, got %d", RespServerError, code)
	}
	if !bytes.Equal(data, decoded) {
		t.Fatal("boundary-size error message roundtrip mismatch")
	}
}

func TestErrorMessageOversizedRejectedOnDecode(t *testing.T) {
	oversized := bytes.Repeat([]byte("C"), MaxErrorMessageSize+1)
	encoded := EncodeResponse(RespSuccess, oversized)
	encoded[0] = RespServerError
	_, _, err := DecodeResponse(bytes.NewReader(encoded))
	if err == nil {
		t.Fatal("expected error for oversize error-body decode, got nil")
	}
}

func TestReqRespPayloadRejectsBlockFormatSnappy(t *testing.T) {
	raw := []byte("hello world")
	blockFormat := snappy.Encode(nil, raw)
	chunk := append(EncodeVarint(uint32(len(raw))), blockFormat...)
	_, err := DecodeReqRespPayload(chunk)
	if err == nil {
		t.Fatal("expected error decoding block-format snappy as framed, got nil")
	}
}

func TestReqRespPayloadRejectsDeclaredLengthMismatch(t *testing.T) {
	encoded := EncodeReqRespPayload([]byte("x"))
	encoded[0] = 0
	_, err := DecodeReqRespPayload(encoded)
	if err == nil {
		t.Fatal("expected length mismatch error")
	}
}

func TestReqRespPayloadRejectsDecodedBodyPastDeclaredLength(t *testing.T) {
	var framed bytes.Buffer
	w := snappy.NewBufferedWriter(&framed)
	if _, err := w.Write(bytes.Repeat([]byte("x"), 1024)); err != nil {
		t.Fatalf("write snappy payload: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close snappy writer: %v", err)
	}

	encoded := append(EncodeVarint(1), framed.Bytes()...)
	_, err := DecodeReqRespPayload(encoded)
	if err == nil {
		t.Fatal("expected length mismatch for decoded body past declared length")
	}
	if !strings.Contains(err.Error(), "length mismatch: declared 1") {
		t.Fatalf("error=%v, want declared length mismatch", err)
	}
}

func TestSuccessUntouchedByErrorMessageCap(t *testing.T) {
	large := bytes.Repeat([]byte("D"), MaxErrorMessageSize+1024)
	encoded := EncodeResponse(RespSuccess, large)
	code, decoded, err := DecodeResponse(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if code != RespSuccess {
		t.Fatalf("code: expected %d, got %d", RespSuccess, code)
	}
	if !bytes.Equal(large, decoded) {
		t.Fatal("success payload should not be truncated")
	}
}

func TestDecodeResponseConcatenatedChunks(t *testing.T) {
	encoded := append(EncodeResponse(RespSuccess, []byte("one")), EncodeResponse(RespSuccess, []byte("two"))...)
	reader := bytes.NewReader(encoded)

	code, data, err := DecodeResponse(reader)
	if err != nil {
		t.Fatalf("decode first: %v", err)
	}
	if code != RespSuccess || string(data) != "one" {
		t.Fatalf("first response: code=%d data=%q", code, data)
	}

	code, data, err = DecodeResponse(reader)
	if err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if code != RespSuccess || string(data) != "two" {
		t.Fatalf("second response: code=%d data=%q", code, data)
	}
}

func TestDispatchMessageRequiresHandler(t *testing.T) {
	h := &Host{}
	if err := h.dispatchMessage(BlockTopic(), nil, nil); err == nil {
		t.Fatal("expected missing handler error")
	}
}

func TestDispatchMessageUsesHostHooks(t *testing.T) {
	block := &types.SignedBlock{
		Block: &types.Block{Body: &types.BlockBody{}},
		Proof: &types.MultiMessageAggregate{},
	}
	data, err := block.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal block: %v", err)
	}

	var observedSize int
	h := &Host{
		Hooks: Hooks{
			GossipBlockSize: func(bytes int) {
				observedSize = bytes
			},
		},
	}
	handler := &captureHandler{}

	if err := h.dispatchMessage(BlockTopic(), data, handler); err != nil {
		t.Fatalf("dispatch block: %v", err)
	}
	if observedSize != len(data) {
		t.Fatalf("observed size=%d, want %d", observedSize, len(data))
	}
	if handler.block == nil {
		t.Fatal("block handler was not called")
	}
}

func TestPublishRejectsNilMessages(t *testing.T) {
	h := &Host{}
	ctx := context.Background()

	if err := h.PublishBlock(ctx, nil); err == nil {
		t.Fatal("expected nil block error")
	}
	if err := h.PublishAttestation(ctx, nil, 1); err == nil {
		t.Fatal("expected nil attestation error")
	}
	if err := h.PublishAggregatedAttestation(ctx, nil); err == nil {
		t.Fatal("expected nil aggregation error")
	}
}

func TestTopicStrings(t *testing.T) {
	if BlockTopic() != "/leanconsensus/12345678/block/ssz_snappy" {
		t.Fatalf("block topic: %s", BlockTopic())
	}
	if AggregationTopic() != "/leanconsensus/12345678/aggregation/ssz_snappy" {
		t.Fatalf("aggregation topic: %s", AggregationTopic())
	}
	if AttestationSubnetTopic(0) != "/leanconsensus/12345678/attestation_0/ssz_snappy" {
		t.Fatalf("attestation subnet 0: %s", AttestationSubnetTopic(0))
	}
	if AttestationSubnetTopic(3) != "/leanconsensus/12345678/attestation_3/ssz_snappy" {
		t.Fatalf("attestation subnet 3: %s", AttestationSubnetTopic(3))
	}
}

func TestIsAttestationSubnetTopic(t *testing.T) {
	tests := []struct {
		topic string
		want  bool
	}{
		{AttestationSubnetTopic(0), true},
		{AttestationSubnetTopic(42), true},
		{"/leanconsensus/12345678/attestation_/ssz_snappy", false},
		{"/leanconsensus/12345678/attestation_x/ssz_snappy", false},
		{"/leanconsensus/12345678/not_attestation_0/ssz_snappy", false},
		{"/other/12345678/attestation_0/ssz_snappy", false},
		{"/leanconsensus/12345678/attestation_0/json", false},
	}

	for _, test := range tests {
		if got := isAttestationSubnetTopic(test.topic); got != test.want {
			t.Fatalf("isAttestationSubnetTopic(%q)=%t, want %t", test.topic, got, test.want)
		}
	}
}

func TestDispatchMessageRejectsNearMissAttestationTopic(t *testing.T) {
	h := &Host{}
	err := h.dispatchMessage("/leanconsensus/12345678/not_attestation_0/ssz_snappy", nil, &captureHandler{})
	if err == nil {
		t.Fatal("expected unknown topic error")
	}
}

func TestSubnetID(t *testing.T) {
	if SubnetID(0, 1) != 0 {
		t.Fatal("subnet 0%1 != 0")
	}
	if SubnetID(5, 3) != 2 {
		t.Fatal("subnet 5%3 != 2")
	}
	if SubnetID(7, 4) != 3 {
		t.Fatal("subnet 7%4 != 3")
	}
	if SubnetID(7, 0) != 0 {
		t.Fatal("subnet with zero committees should return 0")
	}
}

func TestComputeMessageIDDeterministic(t *testing.T) {
	topic := BlockTopic()
	data := SnappyRawEncode([]byte("block data"))

	id1 := ComputeMessageID(topic, data)
	id2 := ComputeMessageID(topic, data)

	if !bytes.Equal(id1, id2) {
		t.Fatal("message IDs should be deterministic")
	}
	if len(id1) != 20 {
		t.Fatalf("message ID should be 20 bytes, got %d", len(id1))
	}
}

func TestComputeMessageIDDifferentTopics(t *testing.T) {
	data := SnappyRawEncode([]byte("same data"))
	id1 := ComputeMessageID(BlockTopic(), data)
	id2 := ComputeMessageID(AggregationTopic(), data)

	if bytes.Equal(id1, id2) {
		t.Fatal("different topics should produce different IDs")
	}
}

func TestComputeMessageIDInvalidSnappy(t *testing.T) {
	id := ComputeMessageID(BlockTopic(), []byte{0xff, 0xfe, 0xfd})
	if len(id) != 20 {
		t.Fatalf("should still produce 20-byte ID, got %d", len(id))
	}
}

func TestComputeMessageIDMatchesWireFormula(t *testing.T) {
	topic := BlockTopic()
	payload := []byte("block data")
	raw := SnappyRawEncode(payload)

	got := ComputeMessageID(topic, raw)
	want := messageIDDigest(domainValidSnappy, topic, payload)
	if !bytes.Equal(got, want) {
		t.Fatalf("valid snappy message ID=%x, want %x", got, want)
	}

	invalidRaw := []byte{0xff, 0xfe, 0xfd}
	got = ComputeMessageID(topic, invalidRaw)
	want = messageIDDigest(domainInvalidSnappy, topic, invalidRaw)
	if !bytes.Equal(got, want) {
		t.Fatalf("invalid snappy message ID=%x, want %x", got, want)
	}
}

func messageIDDigest(domain [4]byte, topic string, data []byte) []byte {
	h := sha256.New()
	var topicLen [8]byte
	topicBytes := []byte(topic)
	binary.LittleEndian.PutUint64(topicLen[:], uint64(len(topicBytes)))
	h.Write(domain[:])
	h.Write(topicLen[:])
	h.Write(topicBytes)
	h.Write(data)
	return h.Sum(nil)[:20]
}

func TestStatusMessageSSZRoundtrip(t *testing.T) {
	status := &StatusMessage{
		FinalizedRoot: [32]byte{0xab},
		FinalizedSlot: 42,
		HeadRoot:      [32]byte{0xcd},
		HeadSlot:      100,
	}
	data := status.MarshalSSZ()
	if len(data) != 80 {
		t.Fatalf("status SSZ should be 80 bytes, got %d", len(data))
	}

	decoded := &StatusMessage{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.FinalizedSlot != 42 || decoded.HeadSlot != 100 {
		t.Fatal("status roundtrip mismatch")
	}
	if decoded.FinalizedRoot != status.FinalizedRoot {
		t.Fatal("finalized root mismatch")
	}
	if decoded.HeadRoot != status.HeadRoot {
		t.Fatal("head root mismatch")
	}
}

func TestBlocksByRootRequestSSZRoundtrip(t *testing.T) {
	roots := [][32]byte{
		{0x01, 0x02, 0x03},
		{0xaa, 0xbb, 0xcc},
	}
	encoded := EncodeBlocksByRootRequest(roots)

	if len(encoded) != 68 {
		t.Fatalf("expected 68 bytes, got %d", len(encoded))
	}
	if encoded[0] != 4 || encoded[1] != 0 || encoded[2] != 0 || encoded[3] != 0 {
		t.Fatalf("unexpected offset bytes: %v", encoded[:4])
	}

	decoded, err := DecodeBlocksByRootRequest(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(decoded))
	}
	if decoded[0] != roots[0] || decoded[1] != roots[1] {
		t.Fatal("root mismatch")
	}
}

func TestBlocksByRootRequestSingleRoot36Bytes(t *testing.T) {
	roots := [][32]byte{{0xde, 0xad, 0xbe, 0xef}}
	encoded := EncodeBlocksByRootRequest(roots)
	if len(encoded) != 36 {
		t.Fatalf("expected 36 bytes for single root, got %d", len(encoded))
	}

	decoded, err := DecodeBlocksByRootRequest(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded) != 1 || decoded[0] != roots[0] {
		t.Fatal("single root roundtrip mismatch")
	}
}

func TestValidateFetchedBlockRootRejectsUnrequestedAndDuplicateBlocks(t *testing.T) {
	requestedBlock, requestedRoot := signedBlockForP2PTest(t, 1)
	otherBlock, _ := signedBlockForP2PTest(t, 2)

	requestedRoots := requestedRootSet([][32]byte{requestedRoot})
	seenRoots := make(map[[32]byte]bool)
	if err := validateFetchedBlockRoot(requestedBlock, requestedRoots, seenRoots); err != nil {
		t.Fatalf("requested block rejected: %v", err)
	}
	if err := validateFetchedBlockRoot(requestedBlock, requestedRoots, seenRoots); err == nil {
		t.Fatal("expected duplicate block root rejection")
	}
	if err := validateFetchedBlockRoot(otherBlock, requestedRoots, make(map[[32]byte]bool)); err == nil {
		t.Fatal("expected unrequested block root rejection")
	}
}

func signedBlockForP2PTest(t *testing.T, slot uint64) (*types.SignedBlock, [32]byte) {
	t.Helper()

	block := &types.Block{
		Slot: slot,
		Body: &types.BlockBody{},
	}
	root, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash test block: %v", err)
	}
	return &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{},
	}, root
}

func TestPeerStoreAddRemove(t *testing.T) {
	ps := NewPeerStore()
	if ps.Count() != 0 {
		t.Fatal("should start empty")
	}

	ps.Add("peer1")
	ps.Add("peer2")
	if ps.Count() != 2 {
		t.Fatalf("expected 2, got %d", ps.Count())
	}

	ps.Remove("peer1")
	if ps.Count() != 1 {
		t.Fatalf("expected 1, got %d", ps.Count())
	}
}

func TestPeerStoreRandomPeer(t *testing.T) {
	ps := NewPeerStore()
	ps.Add("peer1")
	ps.Add("peer2")
	ps.Add("peer3")

	exclude := map[peer.ID]bool{"peer1": true, "peer2": true}
	p := ps.RandomPeer(exclude)
	if p != "peer3" {
		t.Fatalf("expected peer3, got %s", p)
	}
}

func TestPeerStoreRandomPeerNoneAvailable(t *testing.T) {
	ps := NewPeerStore()
	p := ps.RandomPeer(nil)
	if p != "" {
		t.Fatal("should return empty when no peers")
	}
}
func TestLoadBootnodesEmpty(t *testing.T) {
	tmpFile := t.TempDir() + "/nodes.yaml"
	os.WriteFile(tmpFile, []byte("# empty\n"), 0644)

	addrs, err := LoadBootnodes(tmpFile)
	if err != nil {
		t.Fatalf("should not error on empty: %v", err)
	}
	if len(addrs) != 0 {
		t.Fatalf("expected 0 addrs, got %d", len(addrs))
	}
}

func TestLoadBootnodesWithYAMLList(t *testing.T) {
	content := `- "/ip4/127.0.0.1/udp/9000/quic-v1/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN"
- "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/12D3KooWLc4yBi3vYo4udihGu2HFxCWMWCdJoXYMFNp2CX9otY5A"
`
	tmpFile := t.TempDir() + "/nodes.yaml"
	os.WriteFile(tmpFile, []byte(content), 0644)

	addrs, err := LoadBootnodes(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addrs, got %d", len(addrs))
	}
}

func TestLoadBootnodesRejectsInvalidLine(t *testing.T) {
	tmpFile := t.TempDir() + "/nodes.yaml"
	if err := os.WriteFile(tmpFile, []byte("not-a-bootnode\n"), 0644); err != nil {
		t.Fatalf("write bootnodes: %v", err)
	}

	addrs, err := LoadBootnodes(tmpFile)
	if err == nil {
		t.Fatalf("expected invalid line error, got addrs=%v", addrs)
	}
	if !strings.Contains(err.Error(), "invalid bootnode line 1") {
		t.Fatalf("error=%v, want invalid line with number", err)
	}
}
