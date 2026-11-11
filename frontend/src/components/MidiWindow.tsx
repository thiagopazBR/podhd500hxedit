// MIDI window (HX Edit Command Center, page 46 of its guide): the MIDI Assign
// of the loaded preset, i.e. what each footswitch / pedal SENDS on MIDI OUT.
// Left: the 13 controls laid out as on the pedal's MIDI ASSIGN screen;
// Right: the selected control's settings.
// Purely visual component (props only, no store/API imports).
// Ported from the classic UI (cmd/hd500-edit/web/midi.js).
import { useEffect, useRef, useState, type CSSProperties } from "react";
import { createPortal } from "react-dom";
import "../midiwindow.css";

export interface MidiOption {
  value: number;
  label: string;
}

export interface MidiAssignItem {
  control: number;
  name: string;
  pedal: boolean; // EXP 1/2: CC with Min/Max instead of a message
  message: number; // 0 CC Toggle, 1 CC, 2 Prog Change, 3 Bank Change, 4 Note On
  channel: number; // 16 None (sends nothing), 17 Base, 0..15 = 1..16
  number: number; // CC # / Program # / Bank # / Note (0..127)
  value: number; // Value / Velocity; pedals: Min Value (0..127)
  max: number; // pedals: Max Value (0..127)
}

export type MidiChangeField = "message" | "channel" | "number" | "value" | "max";

export interface MidiWindowProps {
  open?: boolean;
  inline?: boolean; // render dialog inline without fixed full-screen backdrop (for static gallery preview)
  items: MidiAssignItem[];
  messages?: MidiOption[];
  channels?: MidiOption[];
  selected?: number;
  defaultSelected?: number;
  onSelect?: (control: number) => void;
  onChange?: (control: number, patch: Partial<Pick<MidiAssignItem, MidiChangeField>>) => void;
  onClose?: () => void;
}

export const DEFAULT_MIDI_MESSAGES: MidiOption[] = [
  { value: 0, label: "CC Toggle" },
  { value: 1, label: "CC" },
  { value: 2, label: "Prog Change" },
  { value: 3, label: "Bank Change" },
  { value: 4, label: "Note On" },
];

export const DEFAULT_MIDI_CHANNELS: MidiOption[] = [
  { value: 16, label: "None" },
  { value: 17, label: "Base" },
  ...Array.from({ length: 16 }, (_, i) => ({
    value: i,
    label: String(i + 1).padStart(2, "0"),
  })),
];

// Grid of the pedal's MIDI ASSIGN screen: [control, column, row, span]
// (control ids follow the pedal's storage order = this screen read row by row)
const MIDI_GRID: [number, number, number, number?][] = [
  [0, 0, 0],
  [1, 1, 0],
  [2, 2, 0],
  [3, 3, 0],
  [4, 4, 0],
  [5, 5, 0, 2],
  [6, 0, 1],
  [7, 1, 1],
  [8, 2, 1],
  [9, 3, 1],
  [10, 4, 1],
  [11, 5, 1],
  [12, 6, 1],
];

const SwSvg = () => (
  <svg viewBox="0 0 24 24" aria-hidden="true">
    <ellipse cx="12" cy="7" rx="6" ry="2.6" />
    <path d="M6 7v4c0 1.4 2.7 2.6 6 2.6s6-1.2 6-2.6V7M8 13.2v4.2c0 1 1.8 1.9 4 1.9s4-.9 4-1.9v-4.2" />
  </svg>
);

// EXP 1 / EXP 2: a tall treadle, as large as HX Edit's Command Center pedals
const PedalSvg = () => (
  <svg className="mc-pedal" viewBox="0 0 28 52" aria-hidden="true">
    <rect x="3" y="2" width="22" height="48" rx="2.5" />
    <path d="M7 7h14M7 11h14M7 15h14" />
    <rect x="9" y="40" width="10" height="5" rx="1" />
  </svg>
);

