package main

import (
	"github.com/mafredri/lcm"
)

func main() {
	// TODO(): Configuration.

	m, err := lcm.Open(lcm.DefaultTTY)
	if err != nil {
		panic(err)
	}
	_ = m

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
