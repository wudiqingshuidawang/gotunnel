// gotunnel/pkg/tunnel/client.go
package tunnel

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yan/gotunnel/pkg/protocol"
)

// tunnelRoute maps a remote port (assigned by server) to a local port.
type tunnelRoute struct {
	localPort  int
	remotePort int // filled after REGISTER_ACK
}

// Client connects to a tunnel server and forwards traffic to local services.
type Client struct {
	serverAddr  string
	dialTimeout time.Duration
	token       string      // optional auth token
	tlsConfig   *tls.Config // nil = no TLS
	httpMode    bool        // inject X-Forwarded-For headers
	conn        net.Conn
	writer      sync.Mutex

	tunnels  []*tunnelRoute         // ordered list of tunnels
	routes   map[int]*tunnelRoute   // remotePort -> route (populated after registration)
	routeMu  sync.RWMutex

	localConns   map[string]net.Conn
	localConnsMu sync.Mutex

	firstData   map[string]bool // connID -> whether first data has been processed
	firstDataMu sync.Mutex
}

// NewClient creates a client that will connect to the given server.
// Use AddTunnel to register one or more local ports before calling Connect.
func NewClient(serverAddr string) *Client {
	return &Client{
		serverAddr:  serverAddr,
		dialTimeout: 5 * time.Second,
		routes:      make(map[int]*tunnelRoute),
		localConns:  make(map[string]net.Conn),
		firstData:   make(map[string]bool),
	}
}

// AddTunnel registers a tunnel: localPort is the local service to expose,
// remotePort is the requested server port (0 = auto-assign).
func (c *Client) AddTunnel(localPort, remotePort int) {
	c.tunnels = append(c.tunnels, &tunnelRoute{localPort: localPort, remotePort: remotePort})
}

func (c *Client) SetDialTimeout(d time.Duration) {
	c.dialTimeout = d
}

// SetToken configures an authentication token to send during handshake.
func (c *Client) SetToken(token string) {
	c.token = token
}

// SetTLSConfig configures TLS for the control channel. If nil, TLS is disabled.
func (c *Client) SetTLSConfig(cfg *tls.Config) {
	c.tlsConfig = cfg
}

// SetHTTPMode enables HTTP header injection (X-Forwarded-For, X-Real-IP).
func (c *Client) SetHTTPMode(enabled bool) {
	c.httpMode = enabled
}

// RemotePort returns the first assigned remote port (for single-tunnel compatibility).
func (c *Client) RemotePort() int {
	c.routeMu.RLock()
	defer c.routeMu.RUnlock()
	for _, r := range c.routes {
		return r.remotePort
	}
	return 0
}

// RemotePorts returns all assigned remote ports.
func (c *Client) RemotePorts() []int {
	c.routeMu.RLock()
	defer c.routeMu.RUnlock()
	ports := make([]int, 0, len(c.routes))
	for _, r := range c.routes {
		ports = append(ports, r.remotePort)
	}
	return ports
}

