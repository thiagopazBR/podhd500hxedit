// Parameters column of the block editor (classic renderParamsPage): the
// selected block's header bar (icon + model, power) and its parameters;
// cab, mixer, DSP-over and empty block pages. The input block: InputParams.
import { useEffect, useState, type CSSProperties, type ReactNode } from "react";
import { post } from "../../api";
import { useEditor } from "../../store";
import { CATS, catOf, type Cat } from "../../flow/icons";
import {
  LANE_LABEL, MIXER_MAX, MIXER_MIN, fmtDB, fmtPan, fmtParam, fmtTempo, listIndex, pct, rangeSteps, tipOf,
  type EdItem, type EdModelGroup, type EdParam, type EdPreset,
} from "../../editor/format";
import { CatIcon, PrmRow, ToggleRow } from "./PrmRow";
import { AmpIcon } from "./AmpIcon";
import { CabIcon } from "./CabArt";
import { InputParams } from "./InputParams";
import { queueSend } from "./sender";

const CLOCK = <svg viewBox="0 0 14 14" aria-hidden="true"><circle cx="7" cy="7" r="5.5" /><path d="M7 3.8V7l2.2 1.4" /></svg>;

/** Block header as HX Edit: icon + model (details in the tooltip) and the power button. */
export function BlockHead({ it, cat, tip, power = true }: { it: EdItem; cat: Cat; tip: string; power?: boolean }) {
  const { toast, patchPreset, models } = useEditor();
  // an amp or cab shows its own thumbnail (as in the models list) instead of the category glyph
  const style = it.empty || (it.kind !== "amp" && it.kind !== "cab") ? undefined
    : (models as EdModelGroup[]).filter((g) => g.kind === it.kind).flatMap((g) => g.models).find((m) => m.type === it.type)?.style;
  async function toggle() {
    if (it.empty || it.kind === "cab" || it.dspOver) return;
    const active = !it.active;
    const set = (a: boolean) => patchPreset((p) => ({ ...p, items: p.items.map((x) => (x.id === it.id ? { ...x, active: a } : x)) }));
    set(active);
    try { await post("/api/block", { id: it.id, active }); } catch (e) { set(!active); toast(`Couldn't turn ${active ? "on" : "off"} ${it.model}: ${(e as Error).message}`); }
  }
  return (
    <div className="ac-tabs">
      <div className="ac-title" style={{ "--cat": cat.color } as CSSProperties} title={tip}>{style ? (it.kind === "cab" ? <CabIcon style={style} /> : <AmpIcon style={style} />) : <CatIcon cat={cat} />}<span>{it.model}</span></div>
      {power && (
        <button className={`power${it.active ? " on" : ""}`} type="button" title={`${it.active ? "Turn off" : "Turn on"} (Space)`} onClick={toggle}>
          <svg viewBox="0 0 24 24"><path d="M12 3v9M6.3 6.3a8 8 0 1 0 11.4 0" /></svg>
        </button>
      )}
    </div>
  );
}

