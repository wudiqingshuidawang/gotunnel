// gotunnel/pkg/tunnel/server_test.go
package tunnel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"testing"
	"time"

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

func TestServerRejectsUnauthenticatedClient(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	srv.SetToken("secret")
	go srv.Start()
	defer srv.Stop()

	addr := srv.Addr()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send REGISTER without AUTH
	regMsg := protocol.RegisterMsg{LocalPort: 3000}
	frame, _ := protocol.ToFrame(protocol.MsgTypeRegister, regMsg)
	protocol.WriteFrame(conn, frame)

	resp, err := protocol.ReadFrame(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.Type != protocol.MsgTypeError {
		t.Errorf("type = %d, want %d", resp.Type, protocol.MsgTypeError)
	}
}

func TestServerRejectsWrongToken(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	srv.SetToken("secret")
	go srv.Start()
	defer srv.Stop()

	addr := srv.Addr()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send AUTH with wrong token
	authFrame, _ := protocol.ToFrame(protocol.MsgTypeAuth, protocol.AuthMsg{Token: "wrong"})
	protocol.WriteFrame(conn, authFrame)

	resp, _ := protocol.ReadFrame(conn)
	if resp.Type != protocol.MsgTypeError {
		t.Errorf("type = %d, want %d", resp.Type, protocol.MsgTypeError)
	}
}

func TestServerAcceptsValidToken(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	srv.SetToken("secret")
	go srv.Start()
	defer srv.Stop()

	addr := srv.Addr()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send AUTH with correct token
	authFrame, _ := protocol.ToFrame(protocol.MsgTypeAuth, protocol.AuthMsg{Token: "secret"})
	protocol.WriteFrame(conn, authFrame)

	resp, err := protocol.ReadFrame(conn)
	if err != nil {
		t.Fatalf("read auth ack: %v", err)
	}
	if resp.Type != protocol.MsgTypeAuthAck {
		t.Errorf("type = %d, want %d", resp.Type, protocol.MsgTypeAuthAck)
	}

	// Now REGISTER should work
	regFrame, _ := protocol.ToFrame(protocol.MsgTypeRegister, protocol.RegisterMsg{LocalPort: 3000})
	protocol.WriteFrame(conn, regFrame)

	ack, err := protocol.ReadFrame(conn)
	if err != nil {
		t.Fatalf("read register ack: %v", err)
	}
	if ack.Type != protocol.MsgTypeRegisterAck {
		t.Errorf("type = %d, want %d", ack.Type, protocol.MsgTypeRegisterAck)
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

func TestServerMaxClients(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	srv.SetMaxClients(1)
	go srv.Start()
	defer srv.Stop()

	addr := srv.Addr()

	// Client 1 should succeed
	conn1, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial client1: %v", err)
	}
	defer conn1.Close()

	regMsg := protocol.RegisterMsg{LocalPort: 3000}
	frame, _ := protocol.ToFrame(protocol.MsgTypeRegister, regMsg)
	protocol.WriteFrame(conn1, frame)

	ack, err := protocol.ReadFrame(conn1)
	if err != nil {
		t.Fatalf("read ack client1: %v", err)
	}
	if ack.Type != protocol.MsgTypeRegisterAck {
		t.Fatalf("client1: type = %d, want %d", ack.Type, protocol.MsgTypeRegisterAck)
	}

	// Client 2 should be rejected
	conn2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial client2: %v", err)
	}
	defer conn2.Close()

	// The server sends the capacity error immediately on connect, no need to send anything
	resp, err := protocol.ReadFrame(conn2)
	if err != nil {
		t.Fatalf("read error from client2: %v", err)
	}
	if resp.Type != protocol.MsgTypeError {
		t.Fatalf("client2: type = %d, want %d", resp.Type, protocol.MsgTypeError)
	}

	var errMsg protocol.ErrorMsg
	protocol.FromFrame(resp, &errMsg)
	if errMsg.Code != 5 {
		t.Errorf("error code = %d, want 5", errMsg.Code)
	}
}

func TestServerMaxTunnelsPerClient(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	srv.SetMaxTunnelsPerClient(1)
	go srv.Start()
	defer srv.Stop()

	addr := srv.Addr()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// First tunnel should succeed
	regMsg := protocol.RegisterMsg{LocalPort: 3000}
	frame, _ := protocol.ToFrame(protocol.MsgTypeRegister, regMsg)
	protocol.WriteFrame(conn, frame)

	ack, err := protocol.ReadFrame(conn)
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.Type != protocol.MsgTypeRegisterAck {
		t.Fatalf("type = %d, want %d", ack.Type, protocol.MsgTypeRegisterAck)
	}

	// Second tunnel should be rejected
	regMsg2 := protocol.RegisterMsg{LocalPort: 3001}
	frame2, _ := protocol.ToFrame(protocol.MsgTypeRegister, regMsg2)
	protocol.WriteFrame(conn, frame2)

	resp, err := protocol.ReadFrame(conn)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if resp.Type != protocol.MsgTypeError {
		t.Fatalf("type = %d, want %d", resp.Type, protocol.MsgTypeError)
	}

	var errMsg protocol.ErrorMsg
	protocol.FromFrame(resp, &errMsg)
	if errMsg.Code != 6 {
		t.Errorf("error code = %d, want 6", errMsg.Code)
	}
}

func TestServerMaxSessions(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	srv.SetMaxSessions(10)
	if srv.maxSessions != 10 {
		t.Errorf("maxSessions = %d, want 10", srv.maxSessions)
	}
}

func TestServerHeartbeatTimeout(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	srv.SetClientTimeout(100 * time.Millisecond)
	go srv.Start()
	defer srv.Stop()

	addr := srv.Addr()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Don't send any frames — server should close the connection
	// within the timeout window
	deadline := time.After(500 * time.Millisecond)
	buf := make([]byte, 1)
	done := make(chan error, 1)
	go func() {
		_, err := conn.Read(buf)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected connection to be closed by server")
		}
		// Connection closed by server — success
	case <-deadline:
		t.Fatal("timed out waiting for server to close connection")
	}
}

