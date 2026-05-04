// gotunnel/pkg/tunnel/conn.go
package tunnel

import (
	"io"
	"net"
	"sync"
)

// Relay copies data bidirectionally between two connections.
// It blocks until both directions are done (one side closes or errors).
func Relay(conn1, conn2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(conn2, conn1)
		conn2.Close()
	}()

	go func() {
		defer wg.Done()
		io.Copy(conn1, conn2)
		conn1.Close()
	}()

	wg.Wait()
}

// RelayPair is like Relay but returns an error for logging.
func RelayPair(conn1, conn2 net.Conn) error {
	var wg sync.WaitGroup
	var err1, err2 error
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, err1 = io.Copy(conn2, conn1)
		conn2.Close()
	}()

	go func() {
		defer wg.Done()
		_, err2 = io.Copy(conn1, conn2)
		conn1.Close()
	}()

	wg.Wait()

	if err1 != nil && err1 != io.EOF {
		return err1
	}
	if err2 != nil && err2 != io.EOF {
		return err2
	}
	return nil
}
