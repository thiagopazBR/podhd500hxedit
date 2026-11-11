// The input block's parameters (classic inputParamsHTML / variaxParamsHTML).
// In the HD500, In-Z and Inputs Setup are one setting for both inputs, so
// there is a single "Input" page; with the Variax in use, Input | Variax tabs.
// Writes: /api/input, /api/inputs-setup, /api/variax.
import { useState, type CSSProperties } from "react";
import { post, type Option } from "../../api";
import { useEditor } from "../../store";
import { GUITAR_BODY, inputIcon } from "../../flow/icons";
import type { EdPreset } from "../../editor/format";
import { PrmRO, PrmRow, SetupSwitch } from "./PrmRow";
import { queueSend } from "./sender";

export const InputSvg = () => <svg viewBox="0 0 24 24" aria-hidden="true"><path d={GUITAR_BODY} transform="rotate(40 12 12)" /></svg>;
const VARIAX_PATH = "M5 6l7 12 7-12";

// The VDI Input types of the original editor, in its order.
const VDI_TYPES = ["300/600 500/700", "Tyler Variax", "Acoustic 700", "Bass 700", "Bass 705"];
// Tyler Variax / Acoustic 700 options of the original editor: DISPLAY ONLY
// (how the pedal stores them is still unknown), nothing is sent.
const TYLER_LOCAL = ["Unlocked", "Volume Lock", "Tone Lock", "Vol+Tone Lock", "Vol+Tone+Togl Lock"];
const TYLER_STRINGS = [["6", "E"], ["5", "A"], ["4", "D"], ["3", "G"], ["2", "B"], ["1", "E"]];

// VDI Input: a display preference kept in this browser (the pedal doesn't store it).
function loadVdi() {
  try { return localStorage.getItem("hd500.vdiInput") || VDI_TYPES[0]; } catch { return VDI_TYPES[0]; }
}

