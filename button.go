package lcm

// Button represents a LCM button.
//go:generate stringer -type=Button
type Button uint8

// Button enums.
const (
	Up Button = iota + 1
	Down
	Back
	Enter
)

// ButtonReply represents the reply sent when receiving a button press.
var ButtonReply = []byte{ReplyByte, 0x01, 0x80, 0x01}
