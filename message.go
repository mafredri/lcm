package lcm

import (
	"errors"
	"fmt"
	"strings"
)

// Message represents a serial port message with common bits easily accessible.
type Message []byte

// Type returns the message type.
func (m Message) Type() Type {
	if len(m) == 0 {
		return 0
	}
	return Type(m[0])
}

// Function returns the message function.
func (m Message) Function() Function {
	if len(m) == 0 {
		return 0
	}
	return Function(m[2])
}

func (m Message) Value() []byte {
	if len(m) == 0 {
		return nil
	}
	return m[3 : 3+m[1]]
}

func (m Message) Ok() bool {
	if len(m) == 0 {
		return false
	}
	return m[3] == 0
}

// ReplyOk returns a valid Reply for a Command.
func (m Message) ReplyOk() Message {
	if m.Type() == Command {
		return []byte{byte(Reply), 0x01, byte(m.Function()), 0x00}
	}
	return nil
}

// Check that the message is valid (message must not include a checksum).
func (m Message) Check() error {
	if len(m) < 4 {
		return errors.New("message too short")
	}
	if m.Type() != Command && m.Type() != Reply {
		return errors.New("unknown message type")
	}
	if int(m[1])+3 != len(m) {
		return errors.New("wrong message length")
	}
	return nil
}

// Type represents the message type.
type Type byte

// LCM message types.
const (
	Command Type = 0xF0
	Reply   Type = 0xF1
)

// Function represents the message function.
type Function byte

const (
	fflush   Function = 0x00 // Not a real function.
	Fon      Function = 0x11
	Fclear   Function = 0x12
	Fversion Function = 0x13
	Fstatus  Function = 0x22
	Ftext    Function = 0x27
	Fbutton  Function = 0x80
)

// Known commands (for sending to display).
var (
	// flushMCUBuffer is a made up message but is used to resolve
	// serial communication errors, see (*LCM).forceFlushMCU.
	flushMCUBuffer Message = []byte{byte(Command), 0x01, byte(fflush), 0x00}
	// DisplayOn turns the display on.
	DisplayOn Message = []byte{byte(Command), 0x01, byte(Fon), 0x01}
	// DisplayOff turns the display off.
	DisplayOff Message = []byte{byte(Command), 0x01, byte(Fon), 0x00}
	// ClearDisplay clears the current text from the display.
	// Called during re-initialization.
	ClearDisplay Message = []byte{byte(Command), 0x01, byte(Fclear), 0x01}
	// DisplayStatus has an unknown purpose. It is issued after
	// DisplayOn in the init-routine and sometimes before/after
	// updating the text.
	DisplayStatus Message = []byte{byte(Command), 0x01, byte(Fstatus), 0x00}
	// RequestVersion reports the MCU version via command.
	// The only observed version number so far is 0.1.2 on both
	// AS604T and AS6204T.
	//
	// Note: Issuing this request takes 200+ms and acknowledging
	// that we received the message often results in the display
	// thinking we re-requested the version. ASUSTOR does not seem
	// to use this, perhaps there is only one version out there.
	//
	// => 0xf001130105
	// <= 0xf101130005 (ack)
	// <= 0xf0031300010209 (version)
	RequestVersion Message = []byte{byte(Command), 0x01, byte(Fversion), 0x01}
)

var (
	// UnknownCommand0x21, sometimes used between text updates,
	// after setting line 0 and before clearing line 1.
	//
	// Observed behavior: Nothing.
	UnknownCommand0x21 Message = []byte{byte(Command), 0x01, 0x21, 0x00}
	// UnknownCommand0x23, unused. Values come from function arguments.
	//
	// Observed behavior: Nothing.
	UnknownCommand0x23 Message = []byte{byte(Command), 0x02, 0x23, 0x00, 0x00}
	// UnknownCommand0x25, used by Lcmd_User_Menu_Ctl. Values from
	// data in memory, may have something to do with editing menus.
	//
	// Observed behavior: Places one artifact on the display, location
	// depends on values.
	UnknownCommand0x25 Message = []byte{byte(Command), 0x03, 0x25, 0x00, 0x00, 0x00}
	// UnknownCommand0x26, unused.
	//
	// Observed behavior: Clears the display so that there is only
	// one underscore on the top row.
	UnknownCommand0x26 Message = []byte{byte(Command), 0x01, 0x26, 0x00}
)

