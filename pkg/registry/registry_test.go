package registry

import (
	"net"
	"testing"
)

func TestAllocateAndRelease(t *testing.T) {
	reg := New(8000, 8010)

	port, err := reg.Allocate(0)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	if port < 8000 || port > 8010 {
		t.Errorf("port = %d, want 8000-8010", port)
	}

	port2, _ := reg.Allocate(0)
	if port2 == port {
		t.Errorf("allocated same port twice: %d", port)
	}

	reg.Release(port)
	port3, _ := reg.Allocate(0)
	_ = port3
}

func TestAllocateSpecificPort(t *testing.T) {
	reg := New(8000, 9000)

	port, err := reg.Allocate(8080)
	if err != nil {
		t.Fatalf("Allocate specific: %v", err)
	}
	if port != 8080 {
		t.Errorf("port = %d, want 8080", port)
	}

	_, err = reg.Allocate(8080)
	if err == nil {
		t.Error("expected error for duplicate port allocation")
	}
}

func TestAllocateExhausted(t *testing.T) {
	reg := New(8000, 8002)

	for i := 0; i < 3; i++ {
		if _, err := reg.Allocate(0); err != nil {
			t.Fatalf("Allocate %d: %v", i, err)
		}
	}

	_, err := reg.Allocate(0)
	if err == nil {
		t.Error("expected error when port range exhausted")
	}
}

func TestPortInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	reg := New(port, port)
	_, err = reg.Allocate(port)
	if err == nil {
		t.Error("expected error for port already in use")
	}
}
