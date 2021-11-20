/*
Package lcm implements the serial communication protocol for the ASUSTOR
LCD display. This includes controlling and updating and listening for
button presses.

LCM data format:

	MESSAGE_TYPE DATA_LENGTH FUNCTION [[DATA]...] [CRC]
*/
package lcm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/pkg/term"
)

//go:generate protoc --proto_path=. --go_out=. --go-grpc_out=. ./stream/stream.proto

const (
	// DefaultReplyTimeout defines how long we wait for a reply,
	// usually one is received within 10ms. The ASUSTOR daemon
	// resends messages after 100ms if no response is received.
	DefaultReplyTimeout = 20 * time.Millisecond
	// DefaultRetryLimit defines how many times a command will be
	// retried until giving up. Given the default reply timeout,
	// this could lead to nothing happening on the screen for about
	// one second.
	//
	// ASUSTOR tries up to 100 times, however, this rarely helps
	// clear up the communication error.
	DefaultRetryLimit = 25
	// DefaultWriteDelay defines how long to wait before writing the
	// next message. This is used both when writing commands and
	// responding to commands from the display.
	//
	// The ASUSTOR lcmd binary uses 15ms and 45ms sleeps between
	// certain commands, but this seems excessive. Instead we use
	// increasing backoff where applicable because spamming at the
	// same interval can lead to the display not responding at all.
	DefaultWriteDelay = 5000 * time.Microsecond
)

// DefaultTTY represents the default serial tty for LCM.
const DefaultTTY = "/dev/ttyS1"

// LCM represents the ASUSTOR Liquid Crystal Monitor.
type LCM struct {
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
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

	ctx, cancel := context.WithCancel(context.Background())
	m := &LCM{
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
		s:        s,
		writeC:   make(chan sendMessage, 2),
		rawReadC: make(chan Message, 2),
		readC:    make(chan []byte, 5),
	}

	go m.read()
	go m.handle()

	return m, nil
}

type sendMessage struct {
	err          chan error
	data         Message
	retryLimit   int
	replyTimeout time.Duration
	writeDelay   time.Duration
}

// Send messages to the display. Note that checksum should be omitted,
// it is handled transparently as part of the protocol implementation.
//
// TODO(mafredri): Add support for functional arguments:
//
// 	m.Send(msg, lcm.WithRetryLimit(100), lcm.WithReplyTimeout(5 * time.Millisecond))
//
func (m *LCM) Send(msg Message) error {
	data := make([]byte, len(msg), len(msg)+1)
	copy(data, msg)
	data = append(data, checksum(data))

	sm := sendMessage{
		err:          make(chan error, 1),
		data:         data,
		retryLimit:   DefaultRetryLimit,
		replyTimeout: DefaultReplyTimeout,
		writeDelay:   DefaultWriteDelay,
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
				continue
			}
			// TODO(mafredri): Close LCM.
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
	log.Printf("LCM.write: Wrote msg: %#x %d, err: %v", data, n, err)
	if err != nil {
		return err
	}
	return nil
}

// handle incoming and outgoing messages.
func (m *LCM) handle() {
	defer close(m.done)

	var id int64
	var retry func()
	var handleReply func(Message) bool
	var replyTimeout <-chan time.Time

	for {
		var read Message

		// Prioritize processing all messages from the LCM before
		// sending commands. The replyTimeout also serves as a
		// guard against concurrent writes.
		if len(m.rawReadC) > 0 || replyTimeout != nil {
			select {
			case read = <-m.rawReadC:

			case <-replyTimeout:
				log.Printf("LCM.handle: write(%d): timeout, retry...", id)
				retry()

			case <-m.ctx.Done():
				return
			}
		} else {
			select {
			case read = <-m.rawReadC:

			// Handle writes, each write must complete (or fail)
			// before the next one is handled.
			case w := <-m.writeC:
				id++
				log.Printf("LCM.handle: write(%d): %#x", id, w.data)

				// Define reply function for verifying
				// that the command was successful.
				handleReply = func(reply Message) bool {
					if reply.Type() == Reply && reply.Function() == w.data.Function() {
						if reply.Ok() {
							log.Printf("LCM.handle: write(%d): reply OK", id)
							close(w.err)
							handleReply = nil
							retry = nil
							replyTimeout = nil
						} else {
							log.Printf("LCM.handle: write(%d): reply ERROR (%#x), retrying...", id, reply.Value())
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
						// We gave it a try, not much more we can do...
						// Caller could try power-cycling the display.
						if wErr != nil {
							w.err <- fmt.Errorf("retry limit exceeded: %d/%d: last write error: %w", tries-1, w.retryLimit, wErr)
						} else {
							w.err <- fmt.Errorf("retry limit exceeded: %d/%d", tries-1, w.retryLimit)
						}
						handleReply = nil
						retry = nil
						replyTimeout = nil

						return
					}

					// Add a small delay before each write to
					// ensure the serial port is not spammed.
					time.Sleep(w.writeDelay)

					tries++
					err := m.write(w.data)
					if err != nil {
						log.Printf("LCM.handle: write(%d): %#x: %v", id, w.data, err)
						wErr = err
					}

					replyTimeout = time.After(DefaultReplyTimeout)
				}

				retry() // Initiate first try.

			case <-m.ctx.Done():
				return
			}
		}

		if len(read) == 0 || (handleReply != nil && handleReply(read)) {
			continue
		}

		switch read.Type() {
		case Command:
			log.Printf("LCM.handle: read(Command): %#x", read.Function())

			// NOTE(mafredri): In principle, the protocol supports
			// sending acknowledgements, however, sending
			// acknowledgements to the display often leads to
			// corruption and seems to work fine without.
			if false {
				time.Sleep(DefaultWriteDelay)
				reply := read.ReplyOk()
				err := m.write(append(reply, checksum(reply)))
				log.Printf("LCM.handle: read(Command): sent reply %#x, err: %v", reply, err)
			}

		case Reply:
			log.Printf("LCM.handle: read(Reply): unhandled reply (%#x): %#x", read.Function(), read)

		default:
			log.Printf("LCM.handle: read(Unknown): %#x", read)
		}

		read = read[:len(read)-1] // Discard checksum.
		log.Printf("LCM.handle: read: forwarding message: %#x", read)

		select {
		case m.readC <- read:

		default:
			select {
			case <-m.readC:
				log.Printf("LCM.handle: read: buffer full, discarded earliest message")
			default:
				// Buffer got depleted.
			}

			m.readC <- read
		}
	}
}

// Close the serial connection.
func (m *LCM) Close() error {
	m.cancel()
	<-m.done
	return m.s.Close()
}

func checksum(b []byte) (s byte) {
	for _, bb := range b {
		s += bb
	}
	return s
}
