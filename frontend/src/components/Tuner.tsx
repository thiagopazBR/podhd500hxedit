// Tuner (HX Edit layout) over the signal flow while the pedal's tuner is on.
// The reading comes in state.tuner (~5/s); settings go to POST /api/tuner.
import { useState } from "react";
import { post } from "../api";
import { useEditor } from "../store";

// Marker colour by distance from the note, measured on HX Edit:
// 0 #50f44f, 12 #97e44b, 21 #d1d647, 33 #f77f31, 46+ #ff001c.
const STOPS: [number, number[]][] = [[0, [0x50, 0xf4, 0x4f]], [12, [0x97, 0xe4, 0x4b]], [21, [0xd1, 0xd6, 0x47]], [33, [0xf7, 0x7f, 0x31]], [46, [0xff, 0x00, 0x1c]]];
function centsColor(cents: number) {
  const a = Math.min(46, Math.abs(cents));
  let i = 0;
  while (i < STOPS.length - 2 && a > STOPS[i + 1][0]) i++;
  const [x0, c0] = STOPS[i], [x1, c1] = STOPS[i + 1];
  const f = Math.max(0, Math.min(1, (a - x0) / (x1 - x0)));
  const ch = c0.map((v, k) => Math.round(v + (c1[k] - v) * f));
  return `rgb(${ch[0]}, ${ch[1]}, ${ch[2]})`;
}

export function Tuner() {
  const { state, toast } = useEditor();
  const t = state?.tuner;
  const [refText, setRefText] = useState<string | null>(null); // while typing
  if (!state?.connected || !t?.on) return null;

  const send = (body: object) => post("/api/tuner", body).catch((e: Error) => toast(`Tuner: ${e.message}`));
  const ref = t.reference || 440;
  const setRef = (hz: number) => {
    hz = Math.round(Math.min(455, Math.max(425, hz)));
    if (hz !== ref) void send({ reference: hz });
  };
  const sig = !!t.signal;
  const inTune = sig && Math.abs(t.cents) <= 1;
  const col = sig ? centsColor(t.cents) : "";
  const rc = Math.round(t.cents);
  const c = Math.max(-52, Math.min(52, t.cents));

  return (
    <div className="tuner">
      <button type="button" className={`tuner-mute${t.mute ? " on" : ""}`} aria-pressed={t.mute} title="Output while tuning: Mute or Bypass" onClick={() => send({ mute: !t.mute })}>
        <svg viewBox="0 0 24 24" aria-hidden="true"><path className="f" d="M4 9h4l5-4v14l-5-4H4z" /><path className="x" d="m16 9.5 5 5m0-5-5 5" /><path className="w" d="M16 9.5a3.5 3.5 0 0 1 0 5M18.5 7a7 7 0 0 1 0 10" /></svg>
        <span>{t.mute ? "Mute" : "Bypass"}</span>
      </button>
      <div className="tuner-stage">
        <div className="tuner-top">
          <div className="tuner-cents" hidden={!sig}>Cents: <b style={{ color: col }}>{sig ? (rc > 0 ? `+${rc}` : String(rc)) : ""}</b></div>
          <svg className={`tuner-chev l${sig && (inTune || t.cents < 0) ? " lit" : ""}`} viewBox="0 0 40 72" aria-hidden="true"><path d="M3 3l34 33L3 69" /></svg>
          <div className="tuner-note">
            <span>
              {sig ? t.note?.[0] : ""}
              {sig && t.note?.[1] === "b" && (
                <span className="tuner-flat-sign"><svg viewBox="0 0 20 44" aria-hidden="true"><path d="M3 0V43M3 43C9 38 17 33 17 27C17 20 8 20 3 26" /></svg></span>
              )}
            </span>
          </div>
          <svg className={`tuner-chev r${sig && (inTune || t.cents > 0) ? " lit" : ""}`} viewBox="0 0 40 72" aria-hidden="true"><path d="M37 3L3 36l34 33" /></svg>
        </div>
        <div className="tuner-bars" aria-hidden="true">
          {Array.from({ length: 10 }, (_, i) => <i key={i} />)}
          {sig && <span className="tuner-mark" style={{ left: `calc(50% + ${(c * 0.96).toFixed(2)}%)`, background: col }} />}
        </div>
      </div>
      <div className="tuner-foot">
        <label htmlFor="tuner-ref">Reference:</label>
        <div className="tuner-ref">
          <input
            id="tuner-ref"
            type="text"
            inputMode="numeric"
            aria-label="Reference (Hz)"
            value={refText ?? `${ref} Hz`}
            onFocus={(e) => { setRefText(String(ref)); setTimeout(() => e.target.select(), 0); }}
            onChange={(e) => setRefText(e.target.value)}
            onBlur={() => { const v = parseInt(refText ?? "", 10); setRefText(null); if (Number.isFinite(v)) setRef(v); }}
            onKeyDown={(e) => {
              if (e.key === "Enter") e.currentTarget.blur();
              else if (e.key === "Escape") { setRefText(null); e.currentTarget.blur(); e.stopPropagation(); }
              else if (e.key === "ArrowUp" || e.key === "ArrowDown") { e.preventDefault(); setRef(ref + (e.key === "ArrowUp" ? 1 : -1)); }
            }}
          />
          <div className="tuner-ref-step">
            <button type="button" aria-label="Higher" onClick={() => setRef(ref + 1)}><svg viewBox="0 0 10 6" aria-hidden="true"><path d="M1.5 5 5 1.5 8.5 5" /></svg></button>
            <button type="button" aria-label="Lower" onClick={() => setRef(ref - 1)}><svg viewBox="0 0 10 6" aria-hidden="true"><path d="M1.5 1 5 4.5 8.5 1" /></svg></button>
          </div>
        </div>
      </div>
    </div>
  );
}
