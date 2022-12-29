package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bendahl/uinput"
	"github.com/shirou/gopsutil/v3/net"

	"github.com/mafredri/lcm"
	"github.com/mafredri/lcm/cmd/openlcmd/monitor"
)

const (
	program = "openlcmd"
	version = "v0.0.1"
)

func main() {
	// TODO(): Configuration.
	debug := flag.Bool("debug", false, "Enable debug logging")
	enableSystemd := flag.Bool("systemd", false, "Runs in systemd mode (removes timestamps from logging)")
	enableUinput := flag.Bool("uinput", false, "Relay button presses via uinput virtual keyboard (/devices/virtual/input)")

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
		panic(err)
	}
	defer m.Close()

	var kbd uinput.Keyboard
	if *enableUinput {
		kbd, err = uinput.CreateKeyboard("/dev/uinput", []byte(program))
		if err != nil {
			panic(err)
		}
		defer kbd.Close()
	}

	mon := monitor.New(ctx, program, m, kbd)
	defer mon.Close()

	mon.SetHome(func(ctx context.Context) error {
		hostname, err := os.Hostname()
		if err != nil {
			hostname = "Unknown"
			log.Printf("hostname check failed: %v", err)
		}

		ipaddr := "0.0.0.0"
		netif, err := net.InterfacesWithContext(ctx)
		if err != nil {
			return err
		}
		for _, i := range netif {
			if i.Name == "lo" || strings.HasPrefix(i.Name, "br-") || strings.HasPrefix(i.Name, "docker") || strings.HasPrefix(i.Name, "veth") {
				continue
			}
			if len(i.Addrs) == 0 {
				continue
			}
			ipaddr = i.Addrs[0].Addr
		}

		setDisplay(mon, lcm.DisplayTop, 0, hostname)
		setDisplay(mon, lcm.DisplayBottom, 0, ipaddr)

		return nil
	})

	mon.SetMenu(
		monitor.MenuItem{
			Name: "Main",
			SubMenu: []monitor.MenuItem{
				{
					Name: "Info",
					SubMenu: []monitor.MenuItem{
						{
							Name: "WIP",
							Func: func(ctx context.Context) error {
								return nil
							},
						},
					},
				},
				{
					Name: "System",
					SubMenu: []monitor.MenuItem{
						{
							Name:    "Shutdown",
							Confirm: true,
							Func: func(ctx context.Context) error {
								// if mon.Confirm(ctx, "Are you sure?") {
								// 	setDisplay(mon, lcm.DisplayTop, 0, "Shutting down...")
								// 	setDisplay(mon, lcm.DisplayBottom, 0, "")
								// 	return exec.Command("/usr/sbin/shutdown", "-h", "now").Run()
								// }
								// mon.Back()
								return nil
							},
						},
						{
							Name:    "Restart",
							Confirm: true,
							Func: func(ctx context.Context) error {
								return nil
							},
						},
					},
				},
				{
					Name: program,
					SubMenu: []monitor.MenuItem{
						{
							Name: "Version",
							Func: func(_ context.Context) error {
								setDisplay(mon, lcm.DisplayBottom, 0, program+" "+version)
								time.Sleep(3 * time.Second)
								return nil
							},
						},
					},
				},
			},
		},
	)

	<-ctx.Done()
}

func send(m *monitor.Monitor, b lcm.Message) {
	err := m.Send(b)
	if err != nil {
		log.Println(err)
	}
}

func setDisplay(m *monitor.Monitor, line lcm.DisplayLine, indent int, text string) {
	b, err := lcm.SetDisplay(line, indent, text)
	if err != nil {
		panic(err)
	}
	send(m, b)
}
