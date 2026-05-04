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
	client := tunnel.NewClient(serverAddr, localPort, 0)
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
