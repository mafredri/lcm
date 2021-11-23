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
	"time"

	"github.com/pkg/term"
)

//go:generate protoc --proto_path=. --go_out=. --go-grpc_out=. ./stream/stream.proto

const (
	// DefaultReplyTimeout defines how long we wait for a reply,
	// usually one is received in under 10ms. We keep this timeout
	// fairly tight because a longer delay rarely helps.
	//
	// The ASUSTOR daemon resends messages after 100ms if no
	// response is received. But even this can leads to deadlocks
	// where the same error will be echoed back time and time again.
	DefaultReplyTimeout = 15 * time.Millisecond
	// DefaultRetryLimit defines how many times a command will be
	// retried until giving up. Given the default reply timeout,
	// this could lead to nothing happening on the screen for about
	// 750ms.
	//
	// ASUSTOR tries up to 100 times, however, this rarely helps
	// clear up the communication error.
	DefaultRetryLimit = 50
	// DefaultWriteDelay defines how long to wait before writing the
	// next message. This is used both when writing commands and
	// responding to commands from the display.
	//
	// The ASUSTOR lcmd binary uses 15ms and 45ms sleeps between
	// certain commands, but this seems excessive.
	DefaultWriteDelay = 250 * time.Microsecond
	// forceFlushDelay specifies how long to wait after attempting
	// to flush the MCU receive buffer.
	forceFlushDelay = 250 * time.Microsecond
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
	opts     openOptions
}

type openOptions struct {
	ack bool
	l   Logger
}

// OpenOption configures LCM during open.
type OpenOption func(*openOptions)

// EnableProtocolAckReply specifies if LCM should send acknowledgement
// replies to the screen when it sends us a command (e.g. button press
// or firmware version).
//
// If the screen sent us a command, technically we should acknowledge it
// by sending a reply indicating it was successful. However, it often
// causes later commands (from us) to become corrupt. The frequency of
// the corruption can be lowered with delays, but then again, it seems
// like the display does not care if we reply or not.
func EnableProtocolAckReply() OpenOption {
	return func(o *openOptions) {
		o.ack = true
	}
}

// Logger represents a generic logger (e.g. from the log package).
type Logger interface {
	Printf(format string, v ...interface{})
}

type noopLogger struct{}

func (noopLogger) Printf(format string, v ...interface{}) {}

// WithLogger sets the logger used by LCM (default none).
func WithLogger(l Logger) OpenOption {
	return func(o *openOptions) {
		o.l = l
	}
}

// Open opens the serial port for LCM.
func Open(tty string, opt ...OpenOption) (*LCM, error) {
	opts := openOptions{
		l: noopLogger{},
	}
	for _, o := range opt {
		o(&opts)
	}

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
		opts:     opts,
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

// forceFlushMCU sends a nonsense command in an attempt to flush the MCU
// receive buffer. Sometimes when the MCU gets stuck the only way to
// escape the loop is to send another command, retrying the previous
// command will keep failing in perpetuity.
//
// The fflush (0x00) command seems to have no side-effect, but funnily
// enough, the MCU will reply that the command was successful. Perhaps
// it is a real command but it's not used anywhere else.
//
// Also, sending two of these commands in one go seems to increase the
// speed of recovery further.
//
// Other attemps included sending enough zero bytes to clear the receive
// buffer, but while effective, not foolproof (a good number of bytes
// was 32 or 33) but still unrecoverable states were observed.
func (m *LCM) forceFlushMCU() {
	m.opts.l.Printf("LCM.forceFlushMCU: trying to flush MCU read buffer...")

	data := make([]byte, len(flushMCUBuffer), len(flushMCUBuffer)+1*2)
	copy(data, flushMCUBuffer)
	sum := checksum(data)
	data = append(data, sum)
	data = append(data, data...)

	_, _ = m.s.Write(data)

	// Small delay to allow the MCU to process the message.
	time.Sleep(forceFlushDelay)
}

// Send messages to the display. Note that checksum should be omitted,
// it is handled transparently as part of the protocol implementation.
//
// TODO(mafredri): Add support for functional arguments:
//
// 	m.Send(msg, lcm.WithRetryLimit(100), lcm.WithReplyTimeout(5 * time.Millisecond))
//
func (m *LCM) Send(msg Message) error {
	err := msg.Check()
	if err != nil {
		return err
	}

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

// read the serial port and transmit
// messages on the read channel.
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
				m.opts.l.Printf("LCM.read: %v", err)
				continue
			}
			// TODO(mafredri): Close LCM.
			m.opts.l.Printf("LCM.read: fatal: %v", err)
			return
		}

		b := Message(raw.Bytes())
		m.opts.l.Printf("LCM.read: OK %#x", b)
		m.rawReadC <- b
	}
}

// write to the serial port.
func (m *LCM) write(data []byte) error {
	n, err := m.s.Write(data)
	m.opts.l.Printf("LCM.write: wrote: %#x %d, err: %v", data, n, err)
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
				m.opts.l.Printf("LCM.handle: write(%d): timeout, retry...", id)
				m.forceFlushMCU()
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
				m.opts.l.Printf("LCM.handle: write(%d): %#x", id, w.data)

				// Define reply function for verifying
				// that the command was successful.
				handleReply = func(reply Message) bool {
					if reply.Type() == Reply && reply.Function() == w.data.Function() {
						if reply.Ok() {
							m.opts.l.Printf("LCM.handle: write(%d): reply OK", id)
							close(w.err)
							handleReply = nil
							retry = nil
							replyTimeout = nil
						} else {
							// We don't always forceibly flush the MCU here because it had
							// the sensibility to at least respond to our command.
							m.opts.l.Printf("LCM.handle: write(%d): reply ERROR (%#x)", id, reply.Value())
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
						m.opts.l.Printf("LCM.handle: write(%d): %#x: %v", id, w.data, err)
						wErr = err
					}

					replyTimeout = time.After(w.replyTimeout)
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
			m.opts.l.Printf("LCM.handle: read(Command): %#x", read.Function())

			reply := read.ReplyOk()
			reply = append(reply, checksum(reply))
			if m.opts.ack {
				// A delay is necessary because otherwise the
				// serial communication protcol is guaranteed
				// to become corrupt. What usually works quite
				// well is a delay somewhere between 150us and
				// 5ms. Any longer than that and it seems the
				// display forgets it's waiting for one.
				//
				// It would be possible to reply with more
				// precise control of the delay in (*LCM).read,
				// however, in practice this gives no benefit.
				time.Sleep(DefaultWriteDelay)

				err := m.write(reply)
				m.opts.l.Printf("LCM.handle: read(Command): sent ack reply %#x, err: %v", reply, err)
			} else {
				m.opts.l.Printf("LCM.handle: read(Command): protocol ack disabled, not sending reply %#v", reply)
			}

		case Reply:
			if read.Function() == fflush {
				m.opts.l.Printf("LCM.handle: read(Reply): received ack for flush: %#x", read)
			} else {
				m.opts.l.Printf("LCM.handle: read(Reply): unhandled reply (%#x): %#x", read.Function(), read)
			}

		default:
			m.opts.l.Printf("LCM.handle: read(Unknown): %#x", read)
		}

		read = read[:len(read)-1] // Discard checksum.
		m.opts.l.Printf("LCM.handle: read: forwarding message: %#x", read)

		select {
		case m.readC <- read:

		default:
			select {
			case <-m.readC:
				m.opts.l.Printf("LCM.handle: read: buffer full, discarded earliest message")
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
