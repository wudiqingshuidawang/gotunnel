// gotunnel/pkg/tunnel/server.go
package tunnel

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/yan/gotunnel/pkg/protocol"
	"github.com/yan/gotunnel/pkg/registry"
)

// Server manages tunnel clients and public-facing listeners.
type Server struct {
	controlAddr string
	listener    net.Listener
	registry    *registry.Registry

	mu       sync.RWMutex
	clients  map[string]*clientState
	tunnels  map[int]*tunnelState
	sessions map[string]*sessionState // connID -> session
	stopCh   chan struct{}
	readyCh  chan struct{} // closed when listener is ready
}

type clientState struct {
	id     string
	conn   net.Conn
	writer *sync.Mutex
}

type tunnelState struct {
	clientID   string
	remotePort int
	listener   net.Listener
}

type sessionState struct {
	connID     string
	publicConn net.Conn
	writer     *sync.Mutex
}

func NewServer(controlAddr string) *Server {
	return &Server{
		controlAddr: controlAddr,
		registry:    registry.New(8000, 9000),
		clients:     make(map[string]*clientState),
		tunnels:     make(map[int]*tunnelState),
		sessions:    make(map[string]*sessionState),
		stopCh:      make(chan struct{}),
		readyCh:     make(chan struct{}),
	}
}

func NewServerWithRegistry(controlAddr string, reg *registry.Registry) *Server {
	if reg == nil {
		reg = registry.New(8000, 9000)
	}
	return &Server{
		controlAddr: controlAddr,
		registry:    reg,
		clients:     make(map[string]*clientState),
		tunnels:     make(map[int]*tunnelState),
		sessions:    make(map[string]*sessionState),
		stopCh:      make(chan struct{}),
		readyCh:     make(chan struct{}),
	}
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.controlAddr)
	if err != nil {
		close(s.readyCh)
		return fmt.Errorf("listen %s: %w", s.controlAddr, err)
	}
	s.listener = ln
	close(s.readyCh)
	slog.Info("server started", "addr", ln.Addr().String())

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return nil
			default:
				slog.Error("accept error", "err", err)
				continue
			}
		}
		go s.handleClient(conn)
	}
}

// Addr returns the address the server is listening on, or empty string if not started.
// It blocks until the server has started or failed to start.
func (s *Server) Addr() string {
	<-s.readyCh
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) Stop() {
	// Wait for Start to finish initializing before accessing listener.
	<-s.readyCh
	close(s.stopCh)
	if s.listener != nil {
		s.listener.Close()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for port, ts := range s.tunnels {
		ts.listener.Close()
		s.registry.Release(port)
	}
	s.tunnels = make(map[int]*tunnelState)

	for _, cs := range s.clients {
		cs.conn.Close()
	}
	s.clients = make(map[string]*clientState)

	for _, sess := range s.sessions {
		sess.publicConn.Close()
	}
	s.sessions = make(map[string]*sessionState)
}

func (s *Server) handleClient(conn net.Conn) {
	clientID := newUUID()
	writer := &sync.Mutex{}

	cs := &clientState{
		id:     clientID,
		conn:   conn,
		writer: writer,
	}

	s.mu.Lock()
	s.clients[clientID] = cs
	s.mu.Unlock()

	slog.Info("client connected", "id", clientID, "addr", conn.RemoteAddr())
	defer func() {
		s.removeClient(clientID)
		conn.Close()
		slog.Info("client disconnected", "id", clientID)
	}()

	for {
		frame, err := protocol.ReadFrame(conn)
		if err != nil {
			slog.Debug("read error", "client", clientID, "err", err)
			return
		}

		switch frame.Type {
		case protocol.MsgTypeRegister:
			s.handleRegister(cs, frame)
		case protocol.MsgTypeData:
			s.handleDataFromClient(frame)
		case protocol.MsgTypeClose:
			s.handleCloseFromClient(frame)
		case protocol.MsgTypeHeartbeat:
			s.writeFrame(cs, protocol.Frame{Type: protocol.MsgTypeHeartbeat})
		default:
			slog.Warn("unknown message type", "type", frame.Type, "client", clientID)
		}
	}
}

func (s *Server) handleRegister(cs *clientState, frame protocol.Frame) {
	var msg protocol.RegisterMsg
	if err := protocol.FromFrame(frame, &msg); err != nil {
		slog.Error("parse register", "err", err)
		return
	}

	port, err := s.registry.Allocate(msg.RemotePort)
	if err != nil {
		slog.Error("allocate port", "err", err)
		s.writeFrame(cs, protocol.Frame{
			Type: protocol.MsgTypeError,
			Payload: mustMarshal(protocol.ErrorMsg{
				Code: 1, Msg: "no available ports",
			}),
		})
		return
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		s.registry.Release(port)
		slog.Error("listen public port", "port", port, "err", err)
		s.writeFrame(cs, protocol.Frame{
			Type: protocol.MsgTypeError,
			Payload: mustMarshal(protocol.ErrorMsg{
				Code: 2, Msg: fmt.Sprintf("cannot listen on port %d", port),
			}),
		})
		return
	}

	ts := &tunnelState{
		clientID:   cs.id,
		remotePort: port,
		listener:   ln,
	}

	s.mu.Lock()
	s.tunnels[port] = ts
	s.mu.Unlock()

	s.writeFrame(cs, protocol.Frame{
		Type:    protocol.MsgTypeRegisterAck,
		Payload: mustMarshal(protocol.RegisterAckMsg{RemotePort: port}),
	})

	slog.Info("tunnel created", "client", cs.id, "port", port)

	go s.acceptPublicConnections(ts, cs)
}

func (s *Server) acceptPublicConnections(ts *tunnelState, cs *clientState) {
	for {
		publicConn, err := ts.listener.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
				slog.Debug("public accept error", "port", ts.remotePort, "err", err)
				return
			}
		}

		connID := newUUID()
		slog.Info("new public connection", "connID", connID, "port", ts.remotePort)

		sess := &sessionState{
			connID:     connID,
			publicConn: publicConn,
			writer:     &sync.Mutex{},
		}

		s.mu.Lock()
		s.sessions[connID] = sess
		s.mu.Unlock()

		s.writeFrame(cs, protocol.Frame{
			Type:    protocol.MsgTypeNewConn,
			Payload: mustMarshal(protocol.NewConnMsg{ConnID: connID}),
		})

		go s.relayPublicToClient(publicConn, cs, connID)
	}
}

