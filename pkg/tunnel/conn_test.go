// gotunnel/pkg/tunnel/conn_test.go
package tunnel

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func TestRelay(t *testing.T) {
	server1, client1 := net.Pipe()
	server2, client2 := net.Pipe()
	defer server1.Close()
	defer client1.Close()
	defer server2.Close()
	defer client2.Close()

	done := make(chan struct{})
	go func() {
		Relay(client1, client2)
		close(done)
	}()

	go func() {
		server1.Write([]byte("hello from side 1"))
		server1.Close()
	}()

	buf := new(bytes.Buffer)
	io.Copy(buf, server2)

	if buf.String() != "hello from side 1" {
		t.Errorf("got %q, want 'hello from side 1'", buf.String())
	}
}

func TestRelayBidirectional(t *testing.T) {
	server1, client1 := net.Pipe()
	server2, client2 := net.Pipe()
	defer server1.Close()
	defer client1.Close()
	defer server2.Close()
	defer client2.Close()

	go Relay(client1, client2)

	go func() {
		server1.Write([]byte("ping"))
		server1.Close()
	}()
	go func() {
		server2.Write([]byte("pong"))
		server2.Close()
	}()

	buf1 := new(bytes.Buffer)
	buf2 := new(bytes.Buffer)

	done := make(chan struct{})
	go func() {
		io.Copy(buf1, server1)
		close(done)
	}()
	io.Copy(buf2, server2)
	<-done

	total := buf1.String() + buf2.String()
	if len(total) == 0 {
		t.Error("no data relayed")
	}
}

func TestRelayClosesOnEOF(t *testing.T) {
	server, client := net.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- RelayPair(server, client)
	}()

	server.Close()

	select {
	case err := <-done:
		_ = err
	case <-time.After(2 * time.Second):
		t.Error("RelayPair did not exit after close")
	}
}
