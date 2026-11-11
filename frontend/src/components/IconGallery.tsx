// Development page (/next/#icons): every category icon as a chain block,
// on / off / selected, at the signal flow's scale and 3× — to refine the
// icons without touching the pedal.
import { CATS, catGlyph, type Cat } from "../flow/icons";
import { BOX } from "../flow/layout";
import { ICON_SIZE } from "./SignalFlow";
import { useEffect, useState } from "react";
import { AmpIcon } from "./editor/AmpIcon";
import { AmpArt } from "./editor/AmpArt";
import { CabArt, CabIcon } from "./editor/CabArt";
import { MidiWindow, type MidiAssignItem } from "./MidiWindow";
import type { EdModelGroup as ModelGroup } from "../editor/format";

const SAMPLE_MIDI: MidiAssignItem[] = [
  { control: 0, name: "FS1", pedal: false, message: 0, channel: 0, number: 51, value: 127, max: 0 },
  { control: 1, name: "FS2", pedal: false, message: 1, channel: 17, number: 52, value: 64, max: 0 },
  { control: 2, name: "FS3", pedal: false, message: 2, channel: 1, number: 12, value: 0, max: 0 },
  { control: 3, name: "FS4", pedal: false, message: 1, channel: 16, number: 0, value: 0, max: 0 },
  { control: 4, name: "Looper", pedal: false, message: 0, channel: 17, number: 60, value: 127, max: 0 },
  { control: 5, name: "EXP Toe", pedal: false, message: 0, channel: 0, number: 59, value: 127, max: 0 },
  { control: 6, name: "FS5", pedal: false, message: 3, channel: 0, number: 2, value: 0, max: 0 },
  { control: 7, name: "FS6", pedal: false, message: 4, channel: 9, number: 60, value: 100, max: 0 },
  { control: 8, name: "FS7", pedal: false, message: 1, channel: 16, number: 0, value: 0, max: 0 },
  { control: 9, name: "FS8", pedal: false, message: 1, channel: 16, number: 0, value: 0, max: 0 },
  { control: 10, name: "Tap", pedal: false, message: 1, channel: 16, number: 64, value: 127, max: 0 },
  { control: 11, name: "EXP 1", pedal: true, message: 0, channel: 0, number: 1, value: 0, max: 127 },
  { control: 12, name: "EXP 2", pedal: true, message: 0, channel: 16, number: 2, value: 0, max: 127 },
];

/** Every amp thumbnail (list size and 4×), from /api/models. */
function Cabs() {
  const [groups, setGroups] = useState<ModelGroup[]>([]);
  useEffect(() => { fetch("/api/models").then((r) => r.json()).then(setGroups).catch(() => {}); }, []);
  const cabs = groups.filter((g) => g.kind === "cab").flatMap((g) => g.models).filter((m) => !m.none && m.style);
  return (
    <div className="ig-amps">
      {cabs.map((m) => (
        <div key={m.type} className="ig-amp">
          <CabArt style={m.style!} className="ig-amp-big" />
          <span className="ig-small"><CabIcon style={m.style!} /></span>
          <span>{m.name}</span>
        </div>
      ))}
    </div>
  );
}

function Amps() {
  const [groups, setGroups] = useState<ModelGroup[]>([]);
  useEffect(() => { fetch("/api/models").then((r) => r.json()).then(setGroups).catch(() => {}); }, []);
  const amps = groups.filter((g) => g.kind === "amp").flatMap((g) => g.models).filter((m) => !m.none && m.style);
  const seen = new Set<string>();
  const uniq = amps.filter((m) => { const k = JSON.stringify(m.style); if (seen.has(k)) return false; seen.add(k); return true; });
  return (
    <div className="ig-amps">
      {uniq.map((m) => (
        <div key={m.type} className="ig-amp">
          <AmpArt style={m.style!} className="ig-amp-big" />
          <AmpIcon style={m.style!} className="ig-amp-small" />
          <span>{m.name}</span>
        </div>
      ))}
    </div>
  );
}

const H = BOX / 2;
const NAMES = ["Distortion", "Dynamics", "EQ", "Modulation", "Delay", "Reverb", "Pitch", "Filter", "Wah", "Volume/Pan", "FX Loop", "amp", "preamp", "cab", "mixer"];

function Block({ cat, x, on, sel, label }: { cat: Cat; x: number; on: boolean; sel?: boolean; label: string }) {
  const g = catGlyph(cat);
  const iz = cat.svg ? ICON_SIZE : 18;
  return (
    <g className={`blk${on ? " on" : ""}${sel ? " sel" : ""}`} style={{ "--cat": cat.color } as React.CSSProperties}>
      <rect className="blk-sel" x={x - H - 5} y={30 - H - 5} width={BOX + 10} height={BOX + 10} rx={9} />
      <rect className="blk-box" x={x - H} y={30 - H} width={BOX} height={BOX} rx={3} />
      <svg className={"blk-icon" + g.cls} x={x - iz / 2} y={30 - iz / 2} width={iz} height={iz} viewBox={g.vb} dangerouslySetInnerHTML={{ __html: g.inner }} />
      <text className="blk-fs blk-ac" x={x} y={30 + H + 13}>{label}</text>
    </g>
  );
}

function Row({ scale, state }: { scale: number; state: "on" | "off" | "sel" }) {
  const w = 30 + NAMES.length * 58;
  return (
    <svg className="chain" viewBox={`0 0 ${w} 62`} width={w * scale} height={62 * scale}>
      <g className="wires"><path d={`M10 30H${w - 10}`} /></g>
      {NAMES.map((n, i) => <Block key={n} cat={CATS[n]} x={40 + i * 58} on={state !== "off"} sel={state === "sel"} label={n.replace("Volume/Pan", "Vol").replace("Distortion", "Dist").replace("Modulation", "Mod").replace("Dynamics", "Dyn")} />)}
    </svg>
  );
}

export function IconGallery() {
  return (
    <div className="icon-gallery">
      <h2>Cabs</h2>
      <Cabs />
      <h2>Amps</h2>
      <Amps />
      {[1.2, 3].map((s) => (
        <section key={s}>
          <h2>{s === 1.2 ? "Chain scale" : "3×"}</h2>
          <Row scale={s} state="on" />
          <Row scale={s} state="off" />
          <Row scale={s} state="sel" />
        </section>
      ))}
      <section>
        <h2>MIDI window (static)</h2>
        <div className="mc-static-stage">
          <MidiWindow inline items={SAMPLE_MIDI} />
        </div>
      </section>
    </div>
  );
}
