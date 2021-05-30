# lcm

The lcm project implements the ASUSTOR NAS LCD serial port communication protocol. ASUSTOR has a similar daemon running on their NASes which is responsible for controlling the LCD and buttons.

It can be used to write text on the LCD, turn the display on and off and read button presses.

## Status

This project is a WIP. The protocol has been implemented and documented, although some questions remain.

Edge cases do exist, for instance, sometimes resending a message will work, other times it will not. Usually when resending does not work within the first 5 tries, `DisplayStatus` needs to be sent until the issue is corrected.

All the naming in this project is guesswork, some names and purposes of messages could be wrong.

## Why?

I stopped using ADM and switched to plain Debian on my AS-604T and AS-6204T and the ASUSTOR control software is not portable. So I wrote my own.

## Project structure

- `lcm`
  - The LCM library, implements the protocol
- `lcm/cmd/lcm-client`
  - This client connects to `cmd/lcm-server` and provides the "brains". Used for quick iteration and testing messages and logic
- `lcm/cmd/lcm-server`
  - A dumb server that forwards the `lcm-client` messages to the LCD and vice-versa
- `lcm/stream`
  - Protobuf / GRPC for `lcm-client` and `lcm-server`
- `lcm/cmd/lcm-monitor`
  - Can be used to monitor/intercept ASUSTOR `lcmd` communication to/from the LCD.
- `lcm/cmd/lcmd`
  - Unimplemented, will become a configureable daemon that runs on the ASUSTOR NAS and handles updating of the LCD and reacting to button presses
  - Possibly allow other applications to plug-in and temporarily take control for custom logic

## Research

See [mafredri/asustor_as-6xxt/lcmd-logs](https://github.com/mafredri/asustor_as-6xxt/tree/master/lcmd-logs).