// Connect establishes the control connection and registers with the server.
func (c *Client) Connect() error {
	var conn net.Conn
	var err error
	dialer := &net.Dialer{Timeout: c.dialTimeout}
	if c.tlsConfig != nil {
		conn, err = tls.DialWithDialer(dialer, "tcp", c.serverAddr, c.tlsConfig)
	} else {
		conn, err = dialer.Dial("tcp", c.serverAddr)
	}
	if err != nil {
		return fmt.Errorf("dial server: %w", err)
	}
	c.conn = conn

	// Send AUTH if token is configured
	if c.token != "" {
		authFrame, _ := protocol.ToFrame(protocol.MsgTypeAuth, protocol.AuthMsg{Token: c.token})
		if err := c.writeFrame(authFrame); err != nil {
			conn.Close()
			return fmt.Errorf("send auth: %w", err)
		}
		resp, err := protocol.ReadFrame(conn)
		if err != nil {
			conn.Close()
			return fmt.Errorf("read auth response: %w", err)
		}
		if resp.Type == protocol.MsgTypeError {
			var errMsg protocol.ErrorMsg
			protocol.FromFrame(resp, &errMsg)
			conn.Close()
			return fmt.Errorf("auth failed: %s", errMsg.Msg)
		}
		if resp.Type != protocol.MsgTypeAuthAck {
			conn.Close()
			return fmt.Errorf("unexpected auth response type: %d", resp.Type)
		}
	}

	// Register all tunnels
	for _, route := range c.tunnels {
		regMsg := protocol.RegisterMsg{LocalPort: route.localPort, RemotePort: route.remotePort}
		frame, _ := protocol.ToFrame(protocol.MsgTypeRegister, regMsg)
		if err := c.writeFrame(frame); err != nil {
			conn.Close()
			return fmt.Errorf("send register: %w", err)
		}

		resp, err := protocol.ReadFrame(conn)
		if err != nil {
			conn.Close()
			return fmt.Errorf("read register ack: %w", err)
		}

		switch resp.Type {
		case protocol.MsgTypeRegisterAck:
			var ack protocol.RegisterAckMsg
			protocol.FromFrame(resp, &ack)
			c.routeMu.Lock()
			route.remotePort = ack.RemotePort
			c.routes[ack.RemotePort] = route
			c.routeMu.Unlock()
			slog.Info("tunnel registered", "local", route.localPort, "remote", ack.RemotePort)
		case protocol.MsgTypeError:
			var errMsg protocol.ErrorMsg
			protocol.FromFrame(resp, &errMsg)
			conn.Close()
			return fmt.Errorf("server error: %s", errMsg.Msg)
		default:
			conn.Close()
			return fmt.Errorf("unexpected response type: %d", resp.Type)
		}
	}

	return nil
}

// Run starts the client event loop. Blocks until connection closes.
func (c *Client) Run() {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.heartbeatLoop()
	}()

	for {
		frame, err := protocol.ReadFrame(c.conn)
		if err != nil {
			slog.Info("disconnected from server", "err", err)
			return
		}

		switch frame.Type {
		case protocol.MsgTypeNewConn:
			var msg protocol.NewConnMsg
			protocol.FromFrame(frame, &msg)
			// Call synchronously so localConn is registered in the map
			// before subsequent DataMsg frames arrive. The relay goroutine
			// (started inside handleNewConn) handles ongoing data transfer.
			c.handleNewConn(msg.ConnID, msg.RemotePort)
		case protocol.MsgTypeData:
			var msg protocol.DataMsg
			protocol.FromFrame(frame, &msg)
			slog.Debug("data from server", "connID", msg.ConnID, "len", len(msg.Data))
			c.forwardToLocal(msg.ConnID, msg.Data)
		case protocol.MsgTypeClose:
			var msg protocol.CloseMsg
			protocol.FromFrame(frame, &msg)
			slog.Debug("close from server", "connID", msg.ConnID)
			c.closeLocalConn(msg.ConnID)
		case protocol.MsgTypeHeartbeat:
			// heartbeat ack
		default:
			slog.Warn("unknown message", "type", frame.Type)
		}
	}
}

func (c *Client) handleNewConn(connID string, remotePort int) {
	c.routeMu.RLock()
	route, ok := c.routes[remotePort]
	c.routeMu.RUnlock()

	if !ok {
		// Fallback for old servers that don't send RemotePort: use first route
		c.routeMu.RLock()
		for _, r := range c.routes {
			route = r
			ok = true
			break
		}
		c.routeMu.RUnlock()
	}

	if !ok || route == nil {
		slog.Warn("no route for connection", "connID", connID, "remotePort", remotePort)
		return
	}

	slog.Info("new tunnel connection", "connID", connID, "remotePort", remotePort, "localPort", route.localPort)

	localAddr := fmt.Sprintf("127.0.0.1:%d", route.localPort)
	localConn, err := net.DialTimeout("tcp", localAddr, 3*time.Second)
	if err != nil {
		slog.Error("connect to local service", "err", err, "addr", localAddr)
		c.writeFrame(protocol.Frame{
			Type:    protocol.MsgTypeClose,
			Payload: mustMarshal(protocol.CloseMsg{ConnID: connID}),
		})
		return
	}

	c.localConnsMu.Lock()
	c.localConns[connID] = localConn
	c.localConnsMu.Unlock()

	go c.relayLocalToServer(localConn, connID)
}

