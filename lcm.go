/*
Package lcm implements the serial communication protocol for the ASUSTOR
LCD display. This includes controlling and updating and listening for
button presses.

LCM data format:

	MESSAGE_TYPE DATA_LENGTH COMMAND [[DATA]...] [CRC]
*/
package lcm

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"time"

	"github.com/pkg/term"
)

//go:generate protoc --proto_path=. --go_out=. --go-grpc_out=. ./stream/stream.proto

// LCM message types.
const (
	CommandByte = 0xF0
	ReplyByte   = 0xF1
)

const (
	// ReplyTimeout defines how long we wait for a reply, usually
	// one is received within 10ms. The ASUSTOR daemon seems to
	// resend messages after 100ms if no response is received.
	ReplyTimeout = 100 * time.Millisecond
	// RetryInterval specifies how long to wait between retries or
	// last (incorrect) response. The ASUSTOR daemon seems to wait
	// 100ms after the last received communication. If the initial
	// response delay is 50ms, then there will be a total of 150ms
	// until the next resend.
	RetryInterval = 100 * time.Millisecond
)

// DefaultTTY represents the default serial tty for LCM.
const DefaultTTY = "/dev/ttyS1"

// LCM represents the ASUSTOR Liquid Crystal Monitor.
type LCM struct {
	s        *term.Term
	writeC   chan *sendMessage
	rawReadC chan []byte
	readC    chan []byte
}

// Open opens the serial port for LCM.
func Open(tty string) (*LCM, error) {
	s, err := term.Open(tty, term.Speed(115200), term.RawMode)
	if err != nil {
		return nil, err
	}

	m := &LCM{
		s:        s,
		writeC:   make(chan *sendMessage, 2),
		rawReadC: make(chan []byte, 2),
		readC:    make(chan []byte, 2),
	}

	go m.read()
	go m.handle()

	return m, m.Sync()
}

func (m *LCM) Flush() error {
	return m.s.Flush()
}

type sendMessage struct {
	err  chan error
	data []byte
}

// Write messages to the display. Note that checksum is omitted,
// this is handles by LCM.
func (m *LCM) Write(msg []byte) error {
	data := make([]byte, len(msg), len(msg)+1)
	copy(data, msg)
	data = append(data, checksum(data))

	sm := &sendMessage{err: make(chan error, 1), data: data}
	m.writeC <- sm
	return <-sm.err
}

// Read messages sent from the display.
func (m *LCM) Read() (msg []byte) {
	return <-m.readC
}

// Sync ensures that the serial communication protocol is in sync.
func (m *LCM) Sync() error {
	// The ASUSTOR daemon seems to issue this message to sync with
	// the display, but this assumption could be wrong.
	// TODO(mafredri): Verify assumption?
	err := m.Write(DisplayStatus)
	if err != nil {
		return err
	}
	return nil
}

// read reads asynchronously from the serial port
// and transmits messages on the read channel.
func (m *LCM) read() {
	r := bufio.NewReader(m.s)
	raw := &recvMessage{}
	for {
		raw.Reset()
		err := copyBytes(raw, r)
		if err != nil {
			log.Printf("LCM.read: %v", err)
			continue
		}
		b := raw.Bytes()
		log.Printf("LCM.read: OK %#x", b)
		m.rawReadC <- b
	}
}

// write synchronously to the serial port.
func (m *LCM) write(data []byte) error {
	_, err := io.Copy(m.s, bytes.NewReader(data))
	log.Printf("write: Wrote msg: %#x, err: %v", data, err)
	if err != nil {
		return err
	}
	return nil
}

// handle incoming and outgoing messages.
//
// TODO(mafredri): Refactor, maybe implement reply and retry mechanism.
func (m *LCM) handle() {
	for {
		select {
		case msg := <-m.rawReadC:
			msg = msg[:len(msg)-1] // Strip checksum.

			switch msg[0] {
			case CommandByte:
				log.Printf("LCM.handle: Command %#x", msg[2])

				// Craft acknowledgement.
				// reply := make([]byte, 0, 5)
				// reply = append(reply, ReplyByte, 0x01, b[3], 0x00)
				// reply = append(reply, checksum(reply))
				// err := m.write(reply)
				// log.Printf("LCM.handle: Replied %#x, err: %v", reply, err)

			case ReplyByte:
				log.Printf("LCM.handle: Reply %#x", msg[2])

			default:
				log.Printf("LCM.handle: Unknown message %#x", msg)
			}

			select {
			case m.readC <- msg:
			default:
				log.Printf("LCM.handle: Buffer full, discarding earliest message...")
				<-m.readC
				m.readC <- msg
			}

		case msg := <-m.writeC:
			log.Printf("LCM.handle: Write msg: %#x", msg.data)
			msg.err <- m.write(msg.data)
		}
	}
}

// Close the serial connection.
func (m *LCM) Close() error {
	return m.s.Close()
}

func checksum(b []byte) (s byte) {
	for _, bb := range b {
		s += bb
	}
	return s
}
