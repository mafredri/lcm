# lcm

The lcm project implements the ASUSTOR NAS LCD serial port communication protocol. ASUSTOR has a daemon running on their NASes (`lcmd`) which is responsible for controlling the LCD and buttons, this project aims to reimplement some of this functionality.

It can be used to write text on the LCD, turn the display on and off and read button presses.

## Status

Most of the protocol has been documented in code and the library implementation is quite robust and tries to handle edge cases and error states.

For a standalone program see [`cmd/openlcmd`](cmd/openlcmd). The `openlcmd` program implements some basic functionality and can be used as a reference implementation for more advanced programs. The underlying library is implemented in the `lcm` package.

All the naming in this project is guesswork, some messages may be named incorrectly and have a hidden or unknown purpose.

## Installation

```
git clone https://github.com/mafredri/lcm
cd lcm
go build ./cmd/openlcmd
sudo cp openlcmd /usr/local/sbin/openlcmd
sudo cp cmd/openlcmd/systemd/openlcmd.service /etc/systemd/system/openlcmd.service
sudo systemctl enable openlcmd
sudo systemctl start openlcmd
```

**NOTE:** `openlcmd` does not have to run as root, but the user will need to have read/write access to `/dev/ttyS1`.

### TrueNAS SCALE

TrueNAS SCALE has a read-only root filesystem and no Go toolchain, so the steps above don't apply as-is. Instead, build a static binary in a container, keep it on a pool dataset and start it from an init script. No kernel modules are needed, so this survives TrueNAS updates.

1. Build `openlcmd` (requires Docker, which is available once an Apps pool is configured; replace `tank/lcm` with a dataset of your own):

   ```
   cd /mnt/tank/lcm
   git clone https://github.com/mafredri/lcm
   docker run --rm -e CGO_ENABLED=0 -v "$PWD/lcm":/src -w /src golang:1.25 go build -o openlcmd ./cmd/openlcmd
   ```

2. Test it in the foreground, the LCD should show the hostname and IP address and react to the buttons (Ctrl+C to stop):

   ```
   /mnt/tank/lcm/lcm/openlcmd -debug
   ```

   The message `power cycling disabled: gpiochip gpio_it87 not found` is expected, the TrueNAS kernel does not include the it87 GPIO driver. The display works without it.

3. Start it on boot via _System > Advanced > Init/Shutdown Scripts_, add a script with type _Command_, when _Post Init_ and command:

   ```
   systemd-run --unit=openlcmd -p Restart=always /mnt/tank/lcm/lcm/openlcmd -systemd
   ```

   Check it with `systemctl status openlcmd` and `journalctl -u openlcmd`.

**NOTE:** The binary runs as root from your pool. Make sure it is owned by root and that the dataset is not writable by other users (e.g. via an SMB share).

## Why?

I stopped using ADM and switched to plain Debian on my AS-604T and AS-6204T and the ASUSTOR control software is not portable. So I wrote my own.

This project was already working in 2018 but wasn't published until 2021 after a big refactor.

## Project structure

- `lcm`
  - The LCM library, implements the protocol
- `lcm/cmd/openlcmd`
  - Daemon that runs on the ASUSTOR NAS and handles updating of the LCD and reacting to button presses
  - Exposes buttons as virtual keyboard (`uinput`)
  - Can power cycle the LCD via GPIO

## Research

Initially based on captured logs (see [mafredri/asustor_as-6xxt/lcmd-logs](https://github.com/mafredri/asustor_as-6xxt/tree/master/lcmd-logs)), later reverse-engineering of ASUSTOR `lcmd` to determine.
