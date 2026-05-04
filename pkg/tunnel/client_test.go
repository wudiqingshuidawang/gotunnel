// gotunnel/pkg/tunnel/client_test.go
package tunnel

import (
	"net"
	"testing"
	"time"

	"github.com/yan/gotunnel/pkg/protocol"
)

func TestClientConnectsAndRegisters(t *testing.T) {
	serverLn, _ := net.Listen("tcp", "127.0.0.1:0")
	defer serverLn.Close()

	go func() {
		conn, _ := serverLn.Accept()
		defer conn.Close()

		frame, _ := protocol.ReadFrame(conn)
		if frame.Type != protocol.MsgTypeRegister {
			t.Errorf("server: type = %d, want %d", frame.Type, protocol.MsgTypeRegister)
		}

		ack, _ := protocol.ToFrame(protocol.MsgTypeRegisterAck, protocol.RegisterAckMsg{RemotePort: 8080})
		protocol.WriteFrame(conn, ack)

		time.Sleep(500 * time.Millisecond)
	}()

	localLn, _ := net.Listen("tcp", "127.0.0.1:0")
	defer localLn.Close()
	localPort := localLn.Addr().(*net.TCPAddr).Port

	client := NewClient(serverLn.Addr().String(), localPort, 0)
	client.SetDialTimeout(1 * time.Second)

	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	if client.RemotePort() != 8080 {
		t.Errorf("RemotePort = %d, want 8080", client.RemotePort())
	}
}

func TestClientHandlesNewConn(t *testing.T) {
	serverLn, _ := net.Listen("tcp", "127.0.0.1:0")
	defer serverLn.Close()

	localLn, _ := net.Listen("tcp", "127.0.0.1:0")
	defer localLn.Close()
	localPort := localLn.Addr().(*net.TCPAddr).Port

	go func() {
		for {
			conn, err := localLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				n, _ := c.Read(buf)
				c.Write(buf[:n])
			}(conn)
		}
	}()

	go func() {
		conn, _ := serverLn.Accept()
		defer conn.Close()

		protocol.ReadFrame(conn)

		ack, _ := protocol.ToFrame(protocol.MsgTypeRegisterAck, protocol.RegisterAckMsg{RemotePort: 8080})
		protocol.WriteFrame(conn, ack)

		newConn, _ := protocol.ToFrame(protocol.MsgTypeNewConn, protocol.NewConnMsg{ConnID: "test-conn-1"})
		protocol.WriteFrame(conn, newConn)

		frame, err := protocol.ReadFrame(conn)
		if err != nil {
			return
		}
		if frame.Type != protocol.MsgTypeData {
			t.Errorf("server: expected DATA, got %d", frame.Type)
		}

		protocol.WriteFrame(conn, frame)

		time.Sleep(200 * time.Millisecond)
	}()

	client := NewClient(serverLn.Addr().String(), localPort, 0)
	client.SetDialTimeout(1 * time.Second)

	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	go client.Run()
	time.Sleep(1 * time.Second)
}
