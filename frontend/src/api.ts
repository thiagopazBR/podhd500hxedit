// Typed client for the editor's HTTP API (docs/API.md). The UI keeps no pedal
// logic: it reads State / PresetView and posts edits.

export interface Option { value: number; label: string; cc?: number }

export interface Tuner { on: boolean; signal: boolean; cents: number; note?: string; reference: number; mute: boolean }

export interface State {
  connected: boolean;
  error?: string;
  stalled?: boolean;
  setlist: number;
  setlistName?: string;
  slot: number;
  slotLabel?: string;
  presetName?: string;
  tempo: number;
  waitingTuner?: boolean;
  input1: number;
  input2: number;
  inz: number;
  inputsSetup: number;
  variaxControl: number;
  tempoSync: number;
  midiChannel: number;
  tuner?: Tuner;
  dirty?: boolean;
  loading?: { setlist: number; done: number; total: number; label?: string };
  dspAlert?: number;
  libraryRev: number;
  bufferRev: number;
}

export interface Meta {
  server?: string;
  input1: Option[];
  input2: Option[];
  tempo: { min: number; max: number };
  midiMessages?: Option[];
  midiChannels?: Option[];
  [key: string]: unknown;
}

/** MIDI Assign of one control (preset.midi): what it SENDS on MIDI OUT. See POST /api/midi-assign. */
export interface MidiAssign {
  control: number;
  name: string;
  pedal: boolean;
  message: number;
  channel: number;
  number: number;
  value: number;
  max: number;
}

export type LaneName = "pre" | "aPre" | "bPre" | "aPost" | "bPost" | "post";

export interface Lane { lane: number; name: LaneName; blocks: number[] }

export interface Chain { lanes: Lane[]; dual: boolean; ampPos: "pre" | "a" | "b" | "post" }

export interface Param { index: number; id: number; name: string; kind: string; value: number; lo: number; hi: number; ctl: number }

export interface Item {
  id: number;
  label: string;
  kind: "amp" | "cab" | "fx";
  type: number;
  model: string;
  basedOn?: string;
  group?: string;
  short?: string;
  empty: boolean;
  active: boolean;
  lane: number;
  fs: number;
  assign?: string;
  dspOver?: boolean;
  params?: Param[];
}

export interface PresetView {
  rev: number;
  name: string;
  items: Item[];
  mixer: { aLevel: number; bLevel: number; aPan: number; bPan: number };
  chain: Chain;
  midi?: MidiAssign[];
}

export interface ModelGroup { name: string; kind: "fx" | "amp" | "cab"; models: { type: number; name: string; basedOn?: string; none?: boolean }[] }

export async function apiError(res: Response): Promise<Error> {
  const text = (await res.text()).trim();
  return new Error(text || res.statusText);
}

export async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw await apiError(res);
  return res.json() as Promise<T>;
}

export async function post(url: string, body?: unknown): Promise<Response> {
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw await apiError(res);
  return res;
}