func TestServerHeartbeatResetsTimer(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	srv.SetClientTimeout(200 * time.Millisecond)
	go srv.Start()
	defer srv.Stop()

	addr := srv.Addr()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send heartbeats to keep the connection alive
	for i := 0; i < 3; i++ {
		time.Sleep(100 * time.Millisecond)
		hbFrame, _ := protocol.ToFrame(protocol.MsgTypeHeartbeat, nil)
		protocol.WriteFrame(conn, hbFrame)

		// Read heartbeat response
		resp, err := protocol.ReadFrame(conn)
		if err != nil {
			t.Fatalf("heartbeat %d: read response: %v", i, err)
		}
		if resp.Type != protocol.MsgTypeHeartbeat {
			t.Fatalf("heartbeat %d: type = %d, want %d", i, resp.Type, protocol.MsgTypeHeartbeat)
		}
	}

	// Connection should still be alive after 300ms of heartbeats
	conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, err = conn.Read(make([]byte, 1))
	if err == nil {
		// If we can still read, that's fine — connection is alive
	} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		// Timeout is fine — connection is alive, just no data
	} else {
		// Connection was closed — heartbeat didn't reset timer
		t.Fatalf("connection closed unexpectedly: %v", err)
	}
}

func TestServerTLS(t *testing.T) {
	// Generate self-signed cert for server
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"gotunnel-test"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	srv := NewServer("127.0.0.1:0")
	srv.SetTLSConfig(&tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	go srv.Start()
	defer srv.Stop()

	addr := srv.Addr()
	if addr == "" {
		t.Fatal("server address is empty after start")
	}

	// TLS client with InsecureSkipVerify (self-signed cert)
	tlsConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatalf("tls dial: %v", err)
	}
	defer tlsConn.Close()

	// Send REGISTER over TLS
	regMsg := protocol.RegisterMsg{LocalPort: 3000}
	frame, _ := protocol.ToFrame(protocol.MsgTypeRegister, regMsg)
	protocol.WriteFrame(tlsConn, frame)

	ackFrame, err := protocol.ReadFrame(tlsConn)
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ackFrame.Type != protocol.MsgTypeRegisterAck {
		t.Errorf("type = %d, want %d", ackFrame.Type, protocol.MsgTypeRegisterAck)
	}

	// Plain TCP connection should fail (TLS-only server)
	plainConn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		// Connection refused or similar — expected
		return
	}
	defer plainConn.Close()

	// If we somehow connected, sending plain data should fail
	plainConn.Write([]byte("plain data"))
	_, err = plainConn.Read(make([]byte, 1))
	if err == nil {
		t.Error("expected plain TCP connection to fail against TLS server")
	}
}
