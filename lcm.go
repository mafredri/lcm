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
	"log"
	"sync/atomic"
	"time"

	"github.com/pkg/term"
)

//go:generate protoc --proto_path=. --go_out=. --go-grpc_out=. ./stream/stream.proto

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
	// MessageDelay defines how long to wait before issuing the next
	// message, should be considered both when receiving and writing.
	//
	// The ASUSTOR lcmd binary uses 15ms and 45ms sleeps between
	// certain commands.
	MessageDelay = 15 * time.Millisecond
	// ReplyDelay defines how long to wait before sending a reply to
	// a command (e.g. responding to button presses).
	//
	// Note: This seems to be a good delay as it produces very few
	// errors in communications following it. Going lower or higher
	// can increases error frequency.
	ReplyDelay = 1500 * time.Microsecond
	Retries    = 100
)

// DefaultTTY represents the default serial tty for LCM.
const DefaultTTY = "/dev/ttyS1"

type rawRead struct {
	ts  time.Time
	msg Message
}

// LCM represents the ASUSTOR Liquid Crystal Monitor.
type LCM struct {
	s *term.Term
	// s        serial.Port
	writeC   chan sendMessage
	rawReadC chan rawRead
	readC    chan []byte
	lastRead int64
}

// Open opens the serial port for LCM.
func Open(tty string) (*LCM, error) {
	// s, err := serial.Open(tty, &serial.Mode{BaudRate: 115200})
	s, err := term.Open(tty, term.Speed(115200), term.RawMode, term.FlowControl(term.NONE))
	if err != nil {
		return nil, err
	}

	m := &LCM{
		s:        s,
		writeC:   make(chan sendMessage, 10),
		rawReadC: make(chan rawRead, 10),
		readC:    make(chan []byte, 10),
	}

	go m.read()
	go m.handle()

	return m, m.Sync()
}

// func (m *LCM) Flush() error {
// 	return m.s.Flush()
// }

type sendMessage struct {
	err  chan error
	data []byte
}

// Send messages to the display. Note that checksum should be omitted,
// it is handled transparently as part of the protocol implementation.
func (m *LCM) Send(msg Message) error {
	data := make([]byte, len(msg), len(msg)+1)
	copy(data, msg)
	data = append(data, checksum(data))

	sm := sendMessage{err: make(chan error, 1), data: data}
	m.writeC <- sm
	return <-sm.err
}

// Recv messages sent from the display.
func (m *LCM) Recv() Message {
	return <-m.readC
}

// Sync ensures that the serial communication protocol is in sync.
func (m *LCM) Sync() error {
	err := m.Send(DisplayStatus)
	if err != nil {
		return err
	}
	return nil
}

// read reads asynchronously from the serial port
// and transmits messages on the read channel.
func (m *LCM) read() {
	r := bufio.NewReaderSize(m.s, 32)
	raw := &recvMessage{}
	for {
		raw.Reset()
		err := copyBytes(raw, r)
		atomic.StoreInt64(&m.lastRead, time.Now().Unix())
		if err != nil {
			log.Printf("LCM.read: %v", err)
			continue
		}
		b := raw.Bytes()
		log.Printf("LCM.read: OK %#x", b)
		peek, err := r.Peek(5)
		log.Printf("LCM.read: Peek %#x, err: %v", peek, err)
		m.rawReadC <- rawRead{
			ts:  time.Now(),
			msg: Message(b),
		}
	}
}

// write synchronously to the serial port.
func (m *LCM) write(data []byte) error {
	n, err := m.s.Write(data)
	log.Printf("write: Wrote msg: %#x %d, err: %v", data, n, err)
	if err != nil {
		return err
	}
	// todo reply
	return nil
}

// handle incoming and outgoing messages.
//
// TODO(mafredri): Refactor, maybe implement reply and retry mechanism.
func (m *LCM) handle() {
	// Keep track of when messages were last written to the serial
	// port for both reads and writes. The display microcontroller
	// needs time to settle even after it has sent a message.
	var lastSerialMessage time.Time

	for {
		select {
		case raw := <-m.rawReadC:
			// lastSerialMessage = time.Now()
			m.handleRawRead(raw)

		case msg := <-m.writeC:
			// Prioritize depleting read channel.
			select {
			case raw := <-m.rawReadC:
				m.handleRawRead(raw)
			default:
			}

		LastRead:
			lastRead := time.Unix(atomic.LoadInt64(&m.lastRead), 0)
			if lastSerialMessage.Before(lastRead) {
				lastSerialMessage = lastRead
			}
			if diff := time.Since(lastSerialMessage); diff < MessageDelay {
				diff = MessageDelay - diff
				log.Printf("LCM.handle: Sleeping (command delay): %s", diff)
				time.Sleep(diff)
				goto LastRead
			}
			log.Printf("LCM.handle: Write msg: %#x", msg.data)
			msg.err <- m.write(msg.data)
			lastSerialMessage = time.Now()
		}
	}
}

func (m *LCM) handleRawRead(raw rawRead) {
	msg := raw.msg
	switch msg.Type() {
	case Command:
		log.Printf("LCM.handle: Command %#x", msg[2])

		switch msg.Function() {
		case VersionRequest:
			// Do not reply to version request
			// because it will loop infinitely.
			log.Printf("LCM.handleRawRead: Command: MCU Version %d.%d.%d", msg[3], msg[4], msg[5])
		default:
		}
	ReplySleep:
		ts := raw.ts
		lastRead := time.Unix(atomic.LoadInt64(&m.lastRead), 0)
		if ts.Before(lastRead) {
			ts = lastRead
		}
		if diff := time.Since(ts); diff < ReplyDelay {
			diff = ReplyDelay - diff
			log.Printf("LCM.handleRawRead: Sleeping (reply delay): %s", diff)
			time.Sleep(diff)
			goto ReplySleep
		}
		reply := msg.ReplyOk()
		err := m.write(append(reply, checksum(reply)))
		log.Printf("LCM.handleRawRead: Command: Replied %#x, err: %v", reply, err)
		go m.s.Write(DisplayStatus)

	case Reply:
		if msg[3] == 0 {
			log.Printf("LCM.handleRawRead: Reply: OK %#x", msg.Function())
		} else {
			log.Printf("LCM.handleRawRead: Reply: Command %#x failed", msg.Function())
		}
		return

	default:
		log.Printf("LCM.handleRawRead: Unknown message %#x", msg)
	}

	select {
	case m.readC <- msg:
	default:
		log.Printf("LCM.handleRawRead: Buffer full, discarding earliest message...")
		<-m.readC
		m.readC <- msg
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
