# podhd500hxedit

An editor for the **Line 6 POD HD500**, with an HX Edit–style interface, for macOS.

Line 6 no longer maintains the official POD HD500 Edit, and its driver doesn't work
on current macOS / Apple Silicon. This project talks to the pedal directly over USB
and runs as a small local web app: one Go binary that serves the UI on
`http://127.0.0.1:8500/`.

> ⚠️ **Status: work in progress, use at your own risk.** Edits go to the pedal's edit
> buffer live; *Send / Save to Pedal* writes its memory. **Back up your presets first**
> (the editor's Export / Backup). If the pedal stops responding (the connection light
> turns red), power it off and on.

## Features

- Signal flow: drag blocks, amp positions (Pre / paths / Post), dual amps, on/off,
  footswitch and EXP labels, DSP-limit warnings
- All amps, cabs and effects with their parameters, units matching the pedal's LCD
  (ms, Hz, dB, note sync), validated model by model against the LCD
- Inputs, Variax (300/600, 500/700), mixer, tuner, tap tempo and Tempo Sync
- Bypass / Controller Assign (footswitches, EXP 1/2, Variax knobs, MIDI CCs, MIDI channel)
- MIDI Assign window (what each control sends on MIDI OUT)
- Setlists and presets: load, rename, copy/move, send/receive, import/export
  `.h5e` / `.h5s` / `.h5b`, backups
- An HTTP API with everything the UI does: see [docs/API.md](docs/API.md)

## Requirements

- macOS (Apple Silicon or Intel)
- POD HD500 connected by USB (the Line 6 driver may stay installed for audio)

## Build and run

```sh
scripts/build.sh            # vet, tests and binaries in bin/
sudo ./bin/l6-rebind release  # after each power-on / replug: frees the pedal's control interface
./bin/hd500-edit              # then open http://127.0.0.1:8500/
```

`l6-rebind release` detaches the Line 6 kernel driver from the pedal's USB control
interface only; audio keeps working. Replugging the pedal restores the original state.

Set `HD500_DEBUG=1` to log every message the pedal sends (useful to learn new parts
of the protocol).

## Layout

| Path | What |
|---|---|
| `cmd/hd500-edit` | the editor: HTTP API + embedded web UI (`web/`) |
| `cmd/l6-rebind` | frees the pedal's USB interface from the Line 6 driver |
| `pkg/usb` | USB transport and the pedal's message format |
| `pkg/podhd500` | preset format, model catalog, files |
| `pkg/editor` | session: state, events, edits |
| `docs/API.md` | API reference |

## Acknowledgements

Built with the help of the open work of
[hd500-editor](https://github.com/linepix-idev/hd500-editor), [lpedit-lib](https://github.com/StarAurryon/lpedit-lib),
podhd and the Linux POD HD driver. See [NOTICE](NOTICE) for licenses of included data.

## License

MIT — see [LICENSE](LICENSE). Some data files carry their own licenses: see [NOTICE](NOTICE).

Line 6, POD and HX Edit are trademarks of Yamaha Guitar Group, Inc. This project is
not affiliated with or endorsed by Line 6.
