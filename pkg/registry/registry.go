package registry

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
)

var (
	ErrPortInUse    = errors.New("port already in use")
	ErrPortOccupied = errors.New("port occupied by another process")
	ErrRangeFull    = errors.New("no available ports in range")
)

// Registry manages port allocation for tunnel servers.
type Registry struct {
	mu        sync.Mutex
	minPort   int
	maxPort   int
	allocated map[int]bool
}

// New creates a Registry with the given port range [minPort, maxPort].
func New(minPort, maxPort int) *Registry {
	return &Registry{
		minPort:   minPort,
		maxPort:   maxPort,
		allocated: make(map[int]bool),
	}
}

// Allocate reserves a port. If specificPort > 0, tries to allocate that port.
// If specificPort == 0, picks a random available port from the range.
func (r *Registry) Allocate(specificPort int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if specificPort > 0 {
		return r.allocateSpecific(specificPort)
	}
	return r.allocateRandom()
}

// Release frees a previously allocated port.
func (r *Registry) Release(port int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.allocated, port)
}

func (r *Registry) allocateSpecific(port int) (int, error) {
	if port < r.minPort || port > r.maxPort {
		return 0, fmt.Errorf("port %d out of range [%d, %d]", port, r.minPort, r.maxPort)
	}
	if r.allocated[port] {
		return 0, ErrPortInUse
	}
	if !isPortAvailable(port) {
		return 0, ErrPortOccupied
	}
	r.allocated[port] = true
	return port, nil
}

func (r *Registry) allocateRandom() (int, error) {
	total := r.maxPort - r.minPort + 1
	available := make([]int, 0, total)
	for p := r.minPort; p <= r.maxPort; p++ {
		if !r.allocated[p] {
			available = append(available, p)
		}
	}

	if len(available) == 0 {
		return 0, ErrRangeFull
	}

	rand.Shuffle(len(available), func(i, j int) {
		available[i], available[j] = available[j], available[i]
	})

	for _, port := range available {
		if isPortAvailable(port) {
			r.allocated[port] = true
			return port, nil
		}
	}

	return 0, ErrRangeFull
}

// isPortAvailable checks if a port can be bound on the system.
func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
