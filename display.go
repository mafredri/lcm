package lcm

import (
	"errors"
	"strings"
)

type Message []byte

func (m Message) Action() Action {
	if len(m) == 0 {
		return 0
	}
	return Action(m[0])
}

func (m Message) Function() Function {
	if len(m) == 0 {
		return 0
	}
	return Function(m[2])
}

func (m Message) ReplyOk() Message {
	if m.Action() == Command {
		return []byte{byte(Reply), 0x01, byte(m.Function()), 0x00}
	}
	return nil
}

type Action byte

// LCM message types.
const (
	Command Action = 0xF0
	Reply   Action = 0xF1
)

type Function byte

const (
	On             Function = 0x11
	Clear          Function = 0x12
	VersionRequest Function = 0x13
	Status         Function = 0x22
	SetText        Function = 0x27
	ButtonPress    Function = 0x80
)

// TODO(mafredri): Figure out if there are even more commands, and what,
// if anything, modifying the argument value does.
//
//	Init:
//		0xF0 01 11 01
//		0xF0 01 22 00
//		...
//		0xF0 01 11 01
//		...
//		0xF0 01 11 00 (every 9th second)
//	MsgTask:
//		0xF0 01 11 00
//		0xF0 12 27 ...
//		0xF0 12 27 ...
//		0xF0 01 11 00 (10 seconds time up)
//
//		0xF0 01 22 00
//
//		0xF0 01 12 01 (Re-initialize)
//		0xF0 12 27 ...
//		0xF0 12 27 ...
//		0xF0 12 27 ...
//		0xF0 01 11 01
//
//		0xF0 01 22 00
//
//		0xF0 01 11 01
var (
	// DisplayOn turns the display on.
	DisplayOn Message = []byte{byte(Command), 0x01, byte(On), 0x01}
	// DisplayOff turns the display off.
	DisplayOff Message = []byte{byte(Command), 0x01, byte(On), 0x00}
	// ClearDisplay clears the current text from the display.
	// Called during re-initialization.
	ClearDisplay Message = []byte{byte(Command), 0x01, byte(Clear), 0x01}
	// DisplayStatus has an unknown purpose. It is issued after
	// DisplayOn in the init-routine and sometimes before/after
	// updating the text.
	DisplayStatus Message = []byte{byte(Command), 0x01, byte(Status), 0x00}
)

var (
	// RequestMCUVersion, unused. Reports the MCU version via
	// command. The only observed version number so far is 0.1.2 on
	// both AS604T and AS6204T.
	// => 0xf101130005 (ack)
	// => 0xf0031300010209 (version)
	RequestMCUVersion Message = []byte{byte(Command), 0x01, byte(VersionRequest), 0x01}
	// UnknownCommand0x21, sometimes used between text updates,
	// after setting line 0 and before clearing line 1.
	UnknownCommand0x21 Message = []byte{byte(Command), 0x01, 0x21, 0x00}
	// UnknownCommand0x23, unused. Values come from function arguments.
	UnknownCommand0x23 Message = []byte{byte(Command), 0x02, 0x23, 0x00, 0x00}
	// UnknownCommand0x25, used by Lcmd_User_Menu_Ctl. Values from
	// data in memory, may have something to do with editing menus.
	UnknownCommand0x25 Message = []byte{byte(Command), 0x03, 0x25, 0x00, 0x00, 0x00}
	// UnknownCommand0x26, unused.
	//
	// Observed behavior: Clears the display.
	UnknownCommand0x26 Message = []byte{byte(Command), 0x01, 0x26, 0x00}
)

// UnknownReplies seem to occur when there is either some communication
// error or problem updating the display?
//
// May appear in a corrupted byte sequence, like:
//
//     []byte{0xf1, 0x01, 0x27, 0x82, 0x01, 0x27, 0x02, 0x1b}
//
// In this example it's likely that this is actually supposed to be two
// instances of UnknownReply2.
var (
	// UnknownReply0, unused. Value comes from func (param + 2).
	UnknownReply0 = []byte{byte(Reply), 0x01, 0x10, 0x00}
	// UnknownReply1, perhaps a generic display error, can be a
	// response to DisplayStatus or even updating of the display
	// text (0x27).
	// Also found in lcmd binary, value comes from func (param + 2).
	UnknownReply1 = []byte{byte(Reply), 0x01, 0x11, 0x04, 0x07}
	// UnknownReply2 is an error reply to updating the display text?
	UnknownReply2 = []byte{byte(Reply), 0x01, byte(SetText), 0x02, 0x1b}
	// UnknownReply3 is an error reply to updating the display text?
	UnknownReply3 = []byte{byte(Reply), 0x01, byte(SetText), 0x04, 0x1d}
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

	raw = append([]byte{byte(Command), 0x12, byte(SetText), byte(line), byte(indent)}, []byte(text)...)
	return raw, nil
}
