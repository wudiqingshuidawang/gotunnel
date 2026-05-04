// gotunnel/pkg/protocol/message.go
package protocol

import "encoding/json"

// RegisterMsg: client tells server which local port to expose
type RegisterMsg struct {
	LocalPort  int `json:"local_port"`
	RemotePort int `json:"remote_port,omitempty"`
}

// RegisterAckMsg: server tells client which remote port was assigned
type RegisterAckMsg struct {
	RemotePort int `json:"remote_port"`
}

// NewConnMsg: server tells client a new external connection arrived
type NewConnMsg struct {
	ConnID string `json:"conn_id"`
}

// DataMsg: carries data for a specific tunnel session
type DataMsg struct {
	ConnID string `json:"conn_id"`
	Data   []byte `json:"data"`
}

// CloseMsg: signals a tunnel session is closing
type CloseMsg struct {
	ConnID string `json:"conn_id"`
}

// ErrorMsg: server reports an error to the client
type ErrorMsg struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// AuthMsg: client sends token for authentication
type AuthMsg struct {
	Token string `json:"token"`
}

// AuthAckMsg: server confirms authentication success
type AuthAckMsg struct {
	OK bool `json:"ok"`
}

// ToFrame serializes a message payload into a Frame.
func ToFrame(msgType uint8, payload interface{}) (Frame, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Type: msgType, Payload: data}, nil
}

// FromFrame deserializes a Frame's payload into the target struct.
// For heartbeat (nil payload), this is a no-op.
func FromFrame(f Frame, target interface{}) error {
	if len(f.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(f.Payload, target)
}
