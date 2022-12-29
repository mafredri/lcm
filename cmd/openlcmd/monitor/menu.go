package monitor

import (
	"context"
	"fmt"
	"log"

	"github.com/mafredri/lcm"
)

type menuState struct {
	index   int
	item    *MenuItem
	confirm bool
}

type menu struct {
	lcm     *lcm.LCM
	home    UpdateDisplayFunc
	history []menuState
	state   menuState
	menu    *MenuItem
}

func newMenu(lcm *lcm.LCM, home UpdateDisplayFunc, item MenuItem) *menu {
	m := &menu{lcm: lcm, home: home, menu: &item}
	return m
}

func (m *menu) close() {
	m.state = menuState{}
	m.draw()
}

func (m *menu) up() {
	if m.state.item == nil {
		return
	}
	m.state.index--
	if m.state.index < 0 {
		m.state.index = len(m.state.item.SubMenu) - 1
	}
	m.draw()
}

func (m *menu) down() {
	if m.state.item == nil {
		m.draw()
		return
	}
	m.state.index++
	if m.state.index > len(m.state.item.SubMenu)-1 {
		m.state.index = 0
	}
	m.draw()
}

func (m *menu) back() {
	if len(m.history) == 0 {
		m.state = menuState{}
	} else {
		m.state = m.history[len(m.history)-1]
		m.history = m.history[:len(m.history)-1]
	}
	m.draw()
}

func (m *menu) enter() {
	if m.state.item == nil {
		m.state.item = m.menu
		m.draw()
		return
	}
	if m.state.confirm {
		m.state.item.SubMenu[m.state.index].Func(context.Background())
		return
	}

	m.history = append(m.history, m.state)
	m.state = menuState{
		item: &m.state.item.SubMenu[m.state.index],
	}
	if m.state.item.Func != nil {
		if m.state.item.Confirm {
			m.confirm()
			return
		}
		err := m.state.item.Func(context.Background())
		if err != nil {
			log.Println(err)
		}

		m.history = nil
		m.state = menuState{}
	}

	m.draw()
}

func (m *menu) draw() {
	if m.state.item == nil {
		m.home(context.Background())
		return
	}
	top, _ := lcm.SetDisplay(lcm.DisplayTop, 0, m.state.item.Name)
	bottom, _ := lcm.SetDisplay(lcm.DisplayBottom, 0, fmt.Sprintf(">%s", m.state.item.SubMenu[m.state.index].Name))
	m.lcm.Send(top)
	m.lcm.Send(bottom)
}

func (m *menu) confirm() {
	fn := m.state.item.Func
	m.state = menuState{
		confirm: true,
		item: &MenuItem{
			Name: "Are you sure?",
			SubMenu: []MenuItem{
				{
					Name: "Yes",
					Func: func(ctx context.Context) error {
						err := fn(ctx)
						m.history = nil
						m.state = menuState{}
						m.draw()
						return err
					},
				},
				{
					Name: "No",
					Func: func(context.Context) error {
						// Restore previous state.
						m.back()
						return nil
					},
				},
			},
		},
	}
	m.draw()
}

type MenuItem struct {
	Name    string
	Confirm bool
	Func    UpdateDisplayFunc
	SubMenu []MenuItem
}