// Replies are acknowledgements to commands, when the payload bit is
// zero, the command was successfully received (and applied), when it's
// non-zero, there was an error.
//
// We don't know exactly what the different non-zero bits mean other
// than an error occurred. The bits are usually either 0x02 or 0x04,
// but even ASUSTORs lcmd binary does not care, it simply re-issues
// commands on any non-zero bit.
//
// Documented here are some mysteries found in the lcmd binary.
var (
	// UnknownReply0x10, unused in the lcmd binary. We don't know
	// the purpose of the 0x10 function, but it may be possible for
	// the display to issue this command, in which case this would
	// be the (error) response.
	UnknownReply0x10 Message = []byte{byte(Reply), 0x01, 0x10, 0x02}
	// UnknownReply0x10, unused in the lcmd binary. This is an error
	// reply issued by the display as a response to the On function,
	// however, it's purpose in the lcmd binary is unknown.
	UnknownReply0x11 Message = []byte{byte(Reply), 0x01, byte(Fon), 0x02}
)

// Button represents a LCM button.
//go:generate stringer -type=Button
type Button byte

// Button enums.
const (
	Up Button = iota + 1
	Down
	Back
	Enter
)

// DisplayLine specifies which line to write the text on.
type DisplayLine int

// DisplayLine enums.
const (
	DisplayTop DisplayLine = iota
	DisplayBottom
)

// SetDisplay allows 16 characters to be written on either the top or
// bottom line, and indent can be used in which case not all characters
// in the message will be visible.
//
// When using indent, it's a good idea to fill the display with spaces
// before (first) use so that there is no stray characters in the
// beginning. This can be achieved by first setting the display to the
// empty string as it will be padded with spaces.
//
//      SetDisplay(DisplayTop, 0, "")
//      SetDisplay(DisplayTop, 2, "My message")
//
func SetDisplay(line DisplayLine, indent int, text string) (raw Message, err error) {
	if line != DisplayTop && line != DisplayBottom {
		return nil, errors.New("display line out of bounds")
	}
	if indent > 0xF {
		return nil, errors.New("indentation out of bounds, [0, 15]")
	}
	if len(text) > 16 {
		return nil, errors.New("text too long")
	}
	if len(text) < 16 {
		text += strings.Repeat(" ", 16-len(text))
	}

	raw = append([]byte{byte(Command), 0x12, byte(Ftext), byte(line), byte(indent)}, []byte(text)...)
	return raw, nil
}

// Scroll the text on the display. Each invocation of next() will return
// a message to send. The start value indicates that the text is in the
// starting position and the done value indicates one rotation has
// completed. Done becomes true one step before start meaning that the
// starting position is not yet reached (we have scrolled to the end).
//
// 	next := lcm.Scroll(lcm.DisplayTop, "This text will scroll")
// 	for {
// 		b, start, done := next()
// 		send(m, b)
// 		if start {
// 			time.Sleep(2 * time.Second)
// 		} else {
// 			time.Sleep(1 * time.Second)
// 		}
// 		if start && done {
// 			break
// 		}
// 	}
func Scroll(line DisplayLine, text string) (next func() (raw Message, start, done bool)) {
	i := 0
	done := false
	return func() (Message, bool, bool) {
		if i >= len(text)-16 {
			done = true
		}
		if i > len(text)-16 {
			i = 0
		}
		start := i == 0
		trunc := text[i:]
		if len(trunc) > 16 {
			trunc = trunc[:16]
		}
		i++
		b, _ := SetDisplay(line, 0, trunc)
		return b, start, done
	}
}

// ShowAllCharCodes allows all character codes to be
func ShowAllCharCodes() (next func() (line1, line2 Message, start, done bool), goBack func()) {
	var i uint8
	chars := make([]byte, 16)
	done := false
	next = func() (Message, Message, bool, bool) {
		for j := 0; j < 16; j++ {
			chars[j] = 1 + i + uint8(j)
		}
		line1, _ := SetDisplay(DisplayTop, 0, string(chars))
		line2, _ := SetDisplay(DisplayBottom, 0, fmt.Sprintf("%03d..........%03d", i, i+15))

		start := i == 0
		i += 16
		if i == 0 {
			done = true
		}

		return line1, line2, start, done
	}
	return next, func() { i -= 16 * 2 }
}
