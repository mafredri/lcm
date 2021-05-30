package lcm

import (
	"bytes"
	"fmt"
	"io"
)

type recvMessage struct {
	buf      bytes.Buffer
	len, sum uint8
}

var _ io.ByteWriter = (*recvMessage)(nil)

func (m *recvMessage) WriteByte(c byte) error {
	n := uint8(m.buf.Len())

	err := m.buf.WriteByte(c)
	if err != nil {
		return err
	}

	switch {
	// Message type.
	case n == 0:
		if c != CommandByte && c != ReplyByte {
			return fmt.Errorf("invalid frame: %#x", c)
		}

	// Payload size.
	case n == 1:
		m.len = 3 + c // Header and payload.

	// End of message (checksum).
	case n > 1 && n == m.len:
		if m.sum == c {
			return io.EOF // Success.
		}
		return fmt.Errorf("invalid checksum: %#x", m.buf.Bytes())

	// Impossible state.
	case n > 1 && n > m.len:
		return fmt.Errorf("invalid size: %#x", m.buf.Bytes())
	}

	m.sum += c
	return nil
}

func (m *recvMessage) Bytes() []byte {
	l := m.buf.Len()
	b := make([]byte, l)
	copy(b, m.buf.Bytes())
	return b
}

func (m *recvMessage) Reset() {
	m.buf.Reset()
	m.sum = 0
	m.len = 0
}
