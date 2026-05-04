// gotunnel/pkg/tunnel/client_test.go
package tunnel

import (
	"fmt"
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

	client := NewClient(serverLn.Addr().String())
	client.AddTunnel(localPort, 0)
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

		newConn, _ := protocol.ToFrame(protocol.MsgTypeNewConn, protocol.NewConnMsg{ConnID: "test-conn-1", RemotePort: 8080})
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

	client := NewClient(serverLn.Addr().String())
	client.AddTunnel(localPort, 0)
	client.SetDialTimeout(1 * time.Second)

	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	go client.Run()
	time.Sleep(1 * time.Second)
}

func TestClientMultiTunnelRegistration(t *testing.T) {
	serverLn, _ := net.Listen("tcp", "127.0.0.1:0")
	defer serverLn.Close()

	go func() {
		conn, _ := serverLn.Accept()
		defer conn.Close()

		// Should receive two REGISTER frames
		for i := 0; i < 2; i++ {
			frame, err := protocol.ReadFrame(conn)
			if err != nil {
				return
			}
			if frame.Type != protocol.MsgTypeRegister {
				t.Errorf("server: frame %d type = %d, want %d", i, frame.Type, protocol.MsgTypeRegister)
			}

			var msg protocol.RegisterMsg
			protocol.FromFrame(frame, &msg)

			remotePort := 8080 + i
			ack, _ := protocol.ToFrame(protocol.MsgTypeRegisterAck, protocol.RegisterAckMsg{RemotePort: remotePort})
			protocol.WriteFrame(conn, ack)
		}

		time.Sleep(500 * time.Millisecond)
	}()

	localLn1, _ := net.Listen("tcp", "127.0.0.1:0")
	defer localLn1.Close()
	localLn2, _ := net.Listen("tcp", "127.0.0.1:0")
	defer localLn2.Close()

	client := NewClient(serverLn.Addr().String())
	client.AddTunnel(localLn1.Addr().(*net.TCPAddr).Port, 0)
	client.AddTunnel(localLn2.Addr().(*net.TCPAddr).Port, 0)
	client.SetDialTimeout(1 * time.Second)

	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	ports := client.RemotePorts()
	if len(ports) != 2 {
		t.Fatalf("RemotePorts len = %d, want 2", len(ports))
	}

	// Ports should be 8080 and 8081 (order may vary)
	portSet := make(map[int]bool)
	for _, p := range ports {
		portSet[p] = true
	}
	if !portSet[8080] || !portSet[8081] {
		t.Errorf("RemotePorts = %v, want {8080, 8081}", ports)
	}
}

func TestClientRouteLookup(t *testing.T) {
	localLn1, _ := net.Listen("tcp", "127.0.0.1:0")
	defer localLn1.Close()
	localLn2, _ := net.Listen("tcp", "127.0.0.1:0")
	defer localLn2.Close()

	localPort1 := localLn1.Addr().(*net.TCPAddr).Port
	localPort2 := localLn2.Addr().(*net.TCPAddr).Port

	serverLn, _ := net.Listen("tcp", "127.0.0.1:0")
	defer serverLn.Close()

	// Capture what each local service receives
	received1 := make(chan string, 1)
	received2 := make(chan string, 1)

	go func() {
		conn, _ := localLn1.Accept()
		defer conn.Close()
		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		received1 <- string(buf[:n])
	}()
	go func() {
		conn, _ := localLn2.Accept()
		defer conn.Close()
		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		received2 <- string(buf[:n])
	}()

	serverDone := make(chan error, 1)
	go func() {
		conn, _ := serverLn.Accept()
		defer conn.Close()

		// Two REGISTERs
		for i := 0; i < 2; i++ {
			protocol.ReadFrame(conn)
			remotePort := 9000 + i
			ack, _ := protocol.ToFrame(protocol.MsgTypeRegisterAck, protocol.RegisterAckMsg{RemotePort: remotePort})
			protocol.WriteFrame(conn, ack)
		}

		// NEW_CONN for tunnel 1 (remote port 9000)
		nc1, _ := protocol.ToFrame(protocol.MsgTypeNewConn, protocol.NewConnMsg{ConnID: "conn-1", RemotePort: 9000})
		protocol.WriteFrame(conn, nc1)

		// Send DATA to tunnel 1 — client should route to local service 1
		data1, _ := protocol.ToFrame(protocol.MsgTypeData, protocol.DataMsg{ConnID: "conn-1", Data: []byte("to-tunnel-1")})
		protocol.WriteFrame(conn, data1)

		// NEW_CONN for tunnel 2 (remote port 9001)
		nc2, _ := protocol.ToFrame(protocol.MsgTypeNewConn, protocol.NewConnMsg{ConnID: "conn-2", RemotePort: 9001})
		protocol.WriteFrame(conn, nc2)

		// Send DATA to tunnel 2 — client should route to local service 2
		data2, _ := protocol.ToFrame(protocol.MsgTypeData, protocol.DataMsg{ConnID: "conn-2", Data: []byte("to-tunnel-2")})
		protocol.WriteFrame(conn, data2)

		// Wait for both local services to receive data
		select {
		case d := <-received1:
			if d != "to-tunnel-1" {
				serverDone <- fmt.Errorf("local service 1 got %q, want %q", d, "to-tunnel-1")
				return
			}
		case <-time.After(3 * time.Second):
			serverDone <- fmt.Errorf("timeout waiting for local service 1")
			return
		}

		select {
		case d := <-received2:
			if d != "to-tunnel-2" {
				serverDone <- fmt.Errorf("local service 2 got %q, want %q", d, "to-tunnel-2")
				return
			}
		case <-time.After(3 * time.Second):
			serverDone <- fmt.Errorf("timeout waiting for local service 2")
			return
		}

		serverDone <- nil
	}()

	client := NewClient(serverLn.Addr().String())
	client.AddTunnel(localPort1, 0)
	client.AddTunnel(localPort2, 0)
	client.SetDialTimeout(1 * time.Second)

	if err := client.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	go client.Run()

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("server goroutine: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server goroutine")
	}
}
