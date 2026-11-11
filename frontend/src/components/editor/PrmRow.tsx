// Parameter rows (HX Edit): name, value, ▲▼ and a fader. Ported from the
// classic UI (prmRow / initSliders / editValue / openParamList / toggleRow).
// Fader: drag or click, arrows / Page / Home / End when focused, the mouse
// wheel over the row, ▲▼ next to the value, double-click the value to type
// one; a list's value opens a dropdown.
import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { catGlyph, type Cat } from "../../flow/icons";
import { fracForText, type SliderDesc } from "../../editor/format";
import { useEditor } from "../../store";

/** A category glyph (classic iconSVG). */
export function CatIcon({ cat, cls = "" }: { cat: Cat; cls?: string }) {
  const g = catGlyph(cat);
  return <svg className={`${cls}${g.cls}`} viewBox={g.vb} style={{ "--cat": cat.color } as CSSProperties} dangerouslySetInnerHTML={{ __html: g.inner }} />;
}

const Arrow = ({ d }: { d: string }) => <svg viewBox="0 0 7 4" aria-hidden="true"><path d={d} /></svg>;

/** Read-only row (classic prmRow without a key). */
export function PrmRO({ name, value, frac }: { name: string; value: string; frac: number }) {
  const f = Math.max(0, Math.min(1, frac));
  return (
    <div className="prm ro">
      <span className="prm-name">{name}</span>
      <span className="prm-valbox"><span className="prm-val">{value}</span></span>
      <span className="prm-bar" aria-label={name} aria-valuenow={Math.round(f * 100)} style={{ "--v": f } as CSSProperties}><i /><b /></span>
    </div>
  );
}

