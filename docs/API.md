# POD HD500 Edit X — HTTP API

The editor (`bin/hd500-edit`) is a local server on `http://127.0.0.1:8500`. All
pedal logic lives behind this API; the web UI in `cmd/hd500-edit/web` is only a
client and can be replaced. Requests and responses are JSON. Errors are plain
text with an HTTP status: `400` invalid request, `409` busy or conflict (for
example, the loaded preset has unsaved edits), `503` pedal offline.

## Concepts

- **Setlist** `0..7`, **slot** `0..63` (`slotLabel` `1A`…`16D`).
- **Edit buffer**: the preset loaded on the pedal. Changes to it are live and
  make the preset **dirty** (`state.dirty`). They are stored in the pedal's
  memory only by `POST /api/send`.
- **Pending changes**: changes to presets that are *not* loaded (rename, copy,
  move, import) stay in the editor until `POST /api/send`. `POST /api/receive`
  discards them.
- **Blocks** (`id`): `0` Amp A, `1` Cab A, `2` Amp B, `3` Cab B, `4..11` FX 1–8.
- **Lanes** (FX position in the signal flow): `pre` (before the A/B split),
  `aPre` / `bPre` (path A/B before the amp), `aPost` / `bPost` (path A/B after
  the amp), `post` (after the mixer). With a single amp, path B uses only
  `bPre`.
- **Parameter values** are normalized `0..1`, as stored by the pedal (lists:
  `index / (count - 1)`). The mixer uses real units (see below).

## Live state

| Method | Path | Body | Returns |
|---|---|---|---|
| GET | `/api/events` | — | Server-Sent Events; each message is a `State` |
| GET | `/api/state` | — | `State` |
| POST | `/api/refresh` | `?globals=1` reads only the global settings | `State` |
| GET | `/api/meta` | — | option lists: `input1`, `input2`, `inz`, `inputsSetup`, `mics`, `variaxModels` (by VDI Input type), `variaxControl`, `variaxToneMax`, `tempo {min,max}`, `setupInput` ids |

`State`: `connected`, `error`, `setlist`, `setlistName`, `slot`, `slotLabel`,
`presetName`, `tempo`, `input1`, `input2`, `inz`, `inputsSetup`, `variaxControl`, `tuner`, `dirty`,
`loading {setlist, done, total, label}`, `libraryRev` (the library changed:
re-read setlists), `bufferRev` (the loaded preset changed: re-read
`/api/preset`).

## Loaded preset (edit buffer)