/** Tempo parameter (delay Time ms, mod/filter Speed Hz) with the tempo-sync clock (classic tempoSlider). */
function TempoRow({ blockId, p, extra }: { blockId: number; p: EdParam; extra?: ReactNode }) {
  const { meta, state, toast } = useEditor();
  const [sync, setSync] = useState(p.sync ?? 0);
  useEffect(() => setSync(p.sync ?? 0), [p.sync]);
  const notes = (meta?.syncNotes as string[] | undefined) || [];
  const n = notes.length - 2; // note codes 2..
  const isMs = (p.max ?? 0) > 100;
  const sendSync = (s: number) => queueSend("/api/param/sync", { id: blockId, slot: p.tempoSlot, sync: s }, toast);

  // The clock: free → the note nearest to the current time/speed at the
  // current BPM; synced → back to free (the stored time/speed is kept).
  function toggle() {
    let note = 0;
    if (!sync) {
      const beatsOf = (meta?.syncBeats as number[] | undefined) || [];
      const bpm = state?.tempo || 120;
      const x = (p.min ?? 0) + p.value * ((p.max ?? 0) - (p.min ?? 0));
      const beats = isMs ? (x * bpm) / 60000 : bpm / 60 / x; // ms → beats; Hz → beats per cycle
      let best = Infinity;
      beatsOf.forEach((bt, i) => {
        if (i < 2 || !bt) return;
        const d = Math.abs(Math.log(beats / bt));
        if (d < best) { best = d; note = i; }
      });
    }
    setSync(note);
    void sendSync(note);
  }

  const clock = (
    <button type="button" className={`prm-sync${sync ? " on" : ""}`} aria-pressed={!!sync} onClick={toggle}
      title={sync ? `Synced to tempo (click for ${isMs ? "ms" : "Hz"})` : "Sync to tempo"}>{CLOCK}</button>
  );
  if (sync >= 2 && n > 1) {
    // HX: a synced time reads "Note Sync" ("Left Time" → "Left Note Sync")
    const name = /Time$/.test(p.name) ? p.name.replace(/Time$/, "Note Sync") : p.name;
    return <PrmRow key="note" name={name} extra={extra} after={clock} desc={{
      value: (sync - 2) / (n - 1), steps: n, options: notes.slice(2, 2 + n),
      fmt: (f) => notes[2 + Math.round(f * (n - 1))] ?? "—",
      send: (f) => { const s = 2 + Math.round(f * (n - 1)); setSync(s); void sendSync(s); },
    }} />;
  }
  const unit = isMs ? 1 : 0.01; // arrows: 1 ms or 0.01 Hz
  return <PrmRow key="free" name={p.name} extra={extra} after={clock} desc={{
    value: p.value, steps: Math.round(((p.max ?? 0) - (p.min ?? 0)) / unit) + 1,
    fmt: (v) => fmtTempo(p, v),
    send: (v) => void queueSend("/api/param", { id: blockId, param: p.id, value: v }, toast),
  }} />;
}

function FxParams({ blockId, params }: { blockId: number; params?: EdParam[] }) {
  const { toast } = useEditor();
  if (!params || !params.length) return <span className="dt-meta">No parameters.</span>;
  // HX: if any parameter has a tempo-sync clock, every row narrows and the
  // clocks get a column of their own on the right.
  const sync = params.some((p) => p.kind === "tempo" && p.tempoSlot);
  const send = (p: EdParam) => (v: number) => void queueSend("/api/param", { id: blockId, param: p.id, value: v }, toast);
  return (
    <div className={`params${sync ? " has-sync" : ""}`}>
      {params.map((p, k) => {
        const key = `b${blockId}.${k}`;
        const ctl = p.ctl ? <> <span className="prm-ctl" title={`Controlled by expression pedal ${p.ctl}`}>EXP{p.ctl}</span></> : null;
        if (p.kind === "tempo" && p.tempoSlot) return <TempoRow key={key} blockId={blockId} p={p} extra={ctl} />;
        if (p.kind === "list" && p.options && p.options.length >= 2 && p.options.length <= 4) {
          const n = p.options.length;
          return <ToggleRow key={key} name={p.name} options={p.options} index={listIndex(p)} extra={ctl} onPick={(i) => send(p)(i / (n - 1))} />;
        }
        return <PrmRow key={key} name={p.name} extra={ctl} desc={{
          value: p.value,
          steps: p.kind === "list" && p.options ? p.options.length : rangeSteps(p),
          options: p.kind === "list" ? p.options : undefined,
          fmt: (v) => fmtParam({ ...p, value: v }),
          send: send(p),
        }} />;
      })}
    </div>
  );
}

/** Cab A (0) / B (1): mic and early reflections, then low cut, res level, thump, decay. */
function CabParams({ preset, cabIdx }: { preset: EdPreset; cabIdx: number }) {
  const { meta, toast } = useEditor();
  const cab = preset.items[cabIdx === 0 ? 1 : 3];
  const cv = preset.cabs[cabIdx];
  const mics = (meta?.mics as string[] | undefined) || [];
  const set = (param: string) => (x: number) => void queueSend("/api/cab", { cab: cabIdx, param, value: x }, toast);
  const names = ["lowcut", "res", "thump", "decay"];
  return (
    <div className="params">
      <PrmRow key={`c${cabIdx}.mic`} name="Mic" desc={{
        value: mics.length > 1 ? cv.mic / (mics.length - 1) : 0, steps: mics.length, options: mics,
        fmt: (x) => mics[Math.round(x * (mics.length - 1))] ?? "—",
        send: (x) => set("mic")(Math.round(x * (mics.length - 1))),
      }} />
      <PrmRow key={`c${cabIdx}.er`} name="Early Reflections" desc={{ value: cv.er, steps: 0, fmt: (x) => `${pct(x)} %`, send: set("er") }} />
      {(cab.params || []).slice(0, 4).map((p, k) => (
        <PrmRow key={`c${cabIdx}.${names[k]}`} name={p.name} desc={{
          value: p.value, steps: 0,
          fmt: (x) => (k === 0 ? `${Math.round(20 + x * 480)} Hz` : `${pct(x)} %`),
          send: set(names[k]),
        }} />
      ))}
    </div>
  );
}

