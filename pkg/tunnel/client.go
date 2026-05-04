// gotunnel/pkg/tunnel/client.go
package tunnel

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/yan/gotunnel/pkg/protocol"
)

// Client connects to a tunnel server and forwards traffic to a local service.
type Client struct {
	serverAddr   string
	localPort    int
	remotePort   int
	dialTimeout  time.Duration
	conn         net.Conn
	writer       sync.Mutex
	remotePortMu sync.RWMutex
}

// NewClient creates a client that will expose localPort through the server.
func NewClient(serverAddr string, localPort, remotePort int) *Client {
	return &Client{
		serverAddr:  serverAddr,
		localPort:   localPort,
		remotePort:  remotePort,
		dialTimeout: 5 * time.Second,
	}
}

func (c *Client) SetDialTimeout(d time.Duration) {
	c.dialTimeout = d
}

func (c *Client) RemotePort() int {
	c.remotePortMu.RLock()
	defer c.remotePortMu.RUnlock()
	return c.remotePort
}

// Connect establishes the control connection and registers with the server.
func (c *Client) Connect() error {
	conn, err := net.DialTimeout("tcp", c.serverAddr, c.dialTimeout)
	if err != nil {
		return fmt.Errorf("dial server: %w", err)
	}
	c.conn = conn

	regMsg := protocol.RegisterMsg{LocalPort: c.localPort}
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
		c.remotePortMu.Lock()
		c.remotePort = ack.RemotePort
		c.remotePortMu.Unlock()
		slog.Info("registered", "remote_port", ack.RemotePort)
	case protocol.MsgTypeError:
		var errMsg protocol.ErrorMsg
		protocol.FromFrame(resp, &errMsg)
		conn.Close()
		return fmt.Errorf("server error: %s", errMsg.Msg)
	default:
		conn.Close()
		return fmt.Errorf("unexpected response type: %d", resp.Type)
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
			wg.Add(1)
			go func() {
				defer wg.Done()
				c.handleNewConn(msg.ConnID)
			}()
		case protocol.MsgTypeData:
			var msg protocol.DataMsg
			protocol.FromFrame(frame, &msg)
			slog.Debug("data from server", "connID", msg.ConnID, "len", len(msg.Data))
		case protocol.MsgTypeClose:
			var msg protocol.CloseMsg
			protocol.FromFrame(frame, &msg)
			slog.Debug("close from server", "connID", msg.ConnID)
		case protocol.MsgTypeHeartbeat:
			// heartbeat ack
		default:
			slog.Warn("unknown message", "type", frame.Type)
		}
	}
}

func (c *Client) handleNewConn(connID string) {
	slog.Info("new tunnel connection", "connID", connID)

	localAddr := fmt.Sprintf("127.0.0.1:%d", c.localPort)
	localConn, err := net.DialTimeout("tcp", localAddr, 3*time.Second)
	if err != nil {
		slog.Error("connect to local service", "err", err, "addr", localAddr)
		c.writeFrame(protocol.Frame{
			Type:    protocol.MsgTypeClose,
			Payload: mustMarshal(protocol.CloseMsg{ConnID: connID}),
		})
		return
	}

	go c.relayLocalToServer(localConn, connID)
}

func (c *Client) relayLocalToServer(localConn net.Conn, connID string) {
	defer localConn.Close()

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
