package lcm

import (
	"errors"
	"strings"
)

// TODO(mafredri): Figure out if there are even more commands, and what,
// if anything, modifying the argument value does.
var (
	// DisplayStatus is used to establish (initial) sync, it is sent
	// repeatedly until the correct response is received. It is also
	// used as an occasional probe that everything is OK?
	//
	// TODO(mafredri): Try to verify if this message is correct
	// and/or if it has a dual purpose.
	DisplayStatus = []byte{CommandByte, 0x01, 0x11, 0x01}
	// DisplayOff turns the display off.
	DisplayOff = []byte{CommandByte, 0x01, 0x11, 0x00}
	// ClearDisplay clears the current text from the display.
	ClearDisplay = []byte{CommandByte, 0x01, 0x12, 0x01}
	// DisplayOn turns the display on.
	//
	// TODO(mafredri): Verify if this is the only purpose?
	DisplayOn = []byte{CommandByte, 0x01, 0x22, 0x00}
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
	// UnknownReply1, perhaps a generic display error, can be a
	// response to DisplayStatus or even updating of the display
	// text (0x27).
	UnknownReply1 = []byte{ReplyByte, 0x01, 0x11, 0x04, 0x07}
	// UnknownReply2 is an error reply to updating the display text?
	UnknownReply2 = []byte{ReplyByte, 0x01, 0x27, 0x02, 0x1b}
	// UnknownReply3 is an error reply to updating the display text?
	UnknownReply3 = []byte{ReplyByte, 0x01, 0x27, 0x04, 0x1d}
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
func SetDisplay(line DisplayLine, indent int, text string) (raw []byte, err error) {
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

	raw = append([]byte{CommandByte, 0x12, 0x27, byte(line), byte(indent)}, []byte(text)...)
	return raw, nil
}