function MixerRow({ preset, k, name }: { preset: EdPreset; k: "aLevel" | "bLevel" | "aPan" | "bPan"; name: string }) {
  const { toast } = useEditor();
  const level = k.endsWith("Level");
  const [lo, hi, step] = level ? [MIXER_MIN, MIXER_MAX, 0.5] : [-1, 1, 0.01];
  const toReal = (f: number) => Math.round((lo + f * (hi - lo)) / step) * step;
  return <PrmRow name={name} desc={{
    value: (preset.mixer[k] - lo) / (hi - lo),
    steps: Math.round((hi - lo) / step) + 1,
    fmt: (f) => (level ? fmtDB(toReal(f)) : fmtPan(toReal(f))),
    send: (f) => void queueSend("/api/mixer", { param: k, value: toReal(f) }, toast),
  }} />;
}

export function ParamsPage() {
  const { state, models, sel } = useEditor();
  const preset = useEditor().preset as EdPreset | null;
  const page = (cat: string | undefined, fill: string | undefined, body: ReactNode) => (
    <div className="ed-col ed-params" aria-live="polite" style={{ ...(cat ? { "--cat": cat } : {}), ...(fill ? { "--cat-fill": fill } : {}) } as CSSProperties}>{body}</div>
  );
  if (!state?.connected) return page(undefined, undefined, null);
  if (!preset || sel === null) return page(undefined, undefined, <span className="dt-meta">The parameters of the selected block appear here.</span>);
  if (sel === "input") return page("#c9cbd0", undefined, <InputParams />); // neutral, like the input block of the HX
  if (sel === "mixer") {
    return page(CATS.mixer.color, undefined, <>
      <div className="ac-tabs"><div className="ac-title" style={{ "--cat": CATS.mixer.color } as CSSProperties}><CatIcon cat={CATS.mixer} /><span>Mixer</span></div></div>
      <div className="params">
        <MixerRow preset={preset} k="aLevel" name="Volume A" />
        <MixerRow preset={preset} k="aPan" name="Pan A" />
        <MixerRow preset={preset} k="bLevel" name="Volume B" />
        <MixerRow preset={preset} k="bPan" name="Pan B" />
      </div>
    </>);
  }
  const it = preset.items[sel];
  if (it && it.dspOver) { // the pedal couldn't run this model
    const cat = catOf(it, models);
    return page(cat.color, undefined, <>
      <BlockHead it={it} cat={cat} tip={it.model} power={false} />
      <div className="dsp-note"><b>DSP LIMIT REACHED</b><span>This block is not running. Reduce usage to free up DSP resources.</span></div>
    </>);
  }
  if (!it || it.empty) return page(undefined, undefined, <div className="ac-tabs"><div className="ac-blank" /></div>); // HX: just the blank header bar
  const cat = catOf(it, models);
  if (it.kind === "amp") return page(cat.color, cat.fill, <><BlockHead it={it} cat={cat} tip={tipOf(it, it.id === 0 ? "Amp A" : "Amp B")} /><FxParams key={it.id} blockId={it.id} params={it.params} /></>);
  if (it.kind === "cab") { // no on/off: a cab is on while it has a model
    const cabIdx = it.id === 1 ? 0 : 1;
    return page(cat.color, cat.fill, <><BlockHead it={it} cat={cat} tip={tipOf(it, cabIdx === 0 ? "Cab A" : "Cab B")} power={false} /><CabParams preset={preset} cabIdx={cabIdx} /></>);
  }
  return page(cat.color, cat.fill, <><BlockHead it={it} cat={cat} tip={tipOf(it, `${it.label} · ${LANE_LABEL[it.lane] || ""}`)} /><FxParams key={it.id} blockId={it.id} params={it.params} /></>);
}