// EXP Toe: a two-position toggle switch (lever, threaded bushing, body, two
// pins); a line runs from its pins down to EXP 1 and EXP 2
const ToeSvg = () => (
  <svg className="mc-toe" viewBox="0 0 24 40" aria-hidden="true">
    <path className="lever" d="M12 17 15.5 3.5" />
    <rect x="9" y="13.5" width="6" height="7.5" rx=".8" />
    <path d="M9 16h6M9 18.5h6" />
    <rect x="5" y="21" width="14" height="10" rx="1.2" />
    <path d="M9.5 31v8M14.5 31v8" />
  </svg>
);

const TapSvg = () => (
  <svg viewBox="0 0 24 24" aria-hidden="true">
    <circle cx="12" cy="12" r="7.5" />
    <path d="M12 7.5V12l3 2" />
  </svg>
);

const LoopSvg = () => (
  <svg viewBox="0 0 24 24" aria-hidden="true">
    <path d="M5 12a7 7 0 0 1 12-4.9M19 12a7 7 0 0 1-12 4.9" />
    <path d="M17 3.5V7.5h-4M7 20.5v-4h4" />
  </svg>
);

function MidiIcon({ item }: { item: MidiAssignItem }) {
  if (item.pedal) return <PedalSvg />;
  if (item.control === 5) return <ToeSvg />;
  if (item.control === 10) return <TapSvg />;
  if (item.control === 4) return <LoopSvg />;
  return <SwSvg />;
}

/**
 * List parameter as the editor draws it (PrmRow with options): name, value box
 * that opens a menu, up/down arrows and a fader that snaps to each option.
 * The value stays the option's `value`; the fader works on its index.
 */
