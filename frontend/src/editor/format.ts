// Value formatting of the block editor, ported unchanged from the classic UI
// (cmd/hd500-edit/web/flow.js). Units and rounding were validated model by
// model against the pedal's LCD (webcam, 2026-11-08): keep them exact.
import type { Item, ModelGroup, Param, PresetView } from "../api";

/** A parameter as the API sends it (fields beyond the shared Param type). */
export interface EdParam extends Param {
  unit?: string;
  min?: number;
  max?: number;
  step?: number;
  whole?: boolean;
  options?: string[];
  tempoSlot?: number;
  sync?: number;
}

export type EdItem = Omit<Item, "params"> & { params?: EdParam[] };

export interface EdPreset extends Omit<PresetView, "items"> {
  items: EdItem[];
  cabs: { er: number; mic: number }[];
  variax?: { model: number; tone: number };
}

export interface AmpStyle { knob?: string; tolex?: string; grille?: string; panel?: string; trim?: string; layout?: string; grilleType?: string; speakers?: number; controls?: string; art?: string }

export type EdModelGroup = Omit<ModelGroup, "models"> & {
  models: { type: number; name: string; basedOn?: string; none?: boolean; bass?: boolean; style?: AmpStyle }[];
};

/**
 * An editable slider (classic flow.sliders[key]): value 0..1, steps (option
 * count for lists, else the unit steps, 0 = 1 %), options for a dropdown,
 * fmt(v) the shown text, send(v) the write.
 */
export interface SliderDesc {
  value: number;
  steps: number;
  options?: string[];
  fmt: (v: number) => string;
  send: (v: number) => void;
}

export const FS_LABEL = ["", "FS1", "FS2", "FS3", "FS4", "FS5", "FS6", "FS7", "FS8", "EXP"];
export const LANE_LABEL = ["Pre", "Path A pre", "Path B pre", "Path A post", "Path B post", "Post"];

// The pedal rounds for display (Gain 0.8753 shows 88, Low Cut 344.6 Hz 345).
export const pct = (v: number) => Math.round(v * 100);
export const listIndex = (p: { value: number; options?: string[] }) => Math.round(p.value * Math.max(1, (p.options?.length ?? 0) - 1));

/** ▲▼ / wheel steps of a range in its unit: 0.1 dB, 1 ms, 1 Hz, 0.1 kHz (or the catalog step); 0 = the default 1 %. */
export function rangeSteps(p: EdParam): number {
  const lo = p.min ?? 0, hi = p.max ?? 0;
  if (p.kind !== "range" || !(hi > lo) || !p.unit) return 0;
  const st = p.step || (p.unit === "dB" || p.unit === "kHz" ? 0.1 : 1);
  return Math.round((hi - lo) / st) + 1;
}

export function fmtParam(p: EdParam): string {
  const v = p.value;
  if (!Number.isFinite(v)) return "—";
  if (p.kind === "list" && p.options && p.options.length) return p.options[listIndex(p)] ?? "—";
  const lo = p.min ?? 0, hi = p.max ?? 0; // the API omits a 0 bound
  if (p.kind === "range" && hi > lo) {
    const x = lo + v * (hi - lo);
    // dB with one decimal (EQs "-11.5 dB"); a range with a step shows it in
    // those steps (Hard Gate: 1 dB, 0.32 of -96..0 = "-65.0").
    // Without a step, dB and kHz are cut toward zero like the LCD (Studio EQ
    // -2.86 → "-2.8", -4.68 → "-4.6"; Vintage Pre LPF 10.55 → "10.5 kHz");
    // Hz rounds (197.6 → "198 Hz").
    const cut = (u: number) => Math.trunc(u * 10 + Math.sign(u) * 1e-6) / 10 || 0;
    const xs = p.step ? (Math.trunc(x / p.step + Math.sign(x) * 1e-6) * p.step || 0) : // Hard Gate -35.52 → "-35.0" (LCD)
      p.unit === "dB" || p.unit === "kHz" ? cut(x) : x;
    const n = p.unit === "dB" ? xs.toFixed(p.whole ? 0 : 1) : p.unit === "ms" || p.unit === "Hz" ? String(Math.round(x)) : p.unit === "kHz" ? x.toFixed(1) : Math.abs(x) >= 100 ? x.toFixed(0) : x.toFixed(1);
    // dB above 0 with a "+" ("+3.5 dB"); also signed ranges without a unit (Pitch Glide "+6.2", as the LCD)
    const sign = (p.unit === "dB" || (!p.unit && lo < 0)) && Number(n) > 0 ? "+" : "";
    return p.unit ? `${sign}${n} ${p.unit}` : n;
  }
  return `${pct(v)} %`; // HX Edit: a space before %
}

// Mixer: level in dB (MIXER_MIN..+12, 0.5 dB steps) and pan -1..+1 (0.01 steps).
export const MIXER_MIN = -60, MIXER_MAX = 12;
/** "+3.5 dB", "MUTE" at the minimum. */
export const fmtDB = (x: number) => (x <= MIXER_MIN + 1e-3 ? "MUTE" : `${x > 0 ? "+" : ""}${x.toFixed(1)} dB`);
/** Pan "< 50 %" (HX: a space before %), "50 % >", centre "< 0 % >". */
export const fmtPan = (x: number) => {
  const p = Math.round(Math.abs(x) * 100);
  return p === 0 ? "< 0 % >" : x < 0 ? `< ${p} %` : `${p} % >`;
};

/** Tempo parameters in their unit: delay Time in ms, modulation/filter Speed in Hz. */
export const fmtTempo = (p: EdParam, v: number) => {
  const x = (p.min ?? 0) + v * ((p.max ?? 0) - (p.min ?? 0));
  return (p.max ?? 0) > 100 ? `${Math.round(x)} ms` : `${x.toFixed(2)} Hz`;
};

// Typed values (HX Edit: click the value, type, Enter). The text is matched
// against what the slider would display: an option by name for lists,
// otherwise the number (with "k" = ×1000) nearest to the typed one.
const num = (s: string) => {
  const m = String(s).replace(",", ".").match(/-?\d+(\.\d+)?/);
  if (!m) return NaN;
  return parseFloat(m[0]) * (/\d\s*k/i.test(s) ? 1000 : 1);
};

export function fracForText(d: SliderDesc, text: string): number | null {
  const t = text.trim().toLowerCase();
  if (!t) return null;
  const n = d.steps > 1 ? d.steps : 2001;
  const cands = Array.from({ length: n }, (_, i) => i / (n - 1));
  const exact = cands.find((c) => String(d.fmt(c)).trim().toLowerCase() === t);
  if (exact !== undefined) return exact;
  const want = num(t);
  if (Number.isNaN(want)) {
    const pre = cands.find((c) => String(d.fmt(c)).toLowerCase().startsWith(t));
    return pre === undefined ? null : pre;
  }
  let best: number | null = null, bd = Infinity;
  for (const c of cands) {
    const dv = Math.abs(num(d.fmt(c)) - want);
    if (dv < bd) { bd = dv; best = c; }
  }
  return best;
}

/** Block header tooltip: model · where · footswitch · based on. */
export const tipOf = (it: EdItem, where: string) =>
  [it.model, where, FS_LABEL[it.fs] ? `footswitch ${FS_LABEL[it.fs]}` : "", it.basedOn ? `based on ${it.basedOn}` : ""].filter(Boolean).join(" · ");