export function PrmRow({ name, desc, extra, after }: { name: string; desc: SliderDesc; extra?: ReactNode; after?: ReactNode }) {
  const { toast } = useEditor();
  const [v, setV] = useState(desc.value);
  const [editing, setEditing] = useState<string | null>(null); // typed text while editing
  const [dd, setDD] = useState<{ left: number; top: number; maxHeight: number; cat: string } | null>(null);
  const vRef = useRef(v);
  vRef.current = v;
  const dragging = useRef(false);
  const row = useRef<HTMLDivElement>(null);
  const bar = useRef<HTMLSpanElement>(null);
  const valSpan = useRef<HTMLSpanElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const descRef = useRef(desc);
  descRef.current = desc;

  // follow the pedal, except while the fader is held
  useEffect(() => { if (!dragging.current) setV(desc.value); }, [desc.value]);

  const one = () => (desc.steps > 1 ? 1 / (desc.steps - 1) : 0.01);

  // setFromFrac: lists snap to options; send only real changes
  function setFrac(frac: number) {
    const d = descRef.current;
    let x = Math.max(0, Math.min(1, frac));
    if (d.steps > 1) x = Math.round(x * (d.steps - 1)) / (d.steps - 1);
    if (Math.abs(x - vRef.current) < 1e-6) return;
    vRef.current = x;
    setV(x);
    d.send(x);
  }

  // Mouse wheel over the row moves the fader (HX Edit). One notch = one
  // option/note for lists, 1 % otherwise; Shift = the finest step.
  // Trackpads send many small deltas: they add up to notches.
  useEffect(() => {
    const el = row.current;
    if (!el) return;
    let acc = 0;
    const onWheel = (e: WheelEvent) => {
      const d = descRef.current;
      e.preventDefault();
      const dy = (Math.abs(e.deltaY) >= Math.abs(e.deltaX) ? e.deltaY : e.deltaX) * (e.deltaMode === 1 ? 16 : 1);
      acc += (e as WheelEvent & { webkitDirectionInvertedFromDevice?: boolean }).webkitDirectionInvertedFromDevice ? -dy : dy;
      const notches = Math.trunc(acc / 40);
      if (!notches) return;
      acc -= notches * 40;
      const fine = d.steps > 1 ? 1 / (d.steps - 1) : 0.001;
      const step = e.shiftKey || (d.steps > 1 && d.steps <= 201) ? fine : 0.01;
      setFrac(vRef.current - notches * step); // wheel up = higher value
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // the list dropdown closes on an outside press or Escape
  useEffect(() => {
    if (!dd) return;
    const onDown = (e: PointerEvent) => {
      const t = e.target as Node;
      if (!menu.current?.contains(t) && !valSpan.current?.contains(t)) setDD(null);
    };
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setDD(null); };
    document.addEventListener("pointerdown", onDown);
    document.addEventListener("keydown", onKey);
    menu.current?.querySelector(".cur")?.scrollIntoView({ block: "center" });
    return () => { document.removeEventListener("pointerdown", onDown); document.removeEventListener("keydown", onKey); };
  }, [dd]);

  function openList() {
    if (dd) { setDD(null); return; }
    const span = valSpan.current;
    if (!span) return;
    const r = span.getBoundingClientRect();
    const w = 140; // min width; the menu grows with its options
    const below = innerHeight - r.bottom - 8;
    setDD({
      left: Math.max(8, Math.min(r.right - w + 14, innerWidth - w - 8)),
      top: below >= 160 ? r.bottom + 4 : Math.max(8, r.top - 320),
      maxHeight: Math.max(160, Math.min(320, below)),
      cat: getComputedStyle(span).getPropertyValue("--cat"),
    });
  }

  function commitTyped(text: string | null) {
    setEditing(null);
    if (text === null) return;
    const old = desc.fmt(v);
    const f = fracForText(desc, text);
    if (f !== null) setFrac(f);
    else if (text.trim() !== old) toast(`"${text.trim()}" isn't a valid value`);
  }

  const f = Math.max(0, Math.min(1, v));
  const shown = desc.fmt(v);
  const opts = desc.options;
  const cur = opts ? Math.round(v * (opts.length - 1)) : -1;

  return (
    <div className="prm" ref={row}>
      <span className="prm-name">{name}{extra}</span>
      <span className="prm-valbox">
        {opts ? (
          <span className="prm-val prm-dd" ref={valSpan} onClick={openList}>
            {shown}<svg viewBox="0 0 8 5" aria-hidden="true"><path d="M0 0h8L4 5z" /></svg>
          </span>
        ) : (
          <span className="prm-val" ref={valSpan} onDoubleClick={() => { window.getSelection()?.removeAllRanges(); setEditing(shown); }}>
            {editing !== null ? (
              <input
                className="prm-input"
                autoFocus
                value={editing}
                aria-label={`${name} value`}
                style={{ width: `${Math.max(1, editing.length) + 0.3}ch` }}
                onFocus={(e) => e.target.select()}
                onChange={(e) => setEditing(e.target.value)}
                onBlur={() => commitTyped(editing)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") { e.preventDefault(); commitTyped(editing); }
                  else if (e.key === "Escape") { e.preventDefault(); commitTyped(null); }
                  e.stopPropagation(); // no stepping while typing
                }}
              />
            ) : shown}
          </span>
        )}
        <span className="prm-step">
          <button type="button" aria-label={`Increase ${name}`} onClick={() => setFrac(vRef.current + one())}><Arrow d="M0 4L3.5 0L7 4Z" /></button>
          <button type="button" aria-label={`Decrease ${name}`} onClick={() => setFrac(vRef.current - one())}><Arrow d="M0 0L3.5 4L7 0Z" /></button>
        </span>
        {after}
      </span>
      <span
        ref={bar}
        className="prm-bar"
        tabIndex={0}
        role="slider"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={name}
        aria-valuenow={Math.round(f * 100)}
        style={{ "--v": f } as CSSProperties}
        onPointerDown={(e) => {
          if (e.button !== 0) return;
          e.preventDefault();
          const el = e.currentTarget;
          el.focus({ preventScroll: true });
          el.setPointerCapture(e.pointerId);
          dragging.current = true;
          const r = el.getBoundingClientRect();
          setFrac((e.clientX - r.left - 6) / (r.width - 12)); // the 12px handle stays inside the track
        }}
        onPointerMove={(e) => {
          if (!dragging.current) return;
          const r = e.currentTarget.getBoundingClientRect();
          setFrac((e.clientX - r.left - 6) / (r.width - 12));
        }}
        onPointerUp={() => { dragging.current = false; }}
        onPointerCancel={() => { dragging.current = false; }}
        onKeyDown={(e) => {
          const st = e.shiftKey ? (desc.steps > 1 && desc.steps <= 20 ? one() : Math.max(one(), 0.1)) : one();
          const delta = ({ ArrowRight: st, ArrowUp: st, ArrowLeft: -st, ArrowDown: -st, PageUp: 0.1, PageDown: -0.1 } as Record<string, number>)[e.key];
          if (delta === undefined && e.key !== "Home" && e.key !== "End") return;
          e.preventDefault();
          setFrac(e.key === "Home" ? 0 : e.key === "End" ? 1 : vRef.current + delta);
        }}
      ><i /><b /></span>
      {dd && opts && createPortal(
        <div ref={menu} className="as-dd-list prm-ddmenu" style={{ left: dd.left, top: dd.top, maxHeight: dd.maxHeight, "--cat": dd.cat } as CSSProperties}>
          {opts.map((o, i) => (
            <button key={i} type="button" className={i === cur ? "cur" : ""} onClick={() => {
              setDD(null);
              if (i !== cur) setFrac(opts.length > 1 ? i / (opts.length - 1) : 0);
            }}>{o}</button>
          ))}
        </div>,
        document.body,
      )}
    </div>
  );
}

/** 2–4 option parameters as HX Edit: a row of buttons instead of a fader (classic toggleRow). */
export function ToggleRow({ name, options, index, extra, onPick }: { name: string; options: string[]; index: number; extra?: ReactNode; onPick: (i: number) => void }) {
  const [idx, setIdx] = useState(index);
  useEffect(() => setIdx(index), [index]);
  // "SW1 OFF" → "OFF": the switch name is already the row's label
  const label = (o: string) => o.replace(/^SW\d+\s+/i, "");
  return (
    <div className="prm prm-switch">
      <span className="prm-name">{name}{extra}</span>
      <span className="seg" role="group" aria-label={name}>
        {options.map((o, i) => (
          <button key={i} type="button" className={i === idx ? "cur" : ""} aria-pressed={i === idx} onClick={() => { if (i !== idx) { setIdx(i); onPick(i); } }}>{label(o)}</button>
        ))}
      </span>
    </div>
  );
}

/** A pedal-wide Global / Preset setting as an HX two-button switch row (classic setupSwitch). */
export function SetupSwitch({ name, opts, cur, onPick }: { name: string; opts: { value: number; label: string }[]; cur: number | undefined; onPick: (v: number) => void }) {
  return (
    <div className="prm prm-switch">
      <span className="prm-name">{name}</span>
      <span className="seg" role="group" aria-label={name}>
        {opts.map((o) => (
          <button key={o.value} type="button" className={o.value === cur ? "cur" : ""} aria-pressed={o.value === cur} onClick={() => { if (o.value !== cur) onPick(o.value); }}>{o.label}</button>
        ))}
      </span>
    </div>
  );
}
