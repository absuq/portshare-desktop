package protocol

import (
	"bytes"
	"testing"
)

func TestProofMatchesWithSameSecret(t *testing.T) {
	nonceA := []byte("initiator-nonce")
	nonceB := []byte("responder-nonce")
	proof := ComputeProof("shared-secret", "device-a", "device-b", nonceA, nonceB)
	if !VerifyProof("shared-secret", "device-a", "device-b", nonceA, nonceB, proof) {
		t.Fatalf("expected proof to verify")
	}
}

func TestProofFailsWithDifferentSecret(t *testing.T) {
	nonceA := []byte("initiator-nonce")
	nonceB := []byte("responder-nonce")
	proof := ComputeProof("right-secret", "device-a", "device-b", nonceA, nonceB)
	if VerifyProof("wrong-secret", "device-a", "device-b", nonceA, nonceB, proof) {
		t.Fatalf("expected wrong secret to fail")
	}
}

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	msg := ControlMessage{Type: TypeHello, Version: Version, DeviceID: "device-a"}
	if err := WriteFrame(&buf, msg); err != nil {
		t.Fatal(err)
	}
	var got ControlMessage
	if err := ReadFrame(&buf, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != TypeHello || got.DeviceID != "device-a" {
		t.Fatalf("unexpected message: %+v", got)
	}
}

type shortWriter struct {
	buf   bytes.Buffer
	limit int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		p = p[:w.limit]
	}
	return w.buf.Write(p)
}

func TestFrameRoundTripWithShortWriter(t *testing.T) {
	writer := &shortWriter{limit: 2}
	msg := ControlMessage{Type: TypeHello, Version: Version, DeviceID: "device-a"}
	if err := WriteFrame(writer, msg); err != nil {
		t.Fatal(err)
	}
	var got ControlMessage
	if err := ReadFrame(&writer.buf, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != TypeHello || got.Version != Version || got.DeviceID != "device-a" {
		t.Fatalf("unexpected message: %+v", got)
	}
}
