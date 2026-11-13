# Live snapshots on a Raspberry Pi

The POD HD500 has no snapshots. This project adds four per preset: a snapshot is
the on/off state of the blocks plus the parameter values of the blocks the preset
already has (see [Snapshots in docs/API.md](API.md#snapshots) for exactly what is
stored). You create them in the editor on a computer; a Raspberry Pi next to the
pedal plays them when you press a footswitch.

```
pedal FS5..FS8 --MIDI OUT (DIN)--> USB-MIDI adapter --> Raspberry Pi --USB--> pedal
                 CC 80..83                              hd500-snap
```

`hd500-snap` listens for CC 80–83 (value 127) and applies snapshot 1–4 by sending
only the differences to the pedal's edit buffer. It never writes to the pedal's
flash and never changes a model. After a preset loads, it applies snapshot 1 by
itself (if the preset has a snapshot file).

## What you need

- A Raspberry Pi 3 or newer (tested: Pi 3 B, Raspberry Pi OS Lite 64-bit). The
  program is pure Go: `linux/arm64`, and `linux/arm` (GOARM 6/7) also build.
- A **proper power supply** (5 V, 2.5 A or more for a Pi 3). On a weak one the
  Pi throttles and drops USB: `vcgencmd get_throttled` must print `0x0`.
- A USB-MIDI adapter and a MIDI cable from the pedal's **MIDI OUT** to the adapter's input.
- The pedal's USB cable plugged into the Pi, not the computer: only one computer
  can talk to the pedal at a time (close the editor, or unplug it, first).

## One-time setup

1. **Author the snapshots on the computer** with the editor (Snapshot menu in the
   header: *Turn on snapshots*, set the sound, *Save* on a row). Use *Prepare
   footswitches* so FS5–FS8 send CC 80–83, and put the pedal's footswitches in
   the mode where FS5–FS8 toggle effects (check the LCD). The pedal keeps that
   MIDI Assign in the preset: save the preset to the pedal if you want to keep it.
2. **Build for the Pi** (on the computer, from the repository):
   `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o hd500-snap ./cmd/hd500-snap`
3. **Copy to the Pi:** the binary, `scripts/udev/99-hd500.rules`,
   `scripts/systemd/hd500-snap.service`, and the snapshot files from the
   editor's folder (`~/Library/Application Support/POD HD500 Edit X/snapshots/*.snapshots.json`),
   for example into `~/snap/`.
4. **USB permissions** (the program detaches the kernel driver from the pedal's
   control interface and talks to it through usbfs):
   `sudo cp 99-hd500.rules /etc/udev/rules.d/ && sudo udevadm control --reload && sudo udevadm trigger`
   (the rule gives the `plugdev` group access; the user must be in `plugdev` and `audio`).
5. **Find the adapter's name:** `./hd500-snap -midi-list`. The name differs per
   OS: the same cheap adapter showed as `CH345` on Linux.
6. **Try it:** `./hd500-snap -dir ~/snap -midi CH345` and press FS5/FS6. Each
   press logs `snapshot N: X change(s)`; repeating the same snapshot logs 0.
7. **Start at boot:** edit `User`, the paths and the `-midi` name in
   `hd500-snap.service`, then
   `sudo cp hd500-snap.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now hd500-snap`.
   Logs: `journalctl -u hd500-snap -f`.

## Flags

| Flag | Meaning |
|---|---|
| `-dir` | folder with the snapshot files (default: the editor's own folder) |
| `-midi name` | MIDI input whose name contains `name`; without it only the HTTP trigger works |
| `-midi-list` | list the MIDI inputs and exit |
| `-midi-cc 80` | CC of snapshot 1 (2–4 follow) |
| `-midi-channel 0` | MIDI channel 1–16, 0 = any |
| `-http 127.0.0.1:8600` | HTTP trigger: `POST /snapshot/1..4`, `GET /status`. It has **no authentication**, so the program refuses any address that is not loopback |
| `-settle` | wait after a preset loads before auto-applying snapshot 1 |

`HD500_USB_DEBUG=1` logs every USB packet; `HD500_MIDI_DEBUG=1` logs raw MIDI packets (macOS).

## Which preset does a snapshot file belong to?

One file per preset: `set<setlist>-<slot>.snapshots.json` (for example
`set7-2C.snapshots.json`), outside the pedal and outside `.h5e` files. The file
also records the preset's name; if the name at that slot differs (renamed,
moved, setlist reordered) the snapshots are not applied. Copy the files to the
Pi again after you change them in the editor.

## Troubleshooting

- `bulk OUT 0x01: timeout`: the pedal ignores the host. Power-cycle the pedal;
  make sure nothing else (the editor, another `hd500-snap`) holds the USB interface.
- `open /dev/bus/usb/...: permission denied`: the udev rule is missing or the
  user is not in `plugdev`.
- `no MIDI input matches`: run `-midi-list` and use a part of the name it prints.
- Random drops or `Undervoltage detected!` in `dmesg`: replace the power supply.
- The Pi cannot reach the computer on Wi-Fi: some routers isolate wired and
  wireless clients; put both on the same medium or disable client isolation.

## Status

Verified on a Raspberry Pi 3 B with a real POD HD500 (USB through usbfs, MIDI
through the ALSA raw device, systemd at boot). Pi Zero and other adapters are untested.
