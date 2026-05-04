// gotunnel/pkg/protocol/frame.go
package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Message type constants
const (
	MsgTypeRegister    uint8 = 0x01
	MsgTypeRegisterAck uint8 = 0x02
	MsgTypeNewConn     uint8 = 0x03
	MsgTypeData        uint8 = 0x04
	MsgTypeClose       uint8 = 0x05
	MsgTypeHeartbeat   uint8 = 0x06
	MsgTypeAuth        uint8 = 0x07
	MsgTypeAuthAck     uint8 = 0x08
	MsgTypeError       uint8 = 0xFF
)

// Frame is the basic unit of communication.
// Wire format: [Type:1byte][Length:4bytes][Payload:variable]
type Frame struct {
	Type    uint8
	Payload []byte
}

// headerSize is Type(1) + Length(4) = 5 bytes
const headerSize = 5

// MaxPayloadSize is the maximum allowed frame payload size (1MB).
const MaxPayloadSize = 1 << 20

// ErrFrameTooLarge is returned when a frame payload exceeds MaxPayloadSize.
var ErrFrameTooLarge = fmt.Errorf("frame payload exceeds maximum size of %d bytes", MaxPayloadSize)

// WriteFrame writes a frame to the writer.
func WriteFrame(w io.Writer, f Frame) error {
	header := make([]byte, headerSize)
	header[0] = f.Type
	binary.BigEndian.PutUint32(header[1:5], uint32(len(f.Payload)))

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	if len(f.Payload) > 0 {
		if _, err := w.Write(f.Payload); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}

	return nil
}

// ReadFrame reads a complete frame from the reader.
func ReadFrame(r io.Reader) (Frame, error) {
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return Frame{}, fmt.Errorf("read header: %w", err)
	}

	frameType := header[0]
	length := binary.BigEndian.Uint32(header[1:5])

	if length > MaxPayloadSize {
		return Frame{}, ErrFrameTooLarge
	}

	var payload []byte
	if length > 0 {
		payload = make([]byte, length)
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, fmt.Errorf("read payload: %w", err)
		}
	}

	return Frame{Type: frameType, Payload: payload}, nil
}
