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
	"errors"
	"fmt"
	"log"
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
	// a command (e.g. responding to button presses). Basis for this
	// value is that it reduces the number of corrupt messages
	// following it.
	ReplyDelay = 2000 * time.Microsecond
	// DefaultRetryLimit defines how many times a command will be
	// retried until giving up.
	DefaultRetryLimit = 100
)

// DefaultTTY represents the default serial tty for LCM.
const DefaultTTY = "/dev/ttyS1"

// LCM represents the ASUSTOR Liquid Crystal Monitor.
type LCM struct {
	s        *term.Term
	writeC   chan sendMessage
	rawReadC chan Message
	readC    chan []byte
}

// Open opens the serial port for LCM.
func Open(tty string) (*LCM, error) {
	s, err := term.Open(tty, term.Speed(115200), term.RawMode)
	if err != nil {
		return nil, err
	}

	err = s.Flush()
	if err != nil {
		s.Close()
		return nil, err
	}

	m := &LCM{
		s:        s,
		writeC:   make(chan sendMessage, 1),
		rawReadC: make(chan Message, 1),
		readC:    make(chan []byte, 4),
	}

	go m.read()
	go m.handle()

	return m, nil
}

type sendMessage struct {
	err        chan error
	data       Message
	retryLimit int
}

// Send messages to the display. Note that checksum should be omitted,
// it is handled transparently as part of the protocol implementation.
func (m *LCM) Send(msg Message) error {
	data := make([]byte, len(msg), len(msg)+1)
	copy(data, msg)
	data = append(data, checksum(data))

	sm := sendMessage{
		err:        make(chan error, 1),
		data:       data,
		retryLimit: DefaultRetryLimit,
	}
	m.writeC <- sm
	return <-sm.err
}

// Recv messages sent from the display.
func (m *LCM) Recv() Message {
	return <-m.readC
}

// read reads asynchronously from the serial port
// and transmits messages on the read channel.
func (m *LCM) read() {
	var parseErr parsingError
	// No need for a large buffer, the most common message length is 5.
	r := bufio.NewReaderSize(m.s, 16)
	raw := &recvMessage{}
	for {
		raw.Reset()
		err := copyBytes(raw, r)
		if err != nil {
			if errors.As(err, &parseErr) {
				log.Printf("LCM.read: %v", err)

				// The trailing message could be valid, try to recover.
				b := raw.Bytes()
				last := b[len(b)-1]
				if last == byte(Command) || last == byte(Reply) {
					err = r.UnreadByte()
					log.Printf("LCM.read: Trying to recover possible message (%#x), err: %v", last, err)
				}
				continue
			}
			log.Printf("LCM.read: fatal: %v", err)
			return
		}
		b := raw.Bytes()
		log.Printf("LCM.read: OK %#x", b)
		m.rawReadC <- b
	}
}

// write synchronously to the serial port.
func (m *LCM) write(data []byte) error {
	n, err := m.s.Write(data)
	log.Printf("write: Wrote msg: %#x %d, err: %v", data, n, err)
	if err != nil {
		return err
	}
	return nil
}

// handle incoming and outgoing messages.
//
// TODO(mafredri): Refactor, maybe implement reply and retry mechanism.
func (m *LCM) handle() {
	var id int64
	var retry func()
	var replyTimeout <-chan time.Time
	noopHandleReply := func(Message) bool { return false }
	handleReply := noopHandleReply

	for {
		var read Message

		// Prioritize processing all messages from the LCM before
		// sending commands or retrying timed out ones.
		if len(m.rawReadC) > 0 {
			read = <-m.rawReadC
		} else {
			select {
			case read = <-m.rawReadC:

			case <-replyTimeout:
				log.Printf("LCM.handle: write(%d): timeout, retry...", id)
				retry()

			case w := <-m.writeC:
				id++
				log.Printf("LCM.handle: write(%d): %#x", id, w.data)

				// Define reply function for verifying
				// that the command was successful.
				handleReply = func(reply Message) bool {
					if reply.Action() == Reply && reply.Function() == w.data.Function() {
						time.Sleep(ReplyDelay)

						if reply[3] == 0 {
							log.Printf("LCM.handle: write(%d): reply OK", id)
							close(w.err)
							handleReply = noopHandleReply
							retry = nil
							replyTimeout = nil
						} else {
							log.Printf("LCM.handle: write(%d): reply FAIL (%#x), retrying...", id, reply[3])
							// Give the MCU a chance to catch up instead
							// of bombarding it with retries.
							time.Sleep(2 * MessageDelay)
							retry()
						}

						return true
					}

					return false
				}

				tries := 0
				var wErr error
				retry = func() {
					if tries > w.retryLimit {
						w.err <- fmt.Errorf("retry limit exceeded: %d/%d: last write error: %v", tries-1, w.retryLimit, wErr)
						handleReply = noopHandleReply
						retry = nil
						replyTimeout = nil

						return
					}
					tries++
					err := m.write(w.data)
					if err != nil {
						log.Printf("LCM.handle: write(%d): %#x: %v", id, w.data, err)
						wErr = err
					}

					replyTimeout = time.After(ReplyTimeout)
					time.Sleep(MessageDelay)
				}

				retry() // Initiate first try.
			}
		}

		if len(read) == 0 || handleReply(read) {
			continue
		}

		switch read.Action() {
		case Command:
			log.Printf("LCM.handle: read(Command): %#x", read.Function())

			// Delay before and after writing to give the LCD MCU
			// more time for processing, it is very fickle.
			time.Sleep(ReplyDelay)
			reply := read.ReplyOk()
			err := m.write(append(reply, checksum(reply)))
			log.Printf("LCM.handle: read(Command): Sent reply %#x, err: %v", reply, err)
			time.Sleep(ReplyDelay)

		case Reply:
			log.Printf("LCM.handle: read(Reply): Unhandled reply (%#x): %#x", read.Function(), read)

		default:
			log.Printf("LCM.handle: read(Unknown): %#x", read)
		}

		read = read[:len(read)-1] // Discard checksum.
		log.Printf("LCM.handle: read: Forwarding message: %#x", read)

		select {
		case m.readC <- read:

		default:
			select {
			case <-m.readC:
				log.Printf("LCM.handle: read: Buffer full, discarded earliest message")
			default:
				// Buffer got depleted.
			}

			m.readC <- read
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
