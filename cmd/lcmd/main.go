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

	var chip *gpiod.Chip

	// Find gpiochip representing it87.
	for _, name := range gpiod.Chips() {
		c, err := gpiod.NewChip(name, gpiod.WithConsumer("openlcmd"))
		if err != nil {
			panic(err)
		}
		if c.Label == "gpio_it87" {
			chip = c
			break
		}
		c.Close()
	}
	defer chip.Close()

	powerLine, err := chip.RequestLine(59, gpiod.AsOutput(1))
	if err != nil {
		panic(err)
	}
	defer powerLine.Close()

	m, err := lcm.Open(lcm.DefaultTTY)
	if err != nil {
		log.Println(err)
	}
	defer m.Close()

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
			case <-time.After(2 * time.Second):
				send(m, lcm.DisplayOff)
				send(m, lcm.DisplayStatus)
				setDisplay(m, lcm.DisplayTop, 1, "openlcmd v0.0.1")
				setDisplay(m, lcm.DisplayBottom, 0, "")
				<-activityC
			}
		}
	}()

	go func() {
		for {
			b := m.Recv()
			switch b.Action() {
			case lcm.Command:
				switch b.Function() {
				case lcm.ButtonPress:
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

				case lcm.VersionRequest:
					log.Printf("Detected LCM MCU version %d.%d.%d", b[3], b[4], b[5])
				}
			case lcm.Reply:
				// Command done.
			default:
				return
			}
		}
	}()

	go func() {
		send(m, lcm.RequestMCUVersion)
		time.Sleep(300 * time.Millisecond)

		// initalize(m)
		// time.Sleep(time.Second)
		// powerCycle(powerLine)

		initalize(m)

		activity()

		time.Sleep(time.Second)
	}()

	<-ctx.Done()
}

func powerCycle(line *gpiod.Line) {
	line.SetValue(0)
	time.Sleep(250 * time.Millisecond)
	line.SetValue(1)

	// It takes ~5 seconds for the screen to boot up.
	time.Sleep(6 * time.Second)
}

func initalize(m *lcm.LCM) {
	send(m, lcm.DisplayOn)
	send(m, lcm.DisplayStatus)

	// Clear display lines.
	setDisplay(m, lcm.DisplayTop, 0, " openlcmd v0.0.1")
	setDisplay(m, lcm.DisplayBottom, 0, "")
}

func send(m *lcm.LCM, b []byte) {
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
