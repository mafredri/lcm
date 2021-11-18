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
