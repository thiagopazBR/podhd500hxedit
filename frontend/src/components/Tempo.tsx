// Tap tempo + BPM (HX Edit): the BPM box is the Tap Tempo button. Click
// repeatedly (or T) to tap; drag up/down or scroll to change; right-click
// to type. Only the newest value is sent; values made while a request is in
// flight are coalesced into one follow-up request.
import { useEffect, useRef, useState } from "react";
import { post } from "../api";
import { useEditor } from "../store";

const fmtBPM = (v: number) => (Math.round(v * 10) / 10).toFixed(1);
const STEP_PX = 4;

export function Tempo() {
  const { state, meta, toast } = useEditor();
  const online = !!state?.connected;
  const known = online && !state?.waitingTuner && !!state?.tempo;
  const [text, setText] = useState("—");
  const [typing, setTyping] = useState(false);
  const [hit, setHit] = useState(false);
  const [dragging, setDragging] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const send = useRef({ busy: false, queued: null as number | null });
  const taps = useRef<number[]>([]);
  const wheel = useRef({ acc: 0, timer: 0 });
  const drag = useRef<{ id: number; lastY: number; acc: number; value: number; moved: boolean; timer: number } | null>(null);
  const busyUI = typing || dragging || send.current.busy;

  const clamp = (v: number) => Math.min(meta?.tempo.max ?? 240, Math.max(meta?.tempo.min ?? 30, v));

  // follow the pedal, except while the user is changing it
  useEffect(() => {
    if (!known) { setText("—"); return; }
    if (!busyUI) setText(fmtBPM(state!.tempo));
  }, [known, state?.tempo, busyUI]); // eslint-disable-line react-hooks/exhaustive-deps

  async function sendTempo(bpm: number) {
    bpm = Math.round(clamp(bpm) * 10) / 10;
    setText(fmtBPM(bpm));
    const s = send.current;
    if (s.busy) { s.queued = bpm; return; }
    s.busy = true;
    try {
      await post("/api/tempo", { bpm });
    } catch (e) {
      toast(`Couldn't change the tempo: ${(e as Error).message}`);
      if (state) setText(fmtBPM(state.tempo));
    } finally {
      s.busy = false;
      if (s.queued !== null) { const n = s.queued; s.queued = null; void sendTempo(n); }
    }
  }

  function tap() {
    if (!online) return;
    const now = performance.now();
    const t = taps.current;
    if (t.length && now - t[t.length - 1] > 2000) t.length = 0; // a pause starts a new tempo
    t.push(now);
    if (t.length > 5) t.shift();
    setHit(true);
    setTimeout(() => setHit(false), 90);
    if (t.length < 3) return;
    void sendTempo(60000 / ((t[t.length - 1] - t[0]) / (t.length - 1)));
  }

  const current = () => parseFloat(text.replace(",", ".")) || state?.tempo || 120;

  function step(delta: number) {
    const next = clamp(Math.round((current() + delta) * 10) / 10);
    setText(fmtBPM(next));
    clearTimeout(wheel.current.timer); // wait for the user to stop before sending
    wheel.current.timer = window.setTimeout(() => void sendTempo(next), 250);
  }

  function commitTyped() {
    setTyping(false);
    const v = parseFloat(text.replace(",", "."));
    if (!Number.isFinite(v) || (state && Math.abs(v - state.tempo) < 0.05)) { if (state) setText(fmtBPM(state.tempo)); return; }
    void sendTempo(v);
  }

  // "T" taps anywhere, except while typing in a field
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target;
      if (t instanceof HTMLInputElement || t instanceof HTMLSelectElement || e.metaKey || e.ctrlKey) return;
      if (e.key === "t" || e.key === "T") tap();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  });

  // wheel needs a non-passive listener (preventDefault)
  const field = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = field.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      if (!online) return;
      e.preventDefault();
      const fine = e.altKey || e.shiftKey;
      let steps: number;
      if (e.deltaMode !== 0 || Math.abs(e.deltaY) >= 50) steps = -Math.sign(e.deltaY);
      else { wheel.current.acc -= e.deltaY; steps = Math.trunc(wheel.current.acc / 25); wheel.current.acc -= steps * 25; }
      if (steps) step(steps * (fine ? 0.1 : 1));
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  });

  return (
    <div className="hxh-tempo">
      <div
        ref={field}
        className={`hxh-box bpm-field${hit ? " hit" : ""}${dragging ? " dragging" : ""}`}
        title="Tap Tempo: click repeatedly (or the T key) · drag or scroll to change · right-click to type"
        onPointerDown={(e) => {
          if (!online || e.button !== 0 || typing || e.ctrlKey) return;
          e.preventDefault();
          e.currentTarget.setPointerCapture(e.pointerId);
          drag.current = { id: e.pointerId, lastY: e.clientY, acc: 0, value: current(), moved: false, timer: 0 };
          tap(); // the beat lands on the press
        }}
        onPointerMove={(e) => {
          const d = drag.current;
          if (!d || e.pointerId !== d.id) return;
          d.acc += d.lastY - e.clientY;
          d.lastY = e.clientY;
          if (!d.moved) {
            if (Math.abs(d.acc) < 3) return;
            d.moved = true;
            taps.current.length = 0; // a drag, not taps
            setDragging(true);
            document.body.classList.add("dragging-bpm");
          }
          const steps = Math.trunc(d.acc / STEP_PX);
          if (!steps) return;
          d.acc -= steps * STEP_PX;
          const next = clamp(Math.round((d.value + steps * (e.shiftKey ? 0.1 : 1)) * 10) / 10);
          if (next === d.value) return;
          d.value = next;
          setText(fmtBPM(next));
          if (!d.timer) d.timer = window.setTimeout(() => { d.timer = 0; if (drag.current === d) void sendTempo(d.value); }, 120);
        }}
        onPointerUp={(e) => {
          const d = drag.current;
          if (!d || e.pointerId !== d.id) return;
          drag.current = null;
          clearTimeout(d.timer);
          setDragging(false);
          document.body.classList.remove("dragging-bpm");
          if (d.moved) void sendTempo(d.value);
        }}
        onContextMenu={(e) => {
          e.preventDefault();
          if (!online) return;
          taps.current.length = 0;
          setTyping(true);
          setTimeout(() => { input.current?.focus(); input.current?.select(); }, 0);
        }}
      >
        <input
          ref={input}
          type="text"
          inputMode="decimal"
          autoComplete="off"
          spellCheck={false}
          disabled={!online}
          readOnly={!typing}
          tabIndex={typing ? 0 : -1}
          aria-label="Tempo (BPM)"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onBlur={() => { if (typing) commitTyped(); }}
          onKeyDown={(e) => {
            if (e.key === "Enter") { e.preventDefault(); input.current?.blur(); }
            else if (e.key === "Escape") { if (state) setText(fmtBPM(state.tempo)); setTyping(false); input.current?.blur(); }
            else if (e.key === "ArrowUp" || e.key === "ArrowDown") {
              e.preventDefault();
              const s = e.altKey ? 0.1 : e.shiftKey ? 10 : 1;
              step(e.key === "ArrowUp" ? s : -s);
            }
          }}
        />
      </div>
    </div>
  );
}
