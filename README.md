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
