package monitor

import (
	"context"
	"log"
	"time"

	"github.com/mafredri/lcm"
)

const activityTimeout = 15 * time.Second

type UpdateDisplayFunc func(context.Context) error

type Monitor struct {
	ctx    context.Context
	cancel context.CancelFunc
	lcm    *lcm.LCM
	p      *lcm.Power
	off    bool
	home   UpdateDisplayFunc
	menu   *menu
	actC   chan struct{}
}

func New(ctx context.Context, name string, l *lcm.LCM) *Monitor {
	p, err := lcm.NewPower(name)
	if err != nil {
		log.Printf("power cycling disabled: %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	m := &Monitor{
		ctx:    ctx,
		cancel: cancel,
		lcm:    l,
		p:      p,
		menu:   &menu{},
		actC:   make(chan struct{}),
	}

	go m.idle()
	go m.recv()

	return m
}

func (m *Monitor) SetHome(fn UpdateDisplayFunc) {
	m.home = fn
}

func (m *Monitor) SetMenu(item MenuItem) {
	m.menu = newMenu(m.lcm, m.home, item)
	if m.home != nil {
		m.home(m.ctx)
	}
}

func (m *Monitor) Confirm(ctx context.Context, msg string) bool {
	m.menu.confirm()
	return true
}

func (m *Monitor) Send(msg lcm.Message) error {
	select {
	case m.actC <- struct{}{}:
	default:
	}
	return m.lcm.Send(msg)
}

func (m *Monitor) idle() {
	defer func() {
		if m.p != nil {
			m.p.Close()
		}
	}()

	<-m.actC

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.actC:
		case <-time.After(activityTimeout):
			m.off = true
			m.send(lcm.DisplayOff)
			m.send(lcm.DisplayStatus)
			m.menu.close()
			<-m.actC
			m.off = false
		}
	}
}

func (m *Monitor) recv() {
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		b := m.lcm.Recv()
		switch b.Type() {
		case lcm.Command:
			switch b.Function() {
			case lcm.Fbutton:
				btn := lcm.Button(b.Value()[0])
				log.Printf("Button press: %s", btn)

				switch btn {
				case lcm.Up:
					m.menu.up()
				case lcm.Down:
					m.menu.down()
				case lcm.Back:
					m.menu.back()
				case lcm.Enter:
					m.menu.enter()
				}

				// Screen is implicitly woken on button
				// press, so reset inactivity timer.
				select {
				case m.actC <- struct{}{}:
				default:
				}

			case lcm.Fversion:
				ver := b.Value()
				log.Printf("Detected LCM MCU version %d.%d.%d", ver[0], ver[1], ver[2])

			default:
				log.Printf("Unhandled command: %#x", b.Function())
			}

		case lcm.Reply:

		default:
			log.Printf("Unknown message type: %v", b.Type())
		}
	}
}

func (m *Monitor) send(b lcm.Message) {
	err := m.lcm.Send(b)
	if err != nil {
		log.Println(err)
	}
}

func (m *Monitor) PowerCycle() {
	if m.p != nil {
		<-m.p.Cycle()
	}
}

func (m *Monitor) Close() error {
	m.cancel()
	return nil
}
