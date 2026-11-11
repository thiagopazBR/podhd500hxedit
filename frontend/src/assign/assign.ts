// Bypass / controller assignments (HX Edit "BYPASS/CONTROLLER ASSIGN"): the
// rules and API calls behind the tab, with no UI. Ported from the classic UI
// (cmd/hd500-edit/web/flow.js: assignmentsOf, the option lists of renderAuto,
// postAssign, clearAssign). Backend side: the UI only renders what these return.
import { post, type Item, type Meta, type Option, type PresetView } from "../api";

/** Blocks that can take an assignment (cabs follow their amp). */
export const ASSIGN_BLOCKS = [0, 2, 4, 5, 6, 7, 8, 9, 10, 11];

/** Parameter index of the Bypass footswitch row. */
export const BYPASS = -1;

/** The "MIDI CC" entry of a source menu: shows the CC, sends nothing. */
export const MIDI_OPT = -100;

export type SortCol = 0 | 1 | 2; // Block, Parameter, Source

export interface AssignRow {
  id: number; // block id
  k: number; // parameter index, or BYPASS
  param: string;
  source: string;
}

/** A source option; `label` carries the users in brackets, `bar` the bare name. */
export interface SourceOption extends Option {
  bar?: string;
}

const optionsOf = (meta: Meta | null, key: string): Option[] => (meta?.[key] as Option[] | undefined) ?? [];

const labelOf = (opts: Option[], v: number) => opts.find((o) => o.value === v)?.label ?? "—";

/** Every assignment of the preset, one row each (bypass and each controller). */
export function assignmentsOf(view: PresetView, meta: Meta | null): AssignRow[] {
  const rows: AssignRow[] = [];
  for (const id of ASSIGN_BLOCKS) {
    const it = view.items[id];
    if (!it || it.empty) continue;
    if (it.fs) rows.push({ id, k: BYPASS, param: "Bypass", source: labelOf(optionsOf(meta, "bypassAssign"), it.fs) });
    for (const p of it.params ?? []) {
      if (p.ctl) rows.push({ id, k: p.index, param: p.name, source: labelOf(optionsOf(meta, "controllers"), p.ctl) });
    }
  }
  return rows;
}

/** The rows sorted by a column; dir 1 ascending, -1 descending. */
export function sortRows(rows: AssignRow[], view: PresetView, col: SortCol, dir: 1 | -1): AssignRow[] {
  const keyOf = (r: AssignRow) => [view.items[r.id].model, r.param, r.source][col];
  return [...rows].sort((a, b) => dir * keyOf(a).localeCompare(keyOf(b), undefined, { numeric: true, sensitivity: "base" }));
}

/** The block's parameter menu: Bypass, then each parameter. */
export function assignOptions(it: Item): Option[] {
  return [{ value: BYPASS, label: "Bypass" }, ...(it.params ?? []).map((p) => ({ value: p.index, label: p.name }))];
}

/** The footswitch (bypass) or controller menu. Each entry lists, in brackets,
 *  what already uses that source in this preset. */
export function sourceOptions(view: PresetView, meta: Meta | null, isBypass: boolean): SourceOption[] {
  return optionsOf(meta, isBypass ? "bypassAssign" : "controllers").map((o) => {
    if (!o.value) return o; // None
    const who: string[] = [];
    for (const bid of ASSIGN_BLOCKS) {
      const b = view.items[bid];
      if (!b || b.empty) continue;
      if (isBypass) {
        if (b.fs === o.value) who.push(b.model);
      } else {
        for (const q of b.params ?? []) if (q.ctl === o.value) who.push(`${b.model}: ${q.name}`);
      }
    }
    return who.length ? { ...o, label: `${o.label} [${who.join(", ")}]`, bar: o.label } : o;
  });
}

/** The fixed MIDI CC of a source (FS1–FS8, EXP Toe Switch, EXP 1/2), if it has one. */
export function sourceCc(meta: Meta | null, isBypass: boolean, value: number): number | undefined {
  return optionsOf(meta, isBypass ? "bypassAssign" : "controllers").find((o) => o.value === value)?.cc;
}

// ---- writes (POST /api/assign, /api/assign/clear, /api/midi-channel). The caller refetches the preset.

let nextMidiChannel: number | null = null;
let midiChannelBusy = false;

/** Pedal-wide MIDI channel (1..16). Writes are coalesced: one in flight, newest wins. */
export async function setMidiChannel(channel: number): Promise<void> {
  nextMidiChannel = channel;
  if (midiChannelBusy) return;
  midiChannelBusy = true;
  try {
    while (nextMidiChannel !== null) {
      const ch = nextMidiChannel;
      nextMidiChannel = null;
      await post("/api/midi-channel", { channel: ch });
    }
  } finally {
    midiChannelBusy = false;
  }
}

/** Bypass footswitch of a block (0 = None). */
export const setBypassSource = (id: number, fs: number) => post("/api/assign", { id, fs });

/** Controller of a parameter (0 = None). */
export const setControllerSource = (id: number, param: number, ctl: number) => post("/api/assign", { id, param, ctl });

/** Heel (min) or toe (max) value of a controlled parameter, 0..1. */
export const setRange = (id: number, param: number, which: "min" | "max", value: number) =>
  post("/api/assign", { id, param, [which]: value });

/** Clear one assignment (param BYPASS clears the footswitch). */
export const clearOne = (id: number, param: number) => post("/api/assign/clear", { id, param });

/** Clear every assignment of the preset. */
export const clearAll = () => post("/api/assign/clear", { all: true });
