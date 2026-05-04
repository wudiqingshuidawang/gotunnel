// gotunnel/integration_test.go
package gotunnel_test

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/yan/gotunnel/pkg/tunnel"
)

func TestEndToEnd(t *testing.T) {
	// 1. Start a mock local service (echo server)
	localLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("local listen: %v", err)
	}
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
				io.Copy(c, c) // echo
			}(conn)
		}
	}()

	// 2. Start server on random port
	srv := tunnel.NewServer("127.0.0.1:0")
	go srv.Start()
	defer srv.Stop()

	// Wait for server to be ready and get its address
	serverAddr := srv.Addr()

	// 3. Start client
	client := tunnel.NewClient(serverAddr)
	client.AddTunnel(localPort, 0)
	client.SetDialTimeout(2 * time.Second)

	if err := client.Connect(); err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer client.Close()

	go client.Run()
	time.Sleep(200 * time.Millisecond)

	// 4. Connect to the public port
	publicPort := client.RemotePort()
	if publicPort == 0 {
		t.Fatal("remote port not assigned")
	}

	publicConn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", publicPort), 2*time.Second)
	if err != nil {
		t.Fatalf("dial public: %v", err)
	}
	defer publicConn.Close()

	// 5. Send data through the tunnel
	testData := []byte("hello gotunnel!")
	publicConn.Write(testData)
	publicConn.(*net.TCPConn).CloseWrite()

	// 6. Read response
	buf := make([]byte, 1024)
	publicConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := publicConn.Read(buf)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if string(buf[:n]) != string(testData) {
		t.Errorf("response = %q, want %q", buf[:n], testData)
	}
}

func TestMultiTunnel(t *testing.T) {
	// 1. Start two echo services on different ports
	echoServer := func(ln net.Listener) {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}

	localLn1, _ := net.Listen("tcp", "127.0.0.1:0")
	defer localLn1.Close()
	go echoServer(localLn1)

	localLn2, _ := net.Listen("tcp", "127.0.0.1:0")
	defer localLn2.Close()
	go echoServer(localLn2)

	localPort1 := localLn1.Addr().(*net.TCPAddr).Port
	localPort2 := localLn2.Addr().(*net.TCPAddr).Port

	// 2. Start server
	srv := tunnel.NewServer("127.0.0.1:0")
	go srv.Start()
	defer srv.Stop()
	serverAddr := srv.Addr()

	// 3. Client registers two tunnels
	client := tunnel.NewClient(serverAddr)
	client.AddTunnel(localPort1, 0)
	client.AddTunnel(localPort2, 0)
	client.SetDialTimeout(2 * time.Second)

	if err := client.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()
	go client.Run()
	time.Sleep(200 * time.Millisecond)

	ports := client.RemotePorts()
	if len(ports) != 2 {
		t.Fatalf("expected 2 remote ports, got %d", len(ports))
	}

	// 4. Test tunnel 1: send data, expect echo from local service 1
	conn1, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", ports[0]), 2*time.Second)
	if err != nil {
		t.Fatalf("dial tunnel 1: %v", err)
	}
	defer conn1.Close()

	conn1.Write([]byte("tunnel-1-data"))
	conn1.(*net.TCPConn).CloseWrite()
	buf1 := make([]byte, 1024)
	conn1.SetReadDeadline(time.Now().Add(3 * time.Second))
	n1, _ := conn1.Read(buf1)
	if string(buf1[:n1]) != "tunnel-1-data" {
		t.Errorf("tunnel 1 response = %q, want %q", buf1[:n1], "tunnel-1-data")
	}

	// 5. Test tunnel 2: send data, expect echo from local service 2
	conn2, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", ports[1]), 2*time.Second)
	if err != nil {
		t.Fatalf("dial tunnel 2: %v", err)
	}
	defer conn2.Close()

	conn2.Write([]byte("tunnel-2-data"))
	conn2.(*net.TCPConn).CloseWrite()
	buf2 := make([]byte, 1024)
	conn2.SetReadDeadline(time.Now().Add(3 * time.Second))
	n2, _ := conn2.Read(buf2)
	if string(buf2[:n2]) != "tunnel-2-data" {
		t.Errorf("tunnel 2 response = %q, want %q", buf2[:n2], "tunnel-2-data")
	}
}