function MidiListRow({
  name,
  options,
  value,
  open,
  onOpen,
  onClose,
  onChange,
}: {
  name: string;
  options: MidiOption[];
  value: number;
  open: boolean;
  onOpen: () => void;
  onClose: () => void;
  onChange: (v: number) => void;
}) {
  const n = options.length;
  const propIdx = Math.max(0, options.findIndex((o) => o.value === value));
  const [idx, setIdx] = useState(propIdx);
  const [pos, setPos] = useState<{ left: number; top: number; maxHeight: number } | null>(null);
  const idxRef = useRef(idx);
  idxRef.current = idx;
  const optsRef = useRef(options);
  optsRef.current = options;
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  const dragging = useRef(false);
  const rowRef = useRef<HTMLDivElement>(null);
  const valRef = useRef<HTMLSpanElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!dragging.current) setIdx(propIdx);
  }, [propIdx]);

  function setTo(next: number) {
    const last = optsRef.current.length - 1;
    const i = Math.max(0, Math.min(last, Math.round(next)));
    if (i === idxRef.current) return;
    idxRef.current = i;
    setIdx(i);
    onChangeRef.current(optsRef.current[i].value);
  }

  // mouse wheel over the row: one notch = one option
  useEffect(() => {
    const el = rowRef.current;
    if (!el) return;
    let acc = 0;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const dy = (Math.abs(e.deltaY) >= Math.abs(e.deltaX) ? e.deltaY : e.deltaX) * (e.deltaMode === 1 ? 16 : 1);
      acc += (e as WheelEvent & { webkitDirectionInvertedFromDevice?: boolean }).webkitDirectionInvertedFromDevice ? -dy : dy;
      const notches = Math.trunc(acc / 40);
      if (!notches) return;
      acc -= notches * 40;
      setTo(idxRef.current - notches);
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, []);

  // the menu closes on an outside press (Escape is handled by the window)
  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      const t = e.target as Node;
      if (!menuRef.current?.contains(t) && !valRef.current?.contains(t)) onClose();
    };
    document.addEventListener("pointerdown", onDown);
    menuRef.current?.querySelector(".cur")?.scrollIntoView({ block: "center" });
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open, onClose]);

  function toggleList() {
    if (open) {
      onClose();
      return;
    }
    const span = valRef.current;
    if (!span) return;
    const r = span.getBoundingClientRect();
    const w = 140;
    const below = innerHeight - r.bottom - 8;
    setPos({
      left: Math.max(8, Math.min(r.right - w + 14, innerWidth - w - 8)),
      top: below >= 160 ? r.bottom + 4 : Math.max(8, r.top - 320),
      maxHeight: Math.max(160, Math.min(320, below)),
    });
    onOpen();
  }

  const f = n > 1 ? idx / (n - 1) : 0;
  const shown = options[idx]?.label ?? "—";
  const fromPointer = (e: { clientX: number; currentTarget: HTMLElement }) => {
    const r = e.currentTarget.getBoundingClientRect();
    setTo(((e.clientX - r.left - 6) / (r.width - 12)) * (n - 1));
  };

  return (
    <div className="prm" ref={rowRef}>
      <span className="prm-name">{name}</span>
      <span className="prm-valbox">
        <span className="prm-val prm-dd" ref={valRef} onClick={toggleList}>
          {shown}
          <svg viewBox="0 0 8 5" aria-hidden="true">
            <path d="M0 0h8L4 5z" />
          </svg>
        </span>
        <span className="prm-step">
          <button type="button" aria-label={`Increase ${name}`} onClick={() => setTo(idxRef.current + 1)}>
            <svg viewBox="0 0 7 4" aria-hidden="true">
              <path d="M0 4L3.5 0L7 4Z" />
            </svg>
          </button>
          <button type="button" aria-label={`Decrease ${name}`} onClick={() => setTo(idxRef.current - 1)}>
            <svg viewBox="0 0 7 4" aria-hidden="true">
              <path d="M0 0L3.5 4L7 0Z" />
            </svg>
          </button>
        </span>
      </span>
      <span
        className="prm-bar"
        tabIndex={0}
        role="slider"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={name}
        aria-valuenow={Math.round(f * 100)}
        aria-valuetext={shown}
        style={{ "--v": f } as CSSProperties}
        onPointerDown={(e) => {
          if (e.button !== 0) return;
          e.preventDefault();
          const el = e.currentTarget;
          el.focus({ preventScroll: true });
          el.setPointerCapture(e.pointerId);
          dragging.current = true;
          fromPointer(e);
        }}
        onPointerMove={(e) => {
          if (dragging.current) fromPointer(e);
        }}
        onPointerUp={() => {
          dragging.current = false;
        }}
        onPointerCancel={() => {
          dragging.current = false;
        }}
        onKeyDown={(e) => {
          const delta = ({ ArrowRight: 1, ArrowUp: 1, ArrowLeft: -1, ArrowDown: -1, PageUp: 5, PageDown: -5 } as Record<string, number>)[e.key];
          if (delta === undefined && e.key !== "Home" && e.key !== "End") return;
          e.preventDefault();
          setTo(e.key === "Home" ? 0 : e.key === "End" ? n - 1 : idxRef.current + delta);
        }}
      >
        <i />
        <b />
      </span>
      {open &&
        pos &&
        createPortal(
          <div
            ref={menuRef}
            className="as-dd-list prm-ddmenu"
            role="listbox"
            style={{ left: pos.left, top: pos.top, maxHeight: pos.maxHeight }}
          >
            {options.map((o, i) => (
              <button
                key={o.value}
                type="button"
                role="option"
                aria-selected={i === idx}
                className={i === idx ? "cur" : ""}
                onClick={() => {
                  onClose();
                  setTo(i);
                }}
              >
                {o.label}
              </button>
            ))}
          </div>,
          document.body,
        )}
    </div>
  );
}

