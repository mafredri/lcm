package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bendahl/uinput"

	"github.com/mafredri/lcm"
)

func main() {
	// TODO(): Configuration.

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	kbd, err := uinput.CreateKeyboard("/dev/uinput", []byte("openlcmd"))
	if err != nil {
		panic(err)
	}
	defer kbd.Close()

	m, err := lcm.Open(lcm.DefaultTTY)
	if err != nil {
		log.Println(err)
	}

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
			case <-time.After(3 * time.Second):
				send(m, lcm.DisplayOff)
				<-activityC
			}
		}
	}()

	go func() {
		for {
			b := m.Recv()
			switch b.Type() {
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
					activity()
				}
			case lcm.Reply:
				// Command done.
			default:
				return
			}
		}
	}()

	go func() {
		send(m, lcm.DisplayOn)
		send(m, lcm.DisplayStatus)

		// Clear display lines.
		setDisplay(m, lcm.DisplayTop, 0, "")
		// setDisplay(m, lcm.DisplayBottom, 0, "")

		setDisplay(m, lcm.DisplayBottom, 0, "openlcmd v0.0.0")

		activity()

		send(m, lcm.RequestMCUVersion)
		time.Sleep(time.Second)

		// send(m, lcm.UnknownCommand0x21)
		// time.Sleep(time.Second)

		// send(m, lcm.UnknownCommand0x26)
		// send(m, lcm.ClearDisplay)
		// send(m, []byte{lcm.CommandByte, 0x01, 0x12, 0x00})

		// send(m, lcm.DisplayOn)

		// time.Sleep(10 * time.Second)
		// setDisplay(m, lcm.DisplayTop, 0, "openlcmd v0.0.0")
		// send(m, lcm.DisplayStatus)
		// activity()

		// b := make([]byte, 16)
		// for j := 1; j < 0xFF; j++ {
		// 	for i := 0; i < len(b); i++ {
		// 		b[i] = byte(j + i)
		// 	}
		// 	setDisplay(m, lcm.DisplayTop, 0, string(b))
		// 	time.Sleep(1 * time.Second)
		// 	activity()
		// }
	}()

	<-ctx.Done()

	// m.ButtonPressed(func(b lcm.Button) {
	// 	switch b {
	// 	case lcm.Up:
	// 	case lcm.Down:
	// 	case lcm.Back:
	// 	case lcm.Enter:
	// 	}
	// 	fmt.Printf("Button pressed: %v\n", b)
	// })
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
