package protocol

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer

	frame := Frame{
		Type:    MsgTypeRegister,
		Payload: []byte(`{"local_port":3000}`),
	}

	if err := WriteFrame(&buf, frame); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}

	if got.Type != frame.Type {
		t.Errorf("Type = %d, want %d", got.Type, frame.Type)
	}
	if !bytes.Equal(got.Payload, frame.Payload) {
		t.Errorf("Payload = %s, want %s", got.Payload, frame.Payload)
	}
}

func TestFrameEmptyPayload(t *testing.T) {
	var buf bytes.Buffer

	frame := Frame{Type: MsgTypeHeartbeat, Payload: nil}
	if err := WriteFrame(&buf, frame); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}

	if got.Type != MsgTypeHeartbeat {
		t.Errorf("Type = %d, want %d", got.Type, MsgTypeHeartbeat)
	}
	if len(got.Payload) != 0 {
		t.Errorf("Payload should be empty, got %d bytes", len(got.Payload))
	}
}

func TestFrameOverConnection(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	frame := Frame{
		Type:    MsgTypeData,
		Payload: []byte("hello world"),
	}

	go func() {
		WriteFrame(client, frame)
	}()

	got, err := ReadFrame(server)
	if err != nil {
		t.Fatalf("ReadFrame over conn: %v", err)
	}

	if got.Type != frame.Type {
		t.Errorf("Type = %d, want %d", got.Type, frame.Type)
	}
	if string(got.Payload) != "hello world" {
		t.Errorf("Payload = %s, want 'hello world'", got.Payload)
	}
}

func TestFrameLargePayload(t *testing.T) {
	var buf bytes.Buffer

	bigPayload := make([]byte, 64*1024)
	for i := range bigPayload {
		bigPayload[i] = byte(i % 256)
	}

	frame := Frame{Type: MsgTypeData, Payload: bigPayload}
	if err := WriteFrame(&buf, frame); err != nil {
		t.Fatalf("WriteFrame large: %v", err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame large: %v", err)
	}

	if !bytes.Equal(got.Payload, bigPayload) {
		t.Error("large payload mismatch")
	}
}

func TestFrameRejectsOversizedPayload(t *testing.T) {
	var buf bytes.Buffer
	// Write a header with length > MaxPayloadSize
	header := make([]byte, headerSize)
	header[0] = MsgTypeData
	binary.BigEndian.PutUint32(header[1:5], MaxPayloadSize+1)
	buf.Write(header)

	_, err := ReadFrame(&buf)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFrameAcceptsMaxPayload(t *testing.T) {
	var buf bytes.Buffer
	payload := make([]byte, MaxPayloadSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	frame := Frame{Type: MsgTypeData, Payload: payload}
	if err := WriteFrame(&buf, frame); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !bytes.Equal(got.Payload, payload) {
		t.Error("payload mismatch")
	}
}

func TestFrameRejectsGigabyteLength(t *testing.T) {
	var buf bytes.Buffer
	header := make([]byte, headerSize)
	header[0] = MsgTypeData
	binary.BigEndian.PutUint32(header[1:5], 0xFFFFFFFF)
	buf.Write(header)

	_, err := ReadFrame(&buf)
	if err == nil {
		t.Fatal("expected error for gigabyte length")
	}
}