| Method | Path | Body | Effect |
|---|---|---|---|
| GET | `/api/preset` | — | `PresetView` (below) |
| GET | `/api/models` | — | model browser: `[{name, kind: fx\|amp\|cab, models: [{type, name, basedOn, none}]}]` |
| POST | `/api/block` | `{id, active}` | turn a block on/off |
| POST | `/api/block/model` | `{id, type}` | change a block's model. For amps: the amp always arrives enabled with its cab; `0x0007FFFF` = *Amp Disabled*. Cab `0x0107FFFF` = *No Cab*. FX `0x020DFFFF` = empty slot |
| POST | `/api/param` | `{id, param, value}` | set a block parameter (`param` = parameter id from `PresetView`, `value` 0..1) |
| POST | `/api/cab` | `{cab: 0\|1, param, value}` | cab A/B: `mic` (index 0..7 into `meta.mics`), `er`, `lowcut`, `res`, `thump`, `decay` (0..1; Low Cut = 20 + v·480 Hz) |
| POST | `/api/mixer` | `{param, value}` | `aLevel` / `bLevel` in dB (−60 = MUTE … +12, 0.5 dB steps), `aPan` / `bPan` −1 (left) … +1 (right) |
| POST | `/api/chain/move` | `{block, lane, before?}` | move FX block `4..11` into `lane`, before block `before` (which must be in that lane) or at the end. Returns the new `Chain` |
| POST | `/api/amp/move` | `{pos}` | move amp A and its cab: `pre` (end of the pre section), `a` (the paths: amp A on path A, amp B slot on path B) or `post` (start of the post section). As on the pedal, `pre`/`post` clear amp B (back on the paths it is Amp Disabled); amp B never moves. A single amp on path B = amp A set to None with an amp B model. Returns the new `Chain` |
| POST | `/api/assign` | `{id, fs}` · `{id, param, ctl}` · `{id, param, min}` · `{id, param, max}` | Bypass/controller assignments of block `id` (not cabs). `fs`: bypass footswitch (`meta.bypassAssign`: 0 None, 1–8 FS1–FS8, 9 EXP Toe Switch). `param` = index in the block's `params`; `ctl`: its controller (`meta.controllers`: 0 None, 1 EXP 1, 2 EXP 2, 3 Variax Vol, 4 Variax Tone); `min`/`max`: heel/toe value 0..1 (item `lo`/`hi`). `fs`/`ctl` rewrite the edit buffer; `min`/`max` are sent as Cmd 0x2E/0x2F. 204 on success. The option lists carry `cc`: the fixed MIDI CC of that source (FS1–FS8 = 51–58, EXP Toe = 59, EXP 1/2 = 1/2; none for the Variax) |
| POST | `/api/assign/clear` | `{all: true}` · `{id, param}` | Clear Assignment (`param` −1 = the bypass footswitch) or Clear All Assignments of the preset, in one buffer rewrite. 204 |
| POST | `/api/tempo` | `{bpm}` | 30..240 |
| POST | `/api/input` | `{id, value}` | preset inputs (ids in `meta.setupInput`) |
| POST | `/api/inputs-setup` | `{value}` | global: 1 Global, 0 Preset |
| POST | `/api/tempo-sync` | `{value}` | pedal-wide Tempo Sync: `1` Global (one tempo for every preset), `0` Preset (STATUS 0x0B). `state.tempoSync`; with Global, `state.tempo` is the pedal-wide tempo as last seen (it can be written with `/api/tempo` but not read: `0` = not known yet) |
| POST | `/api/midi-channel` | `{channel}` | pedal-wide MIDI channel 1–16 (STATUS 0x0D), the channel MIDI IN listens on; `state.midiChannel` |
| POST | `/api/midi-assign` | `{control, message?, channel?, number?, value?, max?}` | MIDI Assign (what a control SENDS on MIDI OUT) of the loaded preset. `control` 0–12, in the order of the pedal's MIDI ASSIGN screen read row by row: FS1–FS4, Looper, EXP Toe, FS5–FS8, Tap, EXP 1, EXP 2 (`preset.midi`; each item carries its `name`). `message` (switches, `meta.midiMessages`): 0 CC Toggle, 1 CC, 2 Prog Change, 3 Bank Change, 4 Note On. `channel` (`meta.midiChannels`): 16 None (sends nothing), 17 Base, 0–15 = 1–16. `number` CC/program/bank/note 0–127; `value` value/velocity (pedals: Min); `max` pedals' Max. Sent as Cmd 0x34. 204 |
| POST | `/api/variax` | `{param, value}` | Variax: `model` (index into `meta.variaxModels["300/600 500/700"]`, 0 = Don't Force), `tone` (0..127; the pedal shows it truncated, 63 = 49%), `control` (global Variax Control: 1 Global, 0 Preset) |
| POST | `/api/tuner` | any of `{on, reference, mute}` | the pedal's tuner (global STATUS 0x00 on/off; 0x02 `reference` 425..455 Hz; 0x01 `mute` true = Mute, false = Bypass; settings read back). While on, `state.tuner` streams the reading ~5×/s: `{on, signal, note: "E", octave: 2, cents, reference, mute}` (`signal` false = no note). With the tuner open the pedal doesn't answer preset reads: the editor reads again when it closes |
| GET | `/api/globals` | — | raw global settings 0x00..0x2F (diagnostics) |

`PresetView`:

```json
{
  "rev": 12,
  "name": "R-Billyish",
  "items": [ { "id": 0, "label": "Amp A", "kind": "amp", "type": 458757,
               "model": "Blackface Double Normal", "basedOn": "...", "group": "",
               "empty": false, "active": true, "pos": 5, "lane": 7, "fs": 0,
               "style": {"...": "colours for drawing"},
               "params": [ { "index": 0, "id": 1058013187, "name": "Drive",
                             "kind": "percent", "value": 0.51, "lo": 0.51,
                             "hi": 0.51, "ctl": 0, "options": [], "min": 0, "max": 0 } ] } ],
  "mixer": { "aLevel": 0, "bLevel": 0, "aPan": -1, "bPan": 1 },
  "cabs":  [ { "er": 0.12, "mic": 3 }, { "er": 0.4, "mic": 0 } ],
  "variax": { "model": 10, "tone": 127 },
  "chain": { "dual": false, "ampPos": "pre",
             "lanes": [ { "lane": 0, "name": "pre", "blocks": [5, 4, 6] },
                        { "lane": 2, "name": "bPre", "blocks": [8] },
                        { "lane": 5, "name": "post", "blocks": [7, 9, 10, 11] } ] }
}
```

`chain.lanes` always has the 6 lanes, in lane order. Each lane lists its FX
blocks in signal order, including empty slots. `chain.ampPos` is where the
amp sits: `pre` (end of the pre section), `a`/`b` (path A/B), `post` (start
of the post section); always `a` with two amps. Read from amp A's position
word: 5 = pre, 7 = post (both LCD-verified), 0 = path A. A single amp on path B
is stored as two amps with amp A disabled (`dual: true`, `ampPos: "a"`). `fs`: 0 none, 1–8 FS1–FS8,
9 EXP toe switch. `ctl`: 0 none, 1 EXP 1, 2 EXP 2.

Items: `dspOver` (FX) = a model the pedal couldn't run because the DSP limit was reached (model kept, not in use; LCD "DSP OVER"). `state.dspAlert` counts the pedal's "DSP limit reached" messages (Cmd 0x33); show the warning when it changes.

Notes on the pedal's behaviour:
- Reading the edit buffer and rewriting it whole (model change, Amp Disabled,
  chain move) shows "SYNC IN PROGRESS" on the LCD for a moment. The last two
  also move the LCD cursor to another block. Parameter, on/off, cab and mixer
  changes don't.
- Values are shown by the pedal rounded (e.g. 0.875 → 88%).

## Library (stored presets)

| Method | Path | Body | Effect |
|---|---|---|---|
| GET | `/api/setlists` | — | `[{index, name, stored, edited}]` |
| GET | `/api/setlist?n=` | — | `{setlist, loaded, names[64], edited[64]}`; starts reading the setlist if needed (progress in `state.loading`) |
| POST | `/api/select` | `{setlist, slot}` | load a preset on the pedal |
| POST | `/api/rename` | `{setlist, slot, name}` | rename (loaded preset: live; others: pending) |
| POST | `/api/setlist-name` | `{setlist, name}` | rename a setlist (pending) |
| POST | `/api/arrange` | `{op: move\|copy\|swap, from: [{setlist, slot}], to: {setlist, slot}}` | rearrange presets (pending) |
| POST | `/api/send` | `{scope: selected\|setlist\|all, setlist, slots}` | write changed presets (and pending setlist names) to the pedal, verified. Returns `{done, nothing, failed}` |
| POST | `/api/receive` | same | re-read from the pedal, discarding pending changes |

## Files

| Method | Path | Body | Effect |
|---|---|---|---|
| GET | `/api/export?kind=preset\|setlist\|bundle&setlist=&slot=` | — | `.h5e` / `.h5s` / `.h5b` download (editor's version, pending changes included) |
| POST | `/api/import` | multipart: `file` (one or more), `setlist`, `slot` | import into the editor (pending). Returns `{kind, presets, same, setNames, renamed, hd500x}` |
| POST | `/api/backup` | — | re-read all 512 presets and save a `.h5b` in `~/Documents/POD HD500 Edit X/Backups`. Returns `{path, presets}` |
| POST | `/api/reveal` | `{path}` | show a backup in the Finder |
