package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bendahl/uinput"
	"github.com/warthog618/gpiod"

	"github.com/mafredri/lcm"
)

const (
	it87ChipLabel   = "gpio_it87"
	it87LCMPowerPin = 59
)

func main() {
	// TODO(): Configuration.

	log.SetFlags(log.Flags() | log.Lmicroseconds)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	kbd, err := uinput.CreateKeyboard("/dev/uinput", []byte("openlcmd"))
	if err != nil {
		panic(err)
	}
	defer kbd.Close()

	powerCycle, pcClose := powerCycler()
	defer pcClose()
	_ = powerCycle

	m, err := lcm.Open(lcm.DefaultTTY)
	if err != nil {
		log.Println(err)
	}
	defer m.Close()

	// Keep track of activity, sleep and reset the screen on timeout.
	activityC := make(chan struct{}, 1)
	activity := func() {
		select {
		case activityC <- struct{}{}:
		default:
		}
	}
	go func() {
		<-activityC
		for {
			select {
			case <-activityC:
			case <-time.After(15 * time.Second):
				send(m, lcm.DisplayOff)
				send(m, lcm.DisplayStatus)
				resetText(m)
				<-activityC
			}
		}
	}()

	// Listen for protocol messages, mainly to react to button presses.
	go func() {
		for {
			b := m.Recv()
			switch b.Type() {
			case lcm.Command:
				switch b.Function() {
				case lcm.FButton:
					btn := lcm.Button(b[3])
					switch btn {
					case lcm.Up:
						kbd.KeyPress(uinput.KeyUp)
					case lcm.Down:
						kbd.KeyPress(uinput.KeyDown)
					case lcm.Back:
						kbd.KeyPress(uinput.KeyBack)
					case lcm.Enter:
						kbd.KeyPress(uinput.KeyEnter)
					}

					log.Printf("Button press: %s", btn)

					// Screen is implicitly woken on button
					// press, so reset inactivity timer.
					activity()

				case lcm.FVersion:
					log.Printf("Detected LCM MCU version %d.%d.%d", b[3], b[4], b[5])
				}
			case lcm.Reply:
				// if b.Function() == lcm.FText && !b.Ok() {
				// 	send(m, lcm.UnknownCommand0x21)
				// }
				// Command done.
			default:
				return
			}
		}
	}()

	// Initialization routine.
	go func() {
		// initalize(m)
		// time.Sleep(time.Second)
		//
		// send(m, lcm.UnknownCommand0x23)
		// time.Sleep(time.Second)
		// os.Exit(1)

		send(m, lcm.DisplayOn)
		send(m, lcm.DisplayStatus)
		setDisplay(m, lcm.DisplayBottom, 0, "")

		next := lcm.Scroll(lcm.DisplayTop, "Welcome to openlcmd!")
		for {
			b, start, done := next()
			send(m, b)
			activity()
			if start && done {
				break
			}
			if start || done {
				time.Sleep(2 * time.Second)
			} else {
				time.Sleep(75 * time.Millisecond)
			}
		}

		resetText(m)

		activity()
	}()

	// TODO(mafredri): In case of unrecoverable errors
	// powerCycle(powerLine)

	<-ctx.Done()
}

func resetText(m *lcm.LCM) {
	// Clear display lines.
	setDisplay(m, lcm.DisplayTop, 0, " openlcmd v0.0.1")
	setDisplay(m, lcm.DisplayBottom, 0, "")
}

func send(m *lcm.LCM, b lcm.Message) {
	err := m.Send(b)
	if err != nil {
		log.Println(err)
	}
}

func setDisplay(m *lcm.LCM, line lcm.DisplayLine, indent int, text string) {
	b, err := lcm.SetDisplay(line, indent, text)
	if err != nil {
		panic(err)
	}
	send(m, b)
}

func powerCycler() (func(), func() error) {
	var chip *gpiod.Chip

	// Find gpiochip representing it87.
	for _, name := range gpiod.Chips() {
		c, err := gpiod.NewChip(name, gpiod.WithConsumer("openlcmd"))
		if err != nil {
			panic(err)
		}
		if c.Label == it87ChipLabel {
			chip = c
			break
		}
		c.Close()
	}

	line, err := chip.RequestLine(it87LCMPowerPin, gpiod.AsOutput(1))
	if err != nil {
		chip.Close()
		panic(err)
	}

	close := func() error {
		err1 := line.Close()
		err2 := chip.Close()
		if err1 != nil {
			return err1
		}
		return err2
	}
	cycle := func() {
		line.SetValue(0)
		time.Sleep(250 * time.Millisecond)
		line.SetValue(1)

		// It takes ~5 seconds for the screen to boot up.
		time.Sleep(6 * time.Second)
	}
	return cycle, close
}

func requestVersion(m *lcm.LCM) {
	// The version command is picky, needs time to think.
	time.Sleep(100 * time.Millisecond)
	send(m, lcm.RequestVersion)
	time.Sleep(300 * time.Millisecond)
}
