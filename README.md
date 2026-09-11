# flydigictl

A utility for managing the configuration of [Flydigi](https://en.flydigi.com/) controllers on Linux systems, including the Steam Deck.

It is pure Go (no libusb / cgo): it talks to the controller through `hidraw` and `usbfs`, so it cross-compiles for any Linux target.

## Supported controllers

| Controller | Protocol | Info | Third-party takeover | Calibration | Firmware check | Deadzone / LEDs |
|---|---|---|---|---|---|---|
| Vader 5 Pro (all editions) | v2 | ✅ tested | ✅ tested | ✅ | ✅ tested | ❌ |
| Apex 5 (all editions) | v2 | ✅ | ✅ | ✅ | ✅ | ❌ |
| Apex 6 / 6 Pro | v2 | ✅ | ✅ | ✅ | ✅ | ❌ |
| Vader 4 Pro (all editions) | v1 | ✅ | n/a (no such setting) | ✅ | ✅ | ✅ |
| Apex 4 (all editions) | v1 | ✅ | n/a | ✅ | ✅ | ✅ |
| Vader 3 / 3 Pro | v1 | ✅ tested | n/a | ✅ | ✅ | ✅ |
| Vader 2 / 2 Pro, Apex 2 / 3, Direwolf 2 / 3 | v1 | probably | n/a | probably | ✅ | probably |

"tested" means verified on real hardware; the rest is implemented from the same protocol descriptions (Flydigi Space Station 3 / 4 and SDL) but has not been run against a device yet.

- **v1** controllers show up as `04b4:2412` (DInput mode) or as an Xbox 360 pad `045e:028e` (XInput mode). Both work; in XInput mode the `xpad` driver is detached from the interface while `flydigid` holds the device and re-attached afterwards.
- **v2** controllers use Flydigi's own vendor ID `37d7` and expose a vendor HID interface next to the XInput one. Mapping/LED configuration for them uses a different format that is not implemented yet.

## Installing

### Steam Deck / SteamOS

No toolchain is needed on the Deck. From a machine with Go installed:

```
make deck-deploy DECK=root@<deck-ip>
```

This cross-compiles, copies the binaries to `/usr/local/bin`, installs the DBus policy, systemd units and udev rule (temporarily disabling `steamos-readonly`) and starts `flydigid`. Root SSH access to the Deck is required.

### Debian, Ubuntu and other Debian-based distros

You can download the artifact from the [latest actions run](https://github.com/pipe01/flydigictl/actions) and install the deb package.

### Other distros

Install [Go](https://go.dev/) 1.21 or newer, then run `sudo make install` to install `flydigictl` and `flydigid`.

## Usage

This project consists of two parts: a daemon that runs in the background as root, and a command line utility that talks to this daemon through a DBus interface.
The daemon will be automatically started by SystemD when the DBus interface is requested.

To check if communication to the daemon works, run `flydigictl version`. To test communication with the controller, plug it in then run `flydigictl info`:

```
              Device : 144 (Vader 5 Pro (Dragon Ball Z))
          Model code : f5
            Protocol : v2
            Firmware : 7.1.5.4
         SI firmware : 3.5.1.4
         RF firmware : 1.0.2.6
             Battery : 100% (charging)
     Connection type : wired
Third-party takeover : enabled (held by "SDL")
```

Run `flydigictl help` to see what options the program has.

### Allow third-party apps to take over mappings (Steam Input)

Vader 5 Pro and Apex 5 only expose a plain XInput gamepad unless the *"Allow third-party apps to take over mappings"* setting is enabled. With it enabled, Steam (SDL) acquires the controller directly and can map all the extra buttons. The setting lives in the controller and sometimes gets reset.

```
flydigictl takeover          # show the current state and which app holds the controller
flydigictl takeover on       # enable
flydigictl takeover off      # disable
flydigictl takeover auto on  # re-enable it automatically every time a controller is plugged in
```

`takeover auto on` creates `/etc/flydigictl/auto-takeover`; the hotplug handler (below) then turns the setting on whenever a Flydigi v2 controller is connected.

### Steam only sees a "Generic X-Box pad"

Right after the cable is plugged in the controller takes a moment to boot and doesn't answer requests yet. Steam's Flydigi driver only tries once, in the first ~100 ms, so it frequently gives up and falls back to the plain XInput interface: the controller shows up as *Generic X-Box pad* and the back buttons don't work, even though takeover is enabled. Once the controller is up, re-plugging fixes it.

`flydigictl reconnect` does that re-plug in software (it re-enumerates the USB device, the controller isn't power-cycled), and `flydigictl-hotplug.service`, started by a udev rule on every connect, does it automatically: it waits for the controller to answer, applies `takeover auto` if set, and if takeover is enabled and Steam is running but no application has opened the controller's HID interface after a few seconds, it re-enumerates the device once. Disable with `systemctl mask flydigictl-hotplug.service`.

```
flydigictl reconnect         # software re-plug, then shows who holds the controller
journalctl -u flydigictl-hotplug   # what the hotplug handler did
```

### Calibration

```
flydigictl calibrate
```

Starts the controller's built-in joystick / trigger calibration (the same procedure as Flydigi Space Station's calibration dialog), prints the instructions, and finishes calibration when you press Enter. Keep the controller on a flat surface for the first seconds so the gyro can settle, rotate both sticks fully twice, then pull both triggers fully twice. `--start` and `--stop` run the two steps separately.

Flydigi Space has no separate gyro calibration command; the gyro is zeroed at the beginning of this procedure while the controller rests on a flat surface (the on-controller button combo, e.g. SELECT+START+UP on the Apex 4, does the same thing).

### Firmware

```
flydigictl firmware check            # compare with Flydigi's update service
flydigictl firmware check --latest   # always show the newest published firmware
flydigictl firmware check --json
```

This queries the same endpoint Flydigi Space Station 4 uses (`https://api.flydigi.com/pc/Update/firmware`). When connected through the wireless dongle the dongle firmware is checked instead of the main chip. Flashing is not implemented; use Flydigi Space Station on Windows for that.

### Legacy (v1) configuration

`flydigictl joystick left|right deadzone [value]`, `flydigictl leds ...` and `flydigictl dump` read and write the on-board mapping configuration of v1 controllers. They return "not supported" on v2 controllers.

## Troubleshooting

### `flydigictl info` returns an error instead of information about the controller

- Make sure the controller is on DInput or XInput mode and not Bluetooth, Switch or others.
- Check `lsusb`: you should see `37d7:2401` / `37d7:2501` (v2), `04b4:2412` (v1 DInput) or `045e:028e Microsoft Corp. Xbox360 Controller` (v1 XInput).
- On the Deck, `journalctl -u flydigid` shows the daemon log.

### `Request to own name refused by policy` in the daemon log

The DBus policy file was installed but the bus did not reload it: `systemctl reload dbus`.

## Protocol notes

The v2 command format (from Flydigi Space Station 4 / SDL): 32-byte HID reports on the vendor interface (usage page `FFA0`), framed as `5A A5 <cmd> <len> <payload...> <crc>` where `len = len(payload) + 2` and `crc` is the byte sum of `cmd`, `len` and the payload.

| cmd | meaning |
|---|---|
| `01` | device info: id, connection, MAC, battery, chip types, firmware versions (main, dongle, SI, trigger, screen, ADC, RF) |
| `10` | read raw data / takeover status; byte 9 = takeover enabled, bytes 10-29 = name of the app holding the controller |
| `11` | enable raw data: 5 bytes `[controller, raw, keyboard, mouse, thirdparty]`, `FF` = unchanged |
| `1C` | acquire controller (used by SDL / Steam) |
| `F0` | ADC calibration: `01` start, `02` finish |

## Disclaimer

THIS PROGRAM IS PROVIDAD AS-IS AND MAKES NO EXPRESS OR IMPLIED WARRANTY OF ANY KIND. I AM NOT RESPONSIBLE FOR ANY POSSIBLE DAMAGE CAUSED TO THE DEVICE OR ANY ACCESSORIES.