/** 0..127 integer slider row matching classic UI's slider("mc.<ctl>.<key>", name, { steps: 128 }). */
function MidiSlider({
  name,
  value,
  onChange,
}: {
  name: string;
  value: number;
  onChange: (v: number) => void;
}) {
  const [v, setV] = useState(() => Math.max(0, Math.min(127, Math.round(value))));
  const [editing, setEditing] = useState<string | null>(null);
  const vRef = useRef(v);
  vRef.current = v;
  const dragging = useRef(false);
  const rowRef = useRef<HTMLDivElement>(null);
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;

  useEffect(() => {
    if (!dragging.current) {
      setV(Math.max(0, Math.min(127, Math.round(value))));
    }
  }, [value]);

  function setInt(next: number) {
    const clamped = Math.max(0, Math.min(127, Math.round(next)));
    if (clamped === vRef.current) return;
    vRef.current = clamped;
    setV(clamped);
    onChangeRef.current(clamped);
  }

  useEffect(() => {
    const el = rowRef.current;
    if (!el) return;
    let acc = 0;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const dy = (Math.abs(e.deltaY) >= Math.abs(e.deltaX) ? e.deltaY : e.deltaX) * (e.deltaMode === 1 ? 16 : 1);
      acc += (e as WheelEvent & { webkitDirectionInvertedFromDevice?: boolean }).webkitDirectionInvertedFromDevice ? -dy : dy;
      const notches = Math.trunc(acc / 40);
      if (!notches) return;
      acc -= notches * 40;
      setInt(vRef.current - notches);
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, []);

  function commitTyped(text: string | null) {
    setEditing(null);
    if (text === null) return;
    const m = text.trim().match(/-?\d+/);
    if (m) setInt(parseInt(m[0], 10));
  }

  const f = v / 127;
  const shown = String(v);

  return (
    <div className="prm" ref={rowRef}>
      <span className="prm-name">{name}</span>
      <span className="prm-valbox">
        <span
          className="prm-val"
          onDoubleClick={() => {
            window.getSelection()?.removeAllRanges();
            setEditing(shown);
          }}
        >
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
                if (e.key === "Enter") {
                  e.preventDefault();
                  commitTyped(editing);
                } else if (e.key === "Escape") {
                  e.preventDefault();
                  commitTyped(null);
                }
                e.stopPropagation();
              }}
            />
          ) : (
            shown
          )}
        </span>
        <span className="prm-step">
          <button type="button" aria-label={`Increase ${name}`} onClick={() => setInt(vRef.current + 1)}>
            <svg viewBox="0 0 7 4" aria-hidden="true">
              <path d="M0 4L3.5 0L7 4Z" />
            </svg>
          </button>
          <button type="button" aria-label={`Decrease ${name}`} onClick={() => setInt(vRef.current - 1)}>
            <svg viewBox="0 0 7 4" aria-hidden="true">
              <path d="M0 0L3.5 4L7 0Z" />
            </svg>
          </button>
        </span>
      </span>
      <span
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
          setInt(((e.clientX - r.left - 6) / (r.width - 12)) * 127);
        }}
        onPointerMove={(e) => {
          if (!dragging.current) return;
          const r = e.currentTarget.getBoundingClientRect();
          setInt(((e.clientX - r.left - 6) / (r.width - 12)) * 127);
        }}
        onPointerUp={() => {
          dragging.current = false;
        }}
        onPointerCancel={() => {
          dragging.current = false;
        }}
        onKeyDown={(e) => {
          const st = e.shiftKey ? 13 : 1;
          const delta = (
            {
              ArrowRight: st,
              ArrowUp: st,
              ArrowLeft: -st,
              ArrowDown: -st,
              PageUp: 13,
              PageDown: -13,
            } as Record<string, number>
          )[e.key];
          if (delta === undefined && e.key !== "Home" && e.key !== "End") return;
          e.preventDefault();
          setInt(e.key === "Home" ? 0 : e.key === "End" ? 127 : vRef.current + delta);
        }}
      >
        <i />
        <b />
      </span>
    </div>
  );
}