func (s *Server) relayPublicToClient(publicConn net.Conn, cs *clientState, connID string) {
	buf := make([]byte, 32*1024)
	for {
		n, err := publicConn.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			s.writeFrame(cs, protocol.Frame{
				Type: protocol.MsgTypeData,
				Payload: mustMarshal(protocol.DataMsg{
					ConnID: connID,
					Data:   data,
				}),
			})
		}
		if err != nil {
			// Tell the client that the public side is done sending.
			// The session is cleaned up when the client responds with
			// its own CloseMsg (handled by handleCloseFromClient).
			s.writeFrame(cs, protocol.Frame{
				Type:    protocol.MsgTypeClose,
				Payload: mustMarshal(protocol.CloseMsg{ConnID: connID}),
			})
			return
		}
	}
}

func (s *Server) handleDataFromClient(frame protocol.Frame) {
	var msg protocol.DataMsg
	if err := protocol.FromFrame(frame, &msg); err != nil {
		slog.Error("parse data from client", "err", err)
		return
	}

	s.mu.RLock()
	sess, ok := s.sessions[msg.ConnID]
	s.mu.RUnlock()

	if !ok {
		slog.Warn("data for unknown session", "connID", msg.ConnID)
		return
	}

	sess.writer.Lock()
	defer sess.writer.Unlock()
	sess.publicConn.Write(msg.Data)
}

func (s *Server) handleCloseFromClient(frame protocol.Frame) {
	var msg protocol.CloseMsg
	if err := protocol.FromFrame(frame, &msg); err != nil {
		return
	}

	s.mu.Lock()
	sess, ok := s.sessions[msg.ConnID]
	if ok {
		sess.publicConn.Close()
		delete(s.sessions, msg.ConnID)
	}
	s.mu.Unlock()

	if ok {
		slog.Debug("session closed by client", "connID", msg.ConnID)
	}
}

func (s *Server) removeClient(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for port, ts := range s.tunnels {
		if ts.clientID == clientID {
			ts.listener.Close()
			s.registry.Release(port)
			delete(s.tunnels, port)
			slog.Info("tunnel closed", "port", port, "client", clientID)
		}
	}

	delete(s.clients, clientID)
}

func (s *Server) writeFrame(cs *clientState, frame protocol.Frame) {
	cs.writer.Lock()
	defer cs.writer.Unlock()
	protocol.WriteFrame(cs.conn, frame)
}

// mustMarshal serializes a message payload into raw bytes.
// This is a shared helper reused by both server and client.
func mustMarshal(v interface{}) []byte {
	frame, _ := protocol.ToFrame(0, v)
	return frame.Payload
}

// newUUID generates a UUID v4 string using crypto/rand.
func newUUID() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant is 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