func (c *Client) forwardToLocal(connID string, data []byte) {
	c.localConnsMu.Lock()
	localConn, ok := c.localConns[connID]
	c.localConnsMu.Unlock()

	if !ok {
		slog.Warn("no local conn for data", "connID", connID)
		return
	}

	// HTTP mode: inject headers on first data
	if c.httpMode {
		c.firstDataMu.Lock()
		processed := c.firstData[connID]
		c.firstDataMu.Unlock()

		if !processed {
			data = c.injectHTTPHeaders(connID, data)
			c.firstDataMu.Lock()
			c.firstData[connID] = true
			c.firstDataMu.Unlock()
		}
	}

	if _, err := localConn.Write(data); err != nil {
		slog.Error("write to local", "err", err, "connID", connID)
	}
}

func (c *Client) closeLocalConn(connID string) {
	// Clean up firstData tracking
	c.firstDataMu.Lock()
	delete(c.firstData, connID)
	c.firstDataMu.Unlock()

	c.localConnsMu.Lock()
	localConn, ok := c.localConns[connID]
	c.localConnsMu.Unlock()

	if !ok {
		return
	}

	// Half-close: shut down the write side so the local service sees EOF
	// but can still send back any remaining data. The relayLocalToServer
	// goroutine will continue reading until the local service finishes.
	if tc, ok := localConn.(*net.TCPConn); ok {
		tc.CloseWrite()
	} else {
		localConn.Close()
	}
}

func (c *Client) relayLocalToServer(localConn net.Conn, connID string) {
	defer func() {
		localConn.Close()
		c.localConnsMu.Lock()
		delete(c.localConns, connID)
		c.localConnsMu.Unlock()
	}()

	buf := make([]byte, 32*1024)
	for {
		n, err := localConn.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			c.writeFrame(protocol.Frame{
				Type: protocol.MsgTypeData,
				Payload: mustMarshal(protocol.DataMsg{
					ConnID: connID,
					Data:   data,
				}),
			})
		}
		if err != nil {
			c.writeFrame(protocol.Frame{
				Type:    protocol.MsgTypeClose,
				Payload: mustMarshal(protocol.CloseMsg{ConnID: connID}),
			})
			return
		}
	}
}

func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if err := c.writeFrame(protocol.Frame{Type: protocol.MsgTypeHeartbeat}); err != nil {
			slog.Debug("heartbeat failed", "err", err)
			return
		}
	}
}

func (c *Client) writeFrame(frame protocol.Frame) error {
	c.writer.Lock()
	defer c.writer.Unlock()
	return protocol.WriteFrame(c.conn, frame)
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// injectHTTPHeaders checks if data looks like an HTTP request and adds
// X-Forwarded-For and X-Real-IP headers. Returns data unchanged if not HTTP.
func (c *Client) injectHTTPHeaders(connID string, data []byte) []byte {
	// Quick check: does it look like an HTTP request?
	if !isHTTPRequest(data) {
		return data
	}

	// Parse the HTTP request
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		return data // not parseable, pass through
	}
	defer req.Body.Close()

	// Get the remote address from the connection
	c.localConnsMu.Lock()
	localConn, ok := c.localConns[connID]
	c.localConnsMu.Unlock()
	if !ok {
		return data
	}

	remoteAddr := localConn.RemoteAddr().String()
	// Extract IP from "ip:port"
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}

	// Inject headers if not already present
	if req.Header.Get("X-Forwarded-For") == "" {
		req.Header.Set("X-Forwarded-For", remoteAddr)
	}
	if req.Header.Get("X-Real-IP") == "" {
		req.Header.Set("X-Real-IP", remoteAddr)
	}

	// Reconstruct the request
	var buf bytes.Buffer
	req.Write(&buf)
	return buf.Bytes()
}

// isHTTPRequest does a quick check if data looks like an HTTP request.
func isHTTPRequest(data []byte) bool {
	methods := []string{"GET ", "POST ", "PUT ", "DELETE ", "HEAD ", "OPTIONS ", "PATCH "}
	s := string(data)
	for _, m := range methods {
		if strings.HasPrefix(s, m) {
			return true
		}
	}
	return false
}
