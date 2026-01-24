package p2p

import (
	"bytes"
	"testing"
)

func TestDefaultGossipsubParams(t *testing.T) {
	params := DefaultGossipsubParams()

	if params.D != 8 {
		t.Errorf("D = %d, want 8", params.D)
	}
	if params.DLow != 6 {
		t.Errorf("DLow = %d, want 6", params.DLow)
	}
	if params.DHigh != 12 {
		t.Errorf("DHigh = %d, want 12", params.DHigh)
	}
	if params.ValidationMode != "strict_no_sign" {
		t.Errorf("ValidationMode = %s, want strict_no_sign", params.ValidationMode)
	}
	if params.SeenTTL != 256 {
		t.Errorf("SeenTTL = %d, want 256", params.SeenTTL)
	}
}

func TestComputeMessageID(t *testing.T) {
	topic := []byte("/leanconsensus/blocks/ssz_snappy")
	data := []byte{0x01, 0x02, 0x03, 0x04}

	// Valid snappy
	id1 := ComputeMessageID(topic, data, true)
	// Invalid snappy
	id2 := ComputeMessageID(topic, data, false)

	// IDs should be different due to different domains
	if bytes.Equal(id1[:], id2[:]) {
		t.Error("expected different IDs for valid vs invalid snappy")
	}

	// ID should be 20 bytes
	if len(id1) != 20 {
		t.Errorf("ID length = %d, want 20", len(id1))
	}

	// Same input should produce same output
	id3 := ComputeMessageID(topic, data, true)
	if !bytes.Equal(id1[:], id3[:]) {
		t.Error("expected same ID for same input")
	}
}

func TestComputeMessageID_DifferentTopics(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}

	id1 := ComputeMessageID([]byte("topic1"), data, true)
	id2 := ComputeMessageID([]byte("topic2"), data, true)

	if bytes.Equal(id1[:], id2[:]) {
		t.Error("expected different IDs for different topics")
	}
}

func TestComputeMessageID_DifferentData(t *testing.T) {
	topic := []byte("topic")

	id1 := ComputeMessageID(topic, []byte{0x01}, true)
	id2 := ComputeMessageID(topic, []byte{0x02}, true)

	if bytes.Equal(id1[:], id2[:]) {
		t.Error("expected different IDs for different data")
	}
}