/** Inspector title: the selected control's icon and name, which opens a list of all 13 controls. */
function MidiControlPicker({
  items,
  item,
  open,
  onOpen,
  onClose,
  onPick,
}: {
  items: MidiAssignItem[];
  item: MidiAssignItem;
  open: boolean;
  onOpen: () => void;
  onClose: () => void;
  onPick: (control: number) => void;
}) {
  const [pos, setPos] = useState<{ left: number; top: number; maxHeight: number } | null>(null);
  const btnRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  // the menu closes on an outside press (Escape is handled by the window)
  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      const t = e.target as Node;
      if (!menuRef.current?.contains(t) && !btnRef.current?.contains(t)) onClose();
    };
    document.addEventListener("pointerdown", onDown);
    menuRef.current?.querySelector(".cur")?.scrollIntoView({ block: "nearest" });
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open, onClose]);

  function toggle() {
    if (open) {
      onClose();
      return;
    }
    const b = btnRef.current;
    if (!b) return;
    const r = b.getBoundingClientRect();
    const below = innerHeight - r.bottom - 8;
    setPos({
      left: Math.max(8, Math.min(r.left, innerWidth - 168)),
      top: below >= 160 ? r.bottom + 2 : Math.max(8, r.top - 320),
      maxHeight: Math.max(160, Math.min(340, below)),
    });
    onOpen();
  }

  return (
    <>
      <button type="button" className="mc-ihead-btn" ref={btnRef} aria-haspopup="listbox" aria-expanded={open} title="Choose the control" onClick={toggle}>
        <MidiIcon item={item} />
        <span>{item.name}</span>
        <svg className="mc-caret" viewBox="0 0 8 5" aria-hidden="true">
          <path d="M0 0h8L4 5z" />
        </svg>
      </button>
      {open &&
        pos &&
        createPortal(
          <div
            ref={menuRef}
            className="as-dd-list prm-ddmenu"
            role="listbox"
            style={{ left: pos.left, top: pos.top, maxHeight: pos.maxHeight }}
          >
            {items.map((it) => (
              <button
                key={it.control}
                type="button"
                role="option"
                aria-selected={it.control === item.control}
                className={it.control === item.control ? "cur" : ""}
                onClick={() => onPick(it.control)}
              >
                {it.name}
              </button>
            ))}
          </div>,
          document.body,
        )}
    </>
  );
}

