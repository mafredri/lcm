package main

import (
	"context"
	"flag"
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
	debug := flag.Bool("debug", false, "Enable debug logging")
	enableSystemd := flag.Bool("systemd", false, "Runs in systemd mode (removes timestamps from logging)")
	enableUinput := flag.Bool("uinput", false, "Send button presses via uinput virtual keyboard (/devices/virtual/input)")

	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	flags := log.Flags()
	if *enableSystemd {
		flags ^= (log.Ldate | log.Ltime)
	} else {
		flags |= log.Lmicroseconds
		log.SetPrefix("[openlcmd] ")
	}
	log.SetFlags(flags)

	var opts []lcm.OpenOption
	if *debug {
		opts = append(opts, lcm.WithLogger(log.New(os.Stderr, "[lcm] ", flags)))
	}

	m, err := lcm.Open(lcm.DefaultTTY, opts...)
	if err != nil {
		log.Println(err)
	}
	defer m.Close()

	var kbd uinput.Keyboard
	if *enableUinput {
		kbd, err := uinput.CreateKeyboard("/dev/uinput", []byte("openlcmd"))
		if err != nil {
			panic(err)
		}
		defer kbd.Close()
	}

	powerCycle, pcClose := powerCycler()
	defer pcClose()
	_ = powerCycle

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
	btnCh := make(chan lcm.Button)
	go func() {
		for {
			b := m.Recv()
			switch b.Type() {
			case lcm.Command:
				switch b.Function() {
				case lcm.Fbutton:
					btn := lcm.Button(b.Value()[0])
					kp := 0
					switch btn {
					case lcm.Up:
						kp = uinput.KeyUp
					case lcm.Down:
						kp = uinput.KeyDown
					case lcm.Back:
						kp = uinput.KeyBack
					case lcm.Enter:
						kp = uinput.KeyEnter
					}
					log.Printf("Button press: %s", btn)
					select {
					case btnCh <- btn:
					default:
					}

					if kbd != nil && kp > 0 {
						kbd.KeyPress(kp)
					}

					// Screen is implicitly woken on button
					// press, so reset inactivity timer.
					activity()

				case lcm.Fversion:
					ver := b.Value()
					log.Printf("Detected LCM MCU version %d.%d.%d", ver[0], ver[1], ver[2])
				}
			case lcm.Reply:

			default:
				return
			}
		}
	}()

	// Initialization routine.
	go func() {
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

		nextChars, goBack := lcm.ShowAllCharCodes()
		for {
			b1, b2, _, _ := nextChars()
			send(m, b1)
			send(m, b2)
			activity()

			btn := <-btnCh
			if btn == lcm.Up {
				goBack()
			} else if btn == lcm.Back || btn == lcm.Enter {
				break
			}
		}

		resetText(m)

		activity()
	}()

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

	if chip == nil {
		skip := func() { log.Printf("Could not find %s gpiochip, skipping power cycle...", it87ChipLabel) }
		return skip, func() error { return nil }
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
