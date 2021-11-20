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

type parsingError struct {
	m        string
	checksum bool
}

func (e parsingError) Error() string {
	return e.m
}

func (m *recvMessage) WriteByte(c byte) error {
	n := uint8(m.buf.Len())

	err := m.buf.WriteByte(c)
	if err != nil {
		return err
	}

	switch {
	// Message type.
	case n == 0:
		if c := Type(c); c != Command && c != Reply {
			return parsingError{m: fmt.Sprintf("invalid frame: %#x", c)}
		}

	// Payload size.
	case n == 1:
		// Safeguard against parsing very long messages due to
		// corrupted byte sequence.
		if Type(m.buf.Bytes()[0]) == Reply && c > 1 {
			return parsingError{m: fmt.Sprintf("reply message too long %d, should be 1", c)}
		} else if c > 16 {
			// Although, the longest known message sent by
			// the screen is of length 3, we could be more
			// strict here.
			return parsingError{m: fmt.Sprintf("command message too long %d, should be <= 16", c)}
		}
		m.len = 3 + c // Header and payload.

	// End of message (checksum).
	case n > 1 && n == m.len:
		if m.sum == c {
			return io.EOF // Success.
		}
		return parsingError{m: fmt.Sprintf("invalid checksum: %#x", m.buf.Bytes()), checksum: true}

	// Impossible state.
	case n > 1 && n > m.len:
		return parsingError{m: fmt.Sprintf("invalid size: %#x", m.buf.Bytes())}
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

func copyBytes(dst io.ByteWriter, src io.ByteReader) error {
	for {
		c, err := src.ReadByte()
		if err != nil {
			return err
		}
		err = dst.WriteByte(c)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