export function InputParams() {
  const { state, meta, toast } = useEditor();
  const preset = useEditor().preset as EdPreset | null;
  const [tab, setTab] = useState<"input" | "variax">("input");
  const [vdiPref, setVdiPref] = useState(loadVdi);
  if (!state || !meta) return null;
  const opts = (key: string) => (meta[key] as Option[] | undefined) || [];
  const label = (key: string, v: number) => opts(key).find((o) => o.value === v)?.label ?? "—";
  const setupId = (meta.setupInput as Record<string, number> | undefined) || {};
  const fail = (what: string) => (e: Error) => toast(`Couldn't change ${what}: ${e.message}`);

  const optSlider = (key: "inz" | "input2", name: string) => {
    const o = opts(key);
    const idx = Math.max(0, o.findIndex((x) => x.value === state[key]));
    const n = Math.max(1, o.length - 1);
    return <PrmRow key={`in.${key}`} name={name} desc={{
      value: idx / n, steps: o.length, options: o.map((x) => x.label),
      fmt: (f) => o[Math.round(f * n)]?.label || "—",
      send: (f) => { const x = o[Math.round(f * n)]; if (x) void queueSend("/api/input", { id: setupId[key], value: x.value }, toast); },
    }} />;
  };

  const variax = /variax/i.test(label("input1", state.input1)) || /variax/i.test(label("input2", state.input2));
  const cur = variax ? tab : "input";
  const src = label("input1", state.input1);
  const ic = inputIcon(src);
  const white = { "--cat": "#f2f3f5" } as CSSProperties;
  // Header bar as the other blocks (HX: icon + the source's name); with the
  // Variax in use, two tabs "Input" and "Variax".
  const head = variax ? (
    <div className="ac-tabs" role="tablist" style={white}>
      <button type="button" className={`ac-tab${cur === "input" ? " cur" : ""}`} role="tab" aria-selected={cur === "input"} title={`Input 1 Source: ${src}`} onClick={() => setTab("input")}><InputSvg /><span>Input</span></button>
      <button type="button" className={`ac-tab${cur === "variax" ? " cur" : ""}`} role="tab" aria-selected={cur === "variax"} onClick={() => setTab("variax")}><svg viewBox="0 0 24 24" aria-hidden="true"><path d={VARIAX_PATH} /></svg><span>Variax</span></button>
    </div>
  ) : (
    <div className="ac-tabs"><div className="ac-title" style={white} title={`Input 1 Source: ${src}`}><svg viewBox="0 0 24 24" aria-hidden="true"><path d={ic.d} transform={ic.tf} /></svg><span>{src}</span></div></div>
  );

  const variaxControl = (
    <div className="params">
      <SetupSwitch name="Variax Control" opts={opts("variaxControl")} cur={state.variaxControl}
        onPick={(value) => post("/api/variax", { param: "control", value }).catch(fail("Variax Control"))} />
    </div>
  );

  if (variax && cur === "variax") {
    const vx = preset?.variax || { model: 0, tone: 0 };
    const vdi = VDI_TYPES.includes(vdiPref) ? vdiPref : VDI_TYPES[0];
    const known = (meta.variaxModels as Record<string, string[]> | undefined)?.[vdi] || null;
    // A type without a known list (Bass 700/705) shows the raw index, 0..110.
    const models = known || Array.from({ length: 111 }, (_, i) => (i === 0 ? "(Don't Force)" : `Model ${i}`));
    const nm = Math.max(1, models.length - 1);
    const tmax = (meta.variaxToneMax as number | undefined) || 127;
    const nv = VDI_TYPES.length - 1;
    const vdiRow = (
      <div className="params">
        <PrmRow key="vx.vdi" name="VDI Input" desc={{
          value: VDI_TYPES.indexOf(vdi) / nv, steps: VDI_TYPES.length, options: VDI_TYPES,
          fmt: (f) => VDI_TYPES[Math.round(f * nv)],
          send: (f) => { const t = VDI_TYPES[Math.round(f * nv)]; try { localStorage.setItem("hd500.vdiInput", t); } catch { /* not remembered */ } setVdiPref(t); },
        }} />
      </div>
    );
    const unknown = known ? null : <p className="in-note in-pending">The model list for this type is not known yet: Model shows the number stored in the pedal.</p>;
    const toneText = (f: number) => `${Math.floor(f * 100 + 1e-6)} %`; // the pedal truncates: 63/127 shows 49%
    // Only the 300/600 500/700 is verified on the pedal; for the other types
    // Model and Tone are DISPLAY ONLY (a change with Acoustic 700 selected
    // jammed the USB link, 2026-11-06).
    if (vdi !== VDI_TYPES[0]) {
      return <>
        {head}{vdiRow}{unknown}
        <p className="in-note in-pending"><span className="off-tag">Disabled</span> {vdi}: not working yet. Model and Tone only show what is in the preset; nothing is sent to the pedal.</p>
        <div className="params params-pending" aria-disabled="true">
          <PrmRO name="Model" value={models[vx.model] || `Model ${vx.model}`} frac={vx.model / nm} />
          <PrmRO name="Tone" value={toneText(vx.tone / tmax)} frac={vx.tone / tmax} />
        </div>
        {vdi === "Tyler Variax" && <>
          <p className="in-note in-pending"><span className="off-tag">Disabled</span> Tyler Variax options: not working yet (the codes the pedal uses for them are unknown).</p>
          <div className="params params-pending" aria-disabled="true">
            <PrmRO name="Local Control" value={TYLER_LOCAL[0]} frac={0} />
            <PrmRO name="Variax Tuning" value="Don't Force" frac={0} />
            {TYLER_STRINGS.map(([n, note]) => <PrmRO key={n} name={`String ${n} (${note})`} value="—" frac={0} />)}
          </div>
        </>}
        {vdi === "Acoustic 700" && <>
          <p className="in-note in-pending"><span className="off-tag">Disabled</span> Acoustic 700 options: not working yet (the codes the pedal uses for them are unknown).</p>
          <div className="params params-pending" aria-disabled="true"><PrmRO name="Compression" value="0 %" frac={0} /><PrmRO name="Mic" value="0 %" frac={0} /></div>
        </>}
        {variaxControl}
      </>;
    }
    return <>
      {head}{vdiRow}{unknown}
      <div className="params params-gap">
        <PrmRow key="vx.model" name="Model" desc={{
          value: vx.model / nm, steps: models.length, options: models,
          fmt: (f) => models[Math.round(f * nm)] || "—",
          send: (f) => void queueSend("/api/variax", { param: "model", value: Math.round(f * nm) }, toast),
        }} />
        <PrmRow key="vx.tone" name="Tone" desc={{
          value: vx.tone / tmax, steps: tmax + 1, fmt: toneText,
          send: (f) => void queueSend("/api/variax", { param: "tone", value: Math.round(f * tmax) }, toast),
        }} />
      </div>
      {variaxControl}
    </>;
  }

  const setupOpts = opts("inputsSetup").slice().sort((a, b) => b.value - a.value); // Global, Preset
  return <>
    {head}
    <div className="params">
      {optSlider("inz", "Guitar In-Z")}
      {optSlider("input2", "Input 2 Source")}
      <SetupSwitch name="Inputs Setup" opts={setupOpts} cur={state.inputsSetup} onPick={(value) => post("/api/inputs-setup", { value }).catch(fail("Inputs Setup"))} />
    </div>
    {state.inputsSetup === 1 && <p className="in-note">Inputs Setup is Global: these inputs apply to all presets and are stored in the pedal settings, not in this preset.</p>}
  </>;
}
