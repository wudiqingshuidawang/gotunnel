// gotunnel/pkg/tunnel/server_test.go
package tunnel

import (
	"fmt"
	"net"
	"testing"

	"github.com/yan/gotunnel/pkg/protocol"
)

func TestServerAcceptsClient(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	go srv.Start()
	defer srv.Stop()

	addr := srv.Addr()
	if addr == "" {
		t.Fatal("server address is empty after start")
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	regMsg := protocol.RegisterMsg{LocalPort: 3000}
	frame, _ := protocol.ToFrame(protocol.MsgTypeRegister, regMsg)
	protocol.WriteFrame(conn, frame)

	gotFrame, err := protocol.ReadFrame(conn)
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}

	if gotFrame.Type != protocol.MsgTypeRegisterAck {
		t.Errorf("type = %d, want %d", gotFrame.Type, protocol.MsgTypeRegisterAck)
	}

	var ack protocol.RegisterAckMsg
	protocol.FromFrame(gotFrame, &ack)

	if ack.RemotePort == 0 {
		t.Error("expected non-zero remote port")
	}
}

func TestServerForwardsData(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	go srv.Start()
	defer srv.Stop()

	addr := srv.Addr()
	clientConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial control: %v", err)
	}
	defer clientConn.Close()

	regMsg := protocol.RegisterMsg{LocalPort: 3000}
	frame, _ := protocol.ToFrame(protocol.MsgTypeRegister, regMsg)
	protocol.WriteFrame(clientConn, frame)

	ackFrame, _ := protocol.ReadFrame(clientConn)
	var ack protocol.RegisterAckMsg
	protocol.FromFrame(ackFrame, &ack)

	publicConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", ack.RemotePort))
	if err != nil {
		t.Fatalf("dial public: %v", err)
	}
	defer publicConn.Close()

	newConnFrame, err := protocol.ReadFrame(clientConn)
	if err != nil {
		t.Fatalf("read new conn: %v", err)
	}
	if newConnFrame.Type != protocol.MsgTypeNewConn {
		t.Errorf("type = %d, want %d", newConnFrame.Type, protocol.MsgTypeNewConn)
	}

	var newConnMsg protocol.NewConnMsg
	protocol.FromFrame(newConnFrame, &newConnMsg)

	publicConn.Write([]byte("hello"))

	dataFrame, err := protocol.ReadFrame(clientConn)
	if err != nil {
		t.Fatalf("read data: %v", err)
	}
	if dataFrame.Type != protocol.MsgTypeData {
		t.Errorf("type = %d, want %d", dataFrame.Type, protocol.MsgTypeData)
	}

	var dataMsg protocol.DataMsg
	protocol.FromFrame(dataFrame, &dataMsg)

	if string(dataMsg.Data) != "hello" {
		t.Errorf("data = %s, want 'hello'", dataMsg.Data)
	}
	if dataMsg.ConnID != newConnMsg.ConnID {
		t.Errorf("connID mismatch: %s vs %s", dataMsg.ConnID, newConnMsg.ConnID)
	}
}