export function MidiWindow({
  open = true,
  inline = false,
  items,
  messages = DEFAULT_MIDI_MESSAGES,
  channels = DEFAULT_MIDI_CHANNELS,
  selected,
  defaultSelected = 0,
  onSelect,
  onChange,
  onClose,
}: MidiWindowProps) {
  const [internalSel, setInternalSel] = useState(defaultSelected);
  const [ddOpen, setDdOpen] = useState<"" | "message" | "channel" | "control">("");
  const [localItems, setLocalItems] = useState<MidiAssignItem[]>(items);
  const dialogRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setLocalItems(items);
  }, [items]);

  const sel = selected !== undefined ? selected : internalSel;

  // Escape closes dropdown first, then modal (classic initMidi)
  useEffect(() => {
    if (!open || inline) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        if (ddOpen) setDdOpen("");
        else onClose?.();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, inline, ddOpen, onClose]);

  if (!open) return null;

  const selectControl = (c: number) => {
    setDdOpen("");
    if (selected === undefined) setInternalSel(c);
    onSelect?.(c);
  };

  const updateField = (control: number, field: MidiChangeField, val: number) => {
    setLocalItems((prev) => prev.map((it) => (it.control === control ? { ...it, [field]: val } : it)));
    onChange?.(control, { [field]: val });
  };

  const a = localItems[sel];

  const dialog = (
    <div
      className="mc"
      role="dialog"
      aria-label="MIDI"
      ref={dialogRef}
    >
      <div className="mc-top">
        <span className="mc-title">MIDI Assign</span>
        <button type="button" className="mc-snap" disabled title="Snapshots (not on the POD HD500)">
          <b>1</b>SNAPSHOT 1
        </button>
      </div>
      <div className="mc-body">
        <div className="mc-grid">
          {!localItems.length ? (
            <span className="mc-empty">No preset loaded.</span>
          ) : (
            MIDI_GRID.map(([c, col, row, span]) => {
              const item = localItems[c];
              if (!item) return null;
              const on = item.channel !== 16; // None = sends nothing
              const cls = `mc-cell${c === sel ? " sel" : ""}${on ? " on" : ""}${span ? " wide" : ""}`;
              return (
                <button
                  key={c}
                  type="button"
                  className={cls}
                  data-mc={c}
                  data-pedal={item.pedal ? "1" : undefined}
                  style={{
                    gridColumn: `${col + 1}${span ? ` / span ${span}` : ""}`,
                    gridRow: row + 1,
                  }}
                  title={item.name}
                  onClick={() => selectControl(c)}
                >
                  <i className="mc-tri" />
                  <MidiIcon item={item} />
                  <span>{item.name}</span>
                  {c === 5 && (
                    <>
                      <i className="mc-stem" />
                      <i className="mc-fork" />
                    </>
                  )}
                </button>
              );
            })
          )}
        </div>
        <div className="mc-insp">
          {a && (
            <>
              <div className="mc-ihead">
                <MidiControlPicker
                  items={localItems}
                  item={a}
                  open={ddOpen === "control"}
                  onOpen={() => setDdOpen("control")}
                  onClose={() => setDdOpen("")}
                  onPick={selectControl}
                />
              </div>
              <div className="params">
                <MidiListRow
                  name="MIDI Channel"
                  options={channels}
                  value={a.channel}
                  open={ddOpen === "channel"}
                  onOpen={() => setDdOpen("channel")}
                  onClose={() => setDdOpen("")}
                  onChange={(v) => updateField(a.control, "channel", v)}
                />
                {!a.pedal && (
                  <MidiListRow
                    name="Message"
                    options={messages}
                    value={a.message}
                    open={ddOpen === "message"}
                    onOpen={() => setDdOpen("message")}
                    onClose={() => setDdOpen("")}
                    onChange={(v) => updateField(a.control, "message", v)}
                  />
                )}
                {a.channel !== 16 && (
                  <>
                    {a.pedal ? (
                      <>
                        <MidiSlider key={`mc.${a.control}.n`} name="CC #" value={a.number} onChange={(v) => updateField(a.control, "number", v)} />
                        <MidiSlider key={`mc.${a.control}.v`} name="Min Value" value={a.value} onChange={(v) => updateField(a.control, "value", v)} />
                        <MidiSlider key={`mc.${a.control}.m`} name="Max Value" value={a.max} onChange={(v) => updateField(a.control, "max", v)} />
                      </>
                    ) : a.message <= 1 ? (
                      <>
                        <MidiSlider key={`mc.${a.control}.n`} name="CC #" value={a.number} onChange={(v) => updateField(a.control, "number", v)} />
                        <MidiSlider key={`mc.${a.control}.v`} name="Value" value={a.value} onChange={(v) => updateField(a.control, "value", v)} />
                      </>
                    ) : a.message === 2 ? (
                      <MidiSlider key={`mc.${a.control}.n`} name="Program #" value={a.number} onChange={(v) => updateField(a.control, "number", v)} />
                    ) : a.message === 3 ? (
                      <MidiSlider key={`mc.${a.control}.n`} name="Bank #" value={a.number} onChange={(v) => updateField(a.control, "number", v)} />
                    ) : (
                      <>
                        <MidiSlider key={`mc.${a.control}.n`} name="Note" value={a.number} onChange={(v) => updateField(a.control, "number", v)} />
                        <MidiSlider key={`mc.${a.control}.v`} name="Velocity" value={a.value} onChange={(v) => updateField(a.control, "value", v)} />
                      </>
                    )}
                  </>
                )}
              </div>
            </>
          )}
        </div>
      </div>
      <div className="mc-foot">
        <button type="button" className="mc-done" onClick={() => onClose?.()}>
          Done
        </button>
      </div>
    </div>
  );

  if (inline) return dialog;

  return (
    <div
      className="mc-overlay"
      onPointerDown={(e) => {
        if (e.target === e.currentTarget) onClose?.();
      }}
    >
      {dialog}
    </div>
  );
}
