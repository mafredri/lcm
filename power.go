package lcm

import (
	"fmt"
	"time"

	"github.com/warthog618/gpiod"
)

const (
	it87ChipLabel   = "gpio_it87"
	it87LCMPowerPin = 59

	// It takes ~5 seconds for the screen to settle after power on.
	lcmPowerOnSettleTime = 6 * time.Second
	lcmPowerToggleTime   = 250 * time.Millisecond
)

// Power management via GPIO line.
type Power struct {
	chip *gpiod.Chip
	line *gpiod.Line
}

// On turns the LCM on.
func (p *Power) On() {
	p.line.SetValue(1)
}

// Off turns the LCM off.
func (p *Power) Off() {
	p.line.SetValue(0)
}

// Cycle the LCM power and return a channel that blocks until initial
// animation is completed.
func (p *Power) Cycle() (initialAnimationComplete <-chan time.Time) {
	p.Off()
	time.Sleep(lcmPowerToggleTime)
	p.On()

	return time.After(lcmPowerOnSettleTime)
}

// Close the GPIO line.
func (p *Power) Close() error {
	err1 := p.line.Close()
	err2 := p.chip.Close()
	if err2 != nil {
		return err2
	}
	return err1
}

// NewPower initializes the GPIO line for powering LCM on and off.
func NewPower(consumer string) (*Power, error) {
	p := &Power{}

	// Find gpiochip representing it87.
	for _, name := range gpiod.Chips() {
		c, err := gpiod.NewChip(name, gpiod.WithConsumer(consumer))
		if err != nil {
			panic(err)
		}
		if c.Label == it87ChipLabel {
			p.chip = c
			break
		}
		c.Close()
	}

	if p.chip == nil {
		return nil, fmt.Errorf("gpiochip %s not found", it87ChipLabel)
	}

	var err error
	p.line, err = p.chip.RequestLine(it87LCMPowerPin, gpiod.AsOutput(1))
	if err != nil {
		p.chip.Close()
		return nil, fmt.Errorf("request gpio line %d failed: %w", it87LCMPowerPin, err)
	}

	return p, nil
}
