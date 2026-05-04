// gotunnel/pkg/protocol/message_test.go
package protocol

import (
	"testing"
)

func TestRegisterMessage(t *testing.T) {
	msg := RegisterMsg{LocalPort: 3000}
	frame, err := ToFrame(MsgTypeRegister, msg)
	if err != nil {
		t.Fatalf("ToFrame: %v", err)
	}

	if frame.Type != MsgTypeRegister {
		t.Errorf("Type = %d, want %d", frame.Type, MsgTypeRegister)
	}

	var got RegisterMsg
	if err := FromFrame(frame, &got); err != nil {
		t.Fatalf("FromFrame: %v", err)
	}

	if got.LocalPort != 3000 {
		t.Errorf("LocalPort = %d, want 3000", got.LocalPort)
	}
}

func TestRegisterAckMessage(t *testing.T) {
	msg := RegisterAckMsg{RemotePort: 8080}
	frame, _ := ToFrame(MsgTypeRegisterAck, msg)

	var got RegisterAckMsg
	FromFrame(frame, &got)

	if got.RemotePort != 8080 {
		t.Errorf("RemotePort = %d, want 8080", got.RemotePort)
	}
}

func TestNewConnMessage(t *testing.T) {
	msg := NewConnMsg{ConnID: "abc-123"}
	frame, _ := ToFrame(MsgTypeNewConn, msg)

	var got NewConnMsg
	FromFrame(frame, &got)

	if got.ConnID != "abc-123" {
		t.Errorf("ConnID = %s, want abc-123", got.ConnID)
	}
}

func TestDataMessage(t *testing.T) {
	original := []byte("hello world")
	msg := DataMsg{ConnID: "test-conn", Data: original}
	frame, _ := ToFrame(MsgTypeData, msg)

	var got DataMsg
	FromFrame(frame, &got)

	if got.ConnID != "test-conn" {
		t.Errorf("ConnID = %s, want test-conn", got.ConnID)
	}
	if string(got.Data) != "hello world" {
		t.Errorf("Data = %s, want 'hello world'", got.Data)
	}
}

func TestCloseMessage(t *testing.T) {
	msg := CloseMsg{ConnID: "closing-conn"}
	frame, _ := ToFrame(MsgTypeClose, msg)

	var got CloseMsg
	FromFrame(frame, &got)

	if got.ConnID != "closing-conn" {
		t.Errorf("ConnID = %s, want closing-conn", got.ConnID)
	}
}

func TestErrorMessage(t *testing.T) {
	msg := ErrorMsg{Code: 1, Msg: "port in use"}
	frame, _ := ToFrame(MsgTypeError, msg)

	var got ErrorMsg
	FromFrame(frame, &got)

	if got.Code != 1 || got.Msg != "port in use" {
		t.Errorf("Error = {%d, %s}, want {1, 'port in use'}", got.Code, got.Msg)
	}
}

func TestHeartbeatMessage(t *testing.T) {
	frame := Frame{Type: MsgTypeHeartbeat, Payload: nil}

	var got struct{}
	if err := FromFrame(frame, &got); err != nil {
		t.Fatalf("FromFrame heartbeat: %v", err)
	}
}

func TestAuthMessage(t *testing.T) {
	msg := AuthMsg{Token: "secret123"}
	frame, _ := ToFrame(MsgTypeAuth, msg)
	var got AuthMsg
	FromFrame(frame, &got)
	if got.Token != "secret123" {
		t.Errorf("Token = %s, want secret123", got.Token)
	}
}
